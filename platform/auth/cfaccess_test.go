package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	team = "your-team.cloudflareaccess.com"
	aud  = "aud-tag-123"
)

type fixture struct {
	key    *rsa.PrivateKey
	server *httptest.Server
	v      *Verifier
	now    time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := map[string]any{"keys": []map[string]string{{
		"kid": "k1", "kty": "RSA", "alg": "RS256", "use": "sig",
		"n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes()),
	}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(srv.Close)

	v, err := NewVerifier(team, aud)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{key: key, server: srv, v: v, now: time.Unix(1_800_000_000, 0)}
	v.certsURL = srv.URL
	v.now = func() time.Time { return f.now }
	return f
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (f *fixture) sign(t *testing.T, header, claims map[string]any) string {
	t.Helper()
	h, _ := json.Marshal(header)
	c, _ := json.Marshal(claims)
	signed := b64(h) + "." + b64(c)
	digest := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signed + "." + b64(sig)
}

func (f *fixture) validClaims() map[string]any {
	return map[string]any{
		"email": "me@example.com", "sub": "user-1",
		"iss": "https://" + team, "aud": []string{aud},
		"exp": f.now.Add(10 * time.Minute).Unix(), "nbf": f.now.Add(-time.Minute).Unix(),
	}
}

var rs256 = map[string]any{"alg": "RS256", "kid": "k1", "typ": "JWT"}

func TestVerifyAcceptsValidToken(t *testing.T) {
	f := newFixture(t)
	id, err := f.v.Verify(context.Background(), f.sign(t, rs256, f.validClaims()))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if id.Email != "me@example.com" {
		t.Fatalf("email = %q", id.Email)
	}
}

func TestVerifyRejects(t *testing.T) {
	f := newFixture(t)
	with := func(k string, v any) map[string]any {
		c := f.validClaims()
		c[k] = v
		return c
	}
	tests := []struct {
		name   string
		header map[string]any
		claims map[string]any
		want   error
	}{
		{"wrong audience", rs256, with("aud", []string{"other"}), ErrInvalidToken},
		{"wrong issuer", rs256, with("iss", "https://other.cloudflareaccess.com"), ErrInvalidToken},
		{"expired", rs256, with("exp", f.now.Add(-2*time.Minute).Unix()), ErrInvalidToken},
		{"not valid yet", rs256, with("nbf", f.now.Add(5*time.Minute).Unix()), ErrInvalidToken},
		{"alg none", map[string]any{"alg": "none", "kid": "k1"}, f.validClaims(), ErrInvalidToken},
		{"unknown kid", map[string]any{"alg": "RS256", "kid": "k2"}, f.validClaims(), ErrUnknownKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.v.Verify(context.Background(), f.sign(t, tt.header, tt.claims))
			if !errors.Is(err, tt.want) {
				t.Fatalf("want %v, got %v", tt.want, err)
			}
		})
	}
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	f := newFixture(t)
	parts := strings.Split(f.sign(t, rs256, f.validClaims()), ".")
	c := f.validClaims()
	c["email"] = "attacker@example.com"
	forged, _ := json.Marshal(c)
	token := parts[0] + "." + b64(forged) + "." + parts[2]
	if _, err := f.v.Verify(context.Background(), token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}

func TestMiddleware(t *testing.T) {
	f := newFixture(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := f.v.Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := IdentityFrom(r.Context())
		_, _ = w.Write([]byte(id.Email))
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderName, f.sign(t, rs256, f.validClaims()))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "me@example.com" {
		t.Fatalf("with token: status %d, body %q", rec.Code, rec.Body.String())
	}
}
