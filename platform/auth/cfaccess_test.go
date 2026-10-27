package auth

import (
	"context"
	"encoding/json"
	"errors"
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

type cfFixture struct {
	s   *signer
	v   *Verifier
	now time.Time
}

func newCFFixture(t *testing.T) *cfFixture {
	t.Helper()
	s := newSigner(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(s.jwks())
	}))
	t.Cleanup(srv.Close)

	v, err := NewVerifier(team, aud)
	if err != nil {
		t.Fatal(err)
	}
	f := &cfFixture{s: s, v: v, now: time.Unix(1_800_000_000, 0)}
	v.keys.url = srv.URL
	v.now = func() time.Time { return f.now }
	v.keys.now = v.now
	return f
}

func (f *cfFixture) validClaims() map[string]any {
	return map[string]any{
		"email": "me@example.com", "sub": "user-1",
		"iss": "https://" + team, "aud": []string{aud},
		"exp": f.now.Add(10 * time.Minute).Unix(), "nbf": f.now.Add(-time.Minute).Unix(),
	}
}

func TestVerifyAcceptsValidToken(t *testing.T) {
	f := newCFFixture(t)
	for name, h := range map[string]map[string]any{"RS256": rs256, "ES256": es256} {
		id, err := f.v.Verify(context.Background(), f.s.sign(t, h, f.validClaims()))
		if err != nil {
			t.Fatalf("%s: error: %v", name, err)
		}
		if id.Email != "me@example.com" || id.Permission != PermWrite {
			t.Fatalf("%s: identity %+v", name, id)
		}
	}
}

func TestVerifyRejects(t *testing.T) {
	f := newCFFixture(t)
	c := f.validClaims()
	tests := []struct {
		name   string
		header map[string]any
		claims map[string]any
		want   error
	}{
		{"wrong audience", rs256, with(c, "aud", []string{"other"}), ErrInvalidToken},
		{"wrong issuer", rs256, with(c, "iss", "https://other.cloudflareaccess.com"), ErrInvalidToken},
		{"expired", rs256, with(c, "exp", f.now.Add(-2*time.Minute).Unix()), ErrInvalidToken},
		{"no expiry", rs256, with(c, "exp", nil), ErrInvalidToken},
		{"not valid yet", rs256, with(c, "nbf", f.now.Add(5*time.Minute).Unix()), ErrInvalidToken},
		{"alg none", map[string]any{"alg": "none", "kid": "rsa"}, c, ErrInvalidToken},
		{"HS256", map[string]any{"alg": "HS256", "kid": "rsa"}, c, ErrInvalidToken},
		// An RS256 signature checked against the EC key, and the other way round.
		{"RS256 with the EC kid", map[string]any{"alg": "RS256", "kid": "ec"}, c, ErrInvalidToken},
		{"ES256 with the RSA kid", map[string]any{"alg": "ES256", "kid": "rsa"}, c, ErrInvalidToken},
		{"unknown kid", map[string]any{"alg": "RS256", "kid": "k2"}, c, ErrUnknownKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.v.Verify(context.Background(), f.s.sign(t, tt.header, tt.claims))
			if !errors.Is(err, tt.want) {
				t.Fatalf("want %v, got %v", tt.want, err)
			}
		})
	}
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	f := newCFFixture(t)
	for _, h := range []map[string]any{rs256, es256} {
		parts := strings.Split(f.s.sign(t, h, f.validClaims()), ".")
		forged, _ := json.Marshal(with(f.validClaims(), "email", "attacker@example.com"))
		token := parts[0] + "." + b64(forged) + "." + parts[2]
		if _, err := f.v.Verify(context.Background(), token); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("%v: want ErrInvalidToken, got %v", h["alg"], err)
		}
	}
}

func TestVerifierReadsHeaderAndCookie(t *testing.T) {
	f := newCFFixture(t)
	tok := f.s.sign(t, rs256, f.validClaims())
	a := &Authenticator{Logger: quiet(), CF: f.v}
	h := a.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := IdentityFrom(r.Context())
		_, _ = w.Write([]byte(id.Email))
	}))

	for name, set := range map[string]func(*http.Request){
		"header": func(r *http.Request) { r.Header.Set(HeaderName, tok) },
		"cookie": func(r *http.Request) { r.AddCookie(&http.Cookie{Name: CookieName, Value: tok}) },
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", nil)
		set(req)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != "me@example.com" {
			t.Fatalf("%s: status %d, body %q", name, rec.Code, rec.Body.String())
		}
	}
}
