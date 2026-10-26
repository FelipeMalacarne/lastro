// Package auth verifies the JWT that Cloudflare Access attaches to every request.
//
// The app has no login of its own: whoever reaches it already went through
// Access. The token is still verified here, so a direct request to the
// service that bypasses the tunnel is rejected.
package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// HeaderName is the header Cloudflare Access injects with the JWT.
	HeaderName = "Cf-Access-Jwt-Assertion"
	// CookieName is the cookie Access uses in the browser.
	CookieName = "CF_Authorization"

	keyTTL     = time.Hour
	minRefresh = time.Minute
	leeway     = time.Minute
)

var (
	ErrInvalidToken = errors.New("auth: invalid token")
	ErrUnknownKey   = errors.New("auth: unknown signing key")
)

// Identity is who is making the request.
type Identity struct {
	Email   string
	Subject string
}

type ctxKey struct{}

func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// Verifier validates Cloudflare Access tokens for one team and one application.
type Verifier struct {
	issuer   string
	audience string
	certsURL string
	client   *http.Client
	now      func() time.Time

	mu          sync.Mutex
	keys        map[string]*rsa.PublicKey
	fetchedAt   time.Time
	lastAttempt time.Time
}

// NewVerifier creates a verifier. teamDomain looks like
// "your-team.cloudflareaccess.com" and audience is the application's AUD tag.
func NewVerifier(teamDomain, audience string) (*Verifier, error) {
	d := strings.TrimSpace(teamDomain)
	d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
	d = strings.TrimRight(d, "/")
	if d == "" || strings.TrimSpace(audience) == "" {
		return nil, errors.New("auth: team domain and audience are required")
	}
	issuer := "https://" + d
	return &Verifier{
		issuer:   issuer,
		audience: strings.TrimSpace(audience),
		certsURL: issuer + "/cdn-cgi/access/certs",
		client:   &http.Client{Timeout: 5 * time.Second},
		now:      time.Now,
	}, nil
}

type tokenHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type tokenClaims struct {
	Email string   `json:"email"`
	Sub   string   `json:"sub"`
	Iss   string   `json:"iss"`
	Aud   audience `json:"aud"`
	Exp   int64    `json:"exp"`
	Nbf   int64    `json:"nbf"`
}

// audience accepts "aud" as a string or a list, as the JWT spec allows.
type audience []string

func (a *audience) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*a = audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

// Verify checks the token's signature, issuer, audience and validity window.
func (v *Verifier) Verify(ctx context.Context, token string) (Identity, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Identity{}, fmt.Errorf("%w: malformed", ErrInvalidToken)
	}

	var h tokenHeader
	if err := decodeSegment(parts[0], &h); err != nil {
		return Identity{}, fmt.Errorf("%w: header", ErrInvalidToken)
	}
	if h.Alg != "RS256" {
		return Identity{}, fmt.Errorf("%w: algorithm %q not accepted", ErrInvalidToken, h.Alg)
	}
	if h.Kid == "" {
		return Identity{}, fmt.Errorf("%w: missing kid", ErrInvalidToken)
	}

	key, err := v.key(ctx, h.Kid)
	if err != nil {
		return Identity{}, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[2], "="))
	if err != nil {
		return Identity{}, fmt.Errorf("%w: malformed signature", ErrInvalidToken)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return Identity{}, fmt.Errorf("%w: signature", ErrInvalidToken)
	}

	var c tokenClaims
	if err := decodeSegment(parts[1], &c); err != nil {
		return Identity{}, fmt.Errorf("%w: claims", ErrInvalidToken)
	}
	now := v.now()
	switch {
	case c.Iss != v.issuer:
		return Identity{}, fmt.Errorf("%w: issuer %q", ErrInvalidToken, c.Iss)
	case !contains(c.Aud, v.audience):
		return Identity{}, fmt.Errorf("%w: audience", ErrInvalidToken)
	case c.Exp == 0 || now.After(time.Unix(c.Exp, 0).Add(leeway)):
		return Identity{}, fmt.Errorf("%w: expired", ErrInvalidToken)
	case c.Nbf != 0 && now.Add(leeway).Before(time.Unix(c.Nbf, 0)):
		return Identity{}, fmt.Errorf("%w: not valid yet", ErrInvalidToken)
	}
	return Identity{Email: c.Email, Subject: c.Sub}, nil
}

// key returns the public key for kid, refreshing the cache when needed.
// Refreshes are limited to one per minute, so random kids cannot turn into a
// flood of requests to Cloudflare.
func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := v.now()
	k, known := v.keys[kid]
	if known && now.Sub(v.fetchedAt) < keyTTL {
		return k, nil
	}
	if now.Sub(v.lastAttempt) >= minRefresh {
		v.lastAttempt = now
		keys, err := v.fetchKeys(ctx)
		switch {
		case err == nil:
			v.keys, v.fetchedAt = keys, now
		case !known:
			return nil, err
		}
		// if the refresh failed but the key was already known, keep using it
	}
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, ErrUnknownKey
}

func (v *Verifier) fetchKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.certsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("auth: fetch keys: %w", err)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth: fetch keys: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth: fetch keys: status %d", resp.StatusCode)
	}

	var body struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("auth: decode keys: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(body.Keys))
	for _, k := range body.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		pub, err := rsaPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}
	if len(keys) == 0 {
		return nil, errors.New("auth: no valid RSA keys")
	}
	return keys, nil
}

func rsaPublicKey(n, e string) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(n, "="))
	if err != nil {
		return nil, err
	}
	eb, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(e, "="))
	if err != nil {
		return nil, err
	}
	ei := new(big.Int).SetBytes(eb)
	if !ei.IsInt64() || ei.Int64() < 3 || ei.Int64() > 1<<31-1 {
		return nil, errors.New("auth: invalid exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: int(ei.Int64())}, nil
}

func decodeSegment(seg string, out any) error {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(seg, "="))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Middleware requires a valid Cloudflare Access token.
func (v *Verifier) Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get(HeaderName)
			if token == "" {
				if c, err := r.Cookie(CookieName); err == nil {
					token = c.Value
				}
			}
			if token == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			id, err := v.Verify(r.Context(), token)
			if err != nil {
				logger.WarnContext(r.Context(), "cloudflare access token rejected", "err", err)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}

// Disabled verifies nothing and marks every request as coming from the dev user.
// config.Load refuses this outside LASTRO_ENV=dev.
func Disabled() func(http.Handler) http.Handler {
	dev := Identity{Email: "dev@localhost", Subject: "dev"}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), dev)))
		})
	}
}
