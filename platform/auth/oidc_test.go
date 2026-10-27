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

func TestVerifyAccessToken(t *testing.T) {
	idp := newFakeIdP(t)
	p := idp.provider(t)

	tests := []struct {
		name   string
		header map[string]any
		claims map[string]any
		perm   Permission
		want   error
	}{
		{"CLI writes", rs256, idp.accessClaims(cliID), PermWrite, nil},
		{"Claude reads", es256, idp.accessClaims(claudeID), PermRead, nil},
		{"client_id instead of azp", rs256, with(idp.accessClaims(""), "azp", nil), PermRead, nil},
		{"another user", rs256, with(idp.accessClaims(cliID), "sub", "someone-else"), 0, ErrNotOwner},
		{"another project", rs256, with(idp.accessClaims(cliID), "aud", []string{"other-project"}), 0, ErrInvalidToken},
		{"another issuer", rs256, with(idp.accessClaims(cliID), "iss", "https://evil.example"), 0, ErrInvalidToken},
		{"expired", rs256, with(idp.accessClaims(cliID), "exp", time.Now().Add(-time.Hour).Unix()), 0, ErrInvalidToken},
		{"no expiry", rs256, with(idp.accessClaims(cliID), "exp", nil), 0, ErrInvalidToken},
		{"not valid yet", rs256, with(idp.accessClaims(cliID), "nbf", time.Now().Add(5*time.Minute).Unix()), 0, ErrInvalidToken},
		{"alg none", map[string]any{"alg": "none", "kid": "rsa"}, idp.accessClaims(cliID), 0, ErrInvalidToken},
		{"HS256", map[string]any{"alg": "HS256", "kid": "rsa"}, idp.accessClaims(cliID), 0, ErrInvalidToken},
		// An RS256 signature checked against the EC key, and the other way round.
		{"RS256 with the EC kid", map[string]any{"alg": "RS256", "kid": "ec"}, idp.accessClaims(cliID), 0, ErrInvalidToken},
		{"ES256 with the RSA kid", map[string]any{"alg": "ES256", "kid": "rsa"}, idp.accessClaims(cliID), 0, ErrInvalidToken},
		{"unknown kid", map[string]any{"alg": "RS256", "kid": "k2"}, idp.accessClaims(cliID), 0, ErrUnknownKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := p.VerifyAccessToken(context.Background(), idp.s.sign(t, tt.header, tt.claims))
			if tt.want != nil {
				if !errors.Is(err, tt.want) {
					t.Fatalf("want %v, got %v", tt.want, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if id.Subject != owner || id.Permission != tt.perm {
				t.Fatalf("identity %+v, want permission %v", id, tt.perm)
			}
		})
	}
}

func TestVerifyAccessTokenRejectsTamperedPayload(t *testing.T) {
	idp := newFakeIdP(t)
	p := idp.provider(t)
	for _, h := range []map[string]any{rs256, es256} {
		parts := strings.Split(idp.s.sign(t, h, with(idp.accessClaims(cliID), "sub", "someone-else")), ".")
		forged, _ := json.Marshal(idp.accessClaims(cliID))
		token := parts[0] + "." + b64(forged) + "." + parts[2]
		if _, err := p.VerifyAccessToken(context.Background(), token); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("%v: want ErrInvalidToken, got %v", h["alg"], err)
		}
	}
}

func TestDiscoveryMustMatchTheIssuer(t *testing.T) {
	idp := newFakeIdP(t)
	// Configured as one issuer, the document speaks for another.
	p, err := NewProvider(OIDCConfig{Issuer: idp.srv.URL + "/other", Audience: project, OwnerSubject: owner})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Metadata(context.Background()); err == nil {
		t.Fatal("want an error for a discovery document from another issuer")
	}
}

func TestDiscoveryKeepsTheLastGoodDocument(t *testing.T) {
	idp := newFakeIdP(t)
	p := idp.provider(t)
	now := time.Now()
	p.now = func() time.Time { return now }
	if _, err := p.Metadata(context.Background()); err != nil {
		t.Fatal(err)
	}
	idp.srv.Close()
	now = now.Add(2 * time.Hour) // past the cache, and the provider is down
	m, err := p.Metadata(context.Background())
	if err != nil || m.TokenEndpoint == "" {
		t.Fatalf("want the cached document, got %+v, %v", m, err)
	}
}

func TestProviderDownDoesNotBreakStartup(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	p, err := NewProvider(OIDCConfig{Issuer: srv.URL, Audience: project, OwnerSubject: owner})
	if err != nil {
		t.Fatalf("NewProvider must not call the provider: %v", err)
	}
	if _, err := p.VerifyAccessToken(context.Background(), "a.b.c"); err == nil {
		t.Fatal("want an error while the provider is down")
	}
}

func TestPermissionAllows(t *testing.T) {
	for _, tt := range []struct {
		p      Permission
		method string
		want   bool
	}{
		{PermRead, http.MethodGet, true},
		{PermRead, http.MethodHead, true},
		{PermRead, http.MethodPost, false},
		{PermRead, http.MethodDelete, false},
		{PermWrite, http.MethodPut, true},
		{PermWrite, http.MethodGet, true},
		{0, http.MethodGet, false},
	} {
		if got := tt.p.Allows(tt.method); got != tt.want {
			t.Errorf("%v allows %s = %v", tt.p, tt.method, got)
		}
	}
}
