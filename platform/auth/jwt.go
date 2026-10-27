package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	keyTTL     = time.Hour
	minRefresh = time.Minute
	leeway     = time.Minute
)

var (
	ErrInvalidToken = errors.New("auth: invalid token")
	ErrUnknownKey   = errors.New("auth: unknown signing key")
)

type tokenHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

// tokenClaims holds every claim a verifier here reads. Which ones matter
// depends on the token: an access token or an ID token.
type tokenClaims struct {
	Email    string   `json:"email"`
	Sub      string   `json:"sub"`
	Iss      string   `json:"iss"`
	Aud      audience `json:"aud"`
	Exp      int64    `json:"exp"`
	Nbf      int64    `json:"nbf"`
	Azp      string   `json:"azp"`
	ClientID string   `json:"client_id"`
	Nonce    string   `json:"nonce"`
}

// client is who the token was issued to: azp, then client_id.
func (c tokenClaims) client() string {
	if c.Azp != "" {
		return c.Azp
	}
	return c.ClientID
}

// checkTime checks the validity window, with a minute of leeway for clock skew.
func (c tokenClaims) checkTime(now time.Time) error {
	switch {
	case c.Exp == 0 || now.After(time.Unix(c.Exp, 0).Add(leeway)):
		return fmt.Errorf("%w: expired", ErrInvalidToken)
	case c.Nbf != 0 && now.Add(leeway).Before(time.Unix(c.Nbf, 0)):
		return fmt.Errorf("%w: not valid yet", ErrInvalidToken)
	}
	return nil
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

// parseToken checks a JWT's signature against the key set and returns its
// claims. It checks nothing else: issuer, audience and time are the caller's.
// Only RS256 and ES256 are accepted, and the key must match the algorithm.
func parseToken(ctx context.Context, ks *keySet, token string) (tokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return tokenClaims{}, fmt.Errorf("%w: malformed", ErrInvalidToken)
	}

	var h tokenHeader
	if err := decodeSegment(parts[0], &h); err != nil {
		return tokenClaims{}, fmt.Errorf("%w: header", ErrInvalidToken)
	}
	if h.Alg != "RS256" && h.Alg != "ES256" {
		return tokenClaims{}, fmt.Errorf("%w: algorithm %q not accepted", ErrInvalidToken, h.Alg)
	}
	if h.Kid == "" {
		return tokenClaims{}, fmt.Errorf("%w: missing kid", ErrInvalidToken)
	}

	key, err := ks.key(ctx, h.Kid)
	if err != nil {
		return tokenClaims{}, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[2], "="))
	if err != nil {
		return tokenClaims{}, fmt.Errorf("%w: malformed signature", ErrInvalidToken)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !verifySignature(h.Alg, key, digest[:], sig) {
		return tokenClaims{}, fmt.Errorf("%w: signature", ErrInvalidToken)
	}

	var c tokenClaims
	if err := decodeSegment(parts[1], &c); err != nil {
		return tokenClaims{}, fmt.Errorf("%w: claims", ErrInvalidToken)
	}
	return c, nil
}

func verifySignature(alg string, key crypto.PublicKey, digest, sig []byte) bool {
	switch alg {
	case "RS256":
		k, ok := key.(*rsa.PublicKey)
		return ok && rsa.VerifyPKCS1v15(k, crypto.SHA256, digest, sig) == nil
	case "ES256":
		k, ok := key.(*ecdsa.PublicKey)
		if !ok || k.Curve != elliptic.P256() || len(sig) != 64 {
			return false
		}
		r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
		return ecdsa.Verify(k, digest, r, s)
	}
	return false
}

// keySet is a JWKS: the public keys a provider signs with, fetched from its
// URL and cached.
type keySet struct {
	url    string
	client *http.Client
	now    func() time.Time

	mu          sync.Mutex
	keys        map[string]crypto.PublicKey
	fetchedAt   time.Time
	lastAttempt time.Time
}

func newKeySet(url string, client *http.Client) *keySet {
	return &keySet{url: url, client: client, now: time.Now}
}

// key returns the public key for kid, refreshing the cache when needed.
// Refreshes are limited to one per minute, so random kids cannot turn into a
// flood of requests to the provider.
func (k *keySet) key(ctx context.Context, kid string) (crypto.PublicKey, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	now := k.now()
	key, known := k.keys[kid]
	if known && now.Sub(k.fetchedAt) < keyTTL {
		return key, nil
	}
	if now.Sub(k.lastAttempt) >= minRefresh {
		k.lastAttempt = now
		keys, err := k.fetch(ctx)
		switch {
		case err == nil:
			k.keys, k.fetchedAt = keys, now
		case !known:
			return nil, err
		}
		// if the refresh failed but the key was already known, keep using it
	}
	if key, ok := k.keys[kid]; ok {
		return key, nil
	}
	return nil, ErrUnknownKey
}

func (k *keySet) fetch(ctx context.Context) (map[string]crypto.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.url, nil)
	if err != nil {
		return nil, fmt.Errorf("auth: fetch keys: %w", err)
	}
	resp, err := k.client.Do(req)
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
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("auth: decode keys: %w", err)
	}
	keys := make(map[string]crypto.PublicKey, len(body.Keys))
	for _, j := range body.Keys {
		if j.Kid == "" {
			continue
		}
		var (
			pub crypto.PublicKey
			err error
		)
		switch j.Kty {
		case "RSA":
			pub, err = rsaPublicKey(j.N, j.E)
		case "EC":
			pub, err = ecPublicKey(j.Crv, j.X, j.Y)
		default:
			continue
		}
		if err != nil {
			continue
		}
		keys[j.Kid] = pub
	}
	if len(keys) == 0 {
		return nil, errors.New("auth: no valid keys")
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

// ecPublicKey reads a P-256 key, the only curve ES256 uses.
func ecPublicKey(crv, x, y string) (*ecdsa.PublicKey, error) {
	if crv != "P-256" {
		return nil, fmt.Errorf("auth: curve %q not accepted", crv)
	}
	xb, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(x, "="))
	if err != nil {
		return nil, err
	}
	yb, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(y, "="))
	if err != nil {
		return nil, err
	}
	if len(xb) != 32 || len(yb) != 32 {
		return nil, errors.New("auth: invalid P-256 point")
	}
	// The uncompressed SEC 1 encoding: 0x04 || X || Y. ParseUncompressedPublicKey
	// rejects points that are not on the curve.
	return ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, xb...), yb...))
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
