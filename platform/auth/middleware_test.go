package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func echo() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := IdentityFrom(r.Context())
		_, _ = w.Write([]byte(id.ClientID))
	})
}

func TestRequire(t *testing.T) {
	idp := newFakeIdP(t)
	web := newTestWeb(t, idp)
	a := &Authenticator{Logger: quiet(), Web: web, OIDC: idp.provider(t), PublicURL: publicURL}
	h := a.Require(echo())

	cli := idp.s.sign(t, rs256, idp.accessClaims(cliID))
	claude := idp.s.sign(t, rs256, idp.accessClaims(claudeID))
	stranger := idp.s.sign(t, rs256, with(idp.accessClaims(cliID), "sub", "someone-else"))
	sess, err := web.sealer.Seal(SessionCookie, session{Sub: owner}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		method   string
		path     string
		set      func(*http.Request)
		status   int
		location string
	}{
		{"CLI writes", http.MethodPost, "/api/v1/sync", bearer(cli), http.StatusOK, ""},
		{"Claude reads", http.MethodGet, "/api/v1/budget", bearer(claude), http.StatusOK, ""},
		{"Claude cannot write", http.MethodPost, "/api/v1/sync", bearer(claude), http.StatusForbidden, ""},
		{"someone else", http.MethodGet, "/api/v1/budget", bearer(stranger), http.StatusUnauthorized, ""},
		{"session reads", http.MethodGet, "/api/v1/budget", cookie(sess), http.StatusOK, ""},
		{"session writes from lastro", http.MethodPost, "/api/v1/sync", func(r *http.Request) {
			cookie(sess)(r)
			r.Header.Set("Origin", publicURL)
		}, http.StatusOK, ""},
		{"session writes from another site", http.MethodPost, "/api/v1/sync", func(r *http.Request) {
			cookie(sess)(r)
			r.Header.Set("Origin", "https://evil.example")
		}, http.StatusUnauthorized, ""},
		{"API without credentials", http.MethodGet, "/api/v1/budget", func(*http.Request) {}, http.StatusUnauthorized, ""},
		{"a page without credentials goes to the login", http.MethodGet, "/budget?month=2026-10", func(r *http.Request) {
			r.Header.Set("Accept", "text/html,application/xhtml+xml")
		}, http.StatusFound, LoginPath + "?next=%2Fbudget%3Fmonth%3D2026-10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			tt.set(req)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, tt.status, rec.Body)
			}
			if tt.location != "" && rec.Header().Get("Location") != tt.location {
				t.Fatalf("location %q, want %q", rec.Header().Get("Location"), tt.location)
			}
		})
	}
}

func TestUnauthorizedPointsToTheMetadata(t *testing.T) {
	idp := newFakeIdP(t)
	a := &Authenticator{Logger: quiet(), Web: newTestWeb(t, idp), OIDC: idp.provider(t), PublicURL: publicURL}

	rec := httptest.NewRecorder()
	a.Require(echo()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/budget", nil))
	if got := rec.Header().Get("WWW-Authenticate"); got != `Bearer resource_metadata="`+publicURL+MetadataPath+`"` {
		t.Fatalf("WWW-Authenticate = %q", got)
	}
	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["login"] != LoginPath {
		t.Fatalf("body %v: the web app needs where to sign in", body)
	}

	rec = httptest.NewRecorder()
	a.RequireBearer("/mcp")(echo()).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if got := rec.Header().Get("WWW-Authenticate"); !strings.HasSuffix(got, MetadataPath+`/mcp"`) {
		t.Fatalf("MCP WWW-Authenticate = %q", got)
	}
}

func TestRequireBearer(t *testing.T) {
	idp := newFakeIdP(t)
	web := newTestWeb(t, idp)
	a := &Authenticator{Logger: quiet(), Web: web, OIDC: idp.provider(t), PublicURL: publicURL}
	h := a.RequireBearer("/mcp")(echo())
	sess, _ := web.sealer.Seal(SessionCookie, session{Sub: owner}, time.Hour)

	// A read-only client calls MCP with POST: the method is not the permission here.
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	bearer(idp.s.sign(t, es256, idp.accessClaims(claudeID)))(req)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != claudeID {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body)
	}

	// A session is not enough for MCP.
	req = httptest.NewRequest(http.MethodPost, "/mcp", nil)
	cookie(sess)(req)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("session on MCP: status %d", rec.Code)
	}
}

func bearer(tok string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) }
}

func cookie(v string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: SessionCookie, Value: v}) }
}
