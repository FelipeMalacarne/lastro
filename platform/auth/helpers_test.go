package auth

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// signer holds an RSA key (kid "rsa") and a P-256 key (kid "ec").
type signer struct {
	rsa *rsa.PrivateKey
	ec  *ecdsa.PrivateKey
}

var (
	rs256 = map[string]any{"alg": "RS256", "kid": "rsa", "typ": "JWT"}
	es256 = map[string]any{"alg": "ES256", "kid": "ec", "typ": "JWT"}
)

func newSigner(t *testing.T) *signer {
	t.Helper()
	rk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ek, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &signer{rsa: rk, ec: ek}
}

func (s *signer) jwks() map[string]any {
	pt, _ := s.ec.PublicKey.Bytes() // 0x04 || X || Y
	return map[string]any{"keys": []map[string]string{
		{"kid": "rsa", "kty": "RSA", "alg": "RS256", "use": "sig", "n": b64(s.rsa.N.Bytes()), "e": b64(big.NewInt(int64(s.rsa.E)).Bytes())},
		{"kid": "ec", "kty": "EC", "crv": "P-256", "alg": "ES256", "use": "sig", "x": b64(pt[1:33]), "y": b64(pt[33:])},
	}}
}

// sign signs with the key the header's alg names; an unknown alg signs with RSA.
func (s *signer) sign(t *testing.T, header, claims map[string]any) string {
	t.Helper()
	h, _ := json.Marshal(header)
	c, _ := json.Marshal(claims)
	signed := b64(h) + "." + b64(c)
	digest := sha256.Sum256([]byte(signed))
	var sig []byte
	if header["alg"] == "ES256" {
		r, ss, err := ecdsa.Sign(rand.Reader, s.ec, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		sig = make([]byte, 64)
		r.FillBytes(sig[:32])
		ss.FillBytes(sig[32:])
	} else {
		var err error
		if sig, err = rsa.SignPKCS1v15(rand.Reader, s.rsa, crypto.SHA256, digest[:]); err != nil {
			t.Fatal(err)
		}
	}
	return signed + "." + b64(sig)
}

// fakeIdP is an OIDC provider: discovery, keys and a token endpoint whose
// answer the test sets.
type fakeIdP struct {
	srv    *httptest.Server
	s      *signer
	token  func(r *http.Request) (int, map[string]any)
	tokens int
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	f := &fakeIdP{s: newSigner(t)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                        f.srv.URL,
			"authorization_endpoint":        f.srv.URL + "/oauth/v2/authorize",
			"token_endpoint":                f.srv.URL + "/oauth/v2/token",
			"jwks_uri":                      f.srv.URL + "/oauth/v2/keys",
			"end_session_endpoint":          f.srv.URL + "/oidc/v1/end_session",
			"device_authorization_endpoint": f.srv.URL + "/oauth/v2/device_authorization",
		})
	})
	mux.HandleFunc("GET /oauth/v2/keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(f.s.jwks())
	})
	mux.HandleFunc("POST /oauth/v2/token", func(w http.ResponseWriter, r *http.Request) {
		f.tokens++
		status, body := http.StatusBadRequest, map[string]any{"error": "invalid_grant"}
		if f.token != nil {
			status, body = f.token(r)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

const (
	owner    = "owner-sub"
	project  = "project-123"
	cliID    = "cli-client"
	webID    = "web-client"
	claudeID = "claude-client"
)

func (f *fakeIdP) provider(t *testing.T) *Provider {
	t.Helper()
	p, err := NewProvider(OIDCConfig{Issuer: f.srv.URL, Audience: project, OwnerSubject: owner, WriteClients: []string{cliID}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// accessClaims is a valid access token for the owner through client.
func (f *fakeIdP) accessClaims(client string) map[string]any {
	now := time.Now()
	return map[string]any{
		"iss": f.srv.URL, "sub": owner, "aud": []string{project, client}, "azp": client,
		"exp": now.Add(10 * time.Minute).Unix(), "iat": now.Unix(), "nbf": now.Add(-time.Minute).Unix(),
	}
}

func with(c map[string]any, k string, v any) map[string]any {
	out := make(map[string]any, len(c))
	for kk, vv := range c {
		out[kk] = vv
	}
	if v == nil {
		delete(out, k)
	} else {
		out[k] = v
	}
	return out
}

func testSealer(t *testing.T) *Sealer {
	t.Helper()
	s, err := NewSealer([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
