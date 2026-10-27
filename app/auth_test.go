package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lastro/platform/auth"
	"lastro/platform/config"
)

func quietApp() *App {
	return &App{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Version: "test"}
}

// handlerWith wires the app's routes for an auth config, without a database
// or modules: these tests stop at authentication.
func handlerWith(t *testing.T, a config.Auth) http.Handler {
	t.Helper()
	app := quietApp()
	app.Config = config.Config{Env: "prod", Auth: a}
	w, err := newAuth(app.Config, app.Logger)
	if err != nil {
		t.Fatal(err)
	}
	app.auth = w
	return app.Handler()
}

func oidcConfig() config.Auth {
	return config.Auth{
		PublicURL: "https://lastro.example",
		// Never reached: nothing here gets as far as verifying a token.
		OIDC: config.OIDC{
			Issuer: "https://auth.example", Audience: "project-1", OwnerSubject: "owner-1",
			Scopes: []string{"urn:zitadel:iam:org:project:id:project-1:aud"}, CLIClientID: "cli-1",
			WebClientID: "web-1",
		},
		SessionKey: []byte("0123456789abcdef0123456789abcdef"),
	}
}

func TestProtectedResourceMetadata(t *testing.T) {
	h := handlerWith(t, oidcConfig())
	for path, resource := range map[string]string{
		auth.MetadataPath:          "https://lastro.example",
		auth.MetadataPath + "/mcp": "https://lastro.example/mcp",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", path, rec.Code)
		}
		var m auth.ResourceMetadata
		if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
			t.Fatal(err)
		}
		if m.Resource != resource || len(m.AuthorizationServers) != 1 || m.AuthorizationServers[0] != "https://auth.example" || len(m.ScopesSupported) != 1 {
			t.Fatalf("%s: %+v", path, m)
		}
		if (path == auth.MetadataPath) != (m.CLIClientID == "cli-1") {
			t.Fatalf("%s: CLI client %q", path, m.CLIClientID)
		}
	}
}

func TestRoutesNeedCredentials(t *testing.T) {
	h := handlerWith(t, oidcConfig())
	for _, tt := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/api/v1/me", http.StatusUnauthorized},
		{http.MethodPost, "/api/v1/sync", http.StatusUnauthorized},
		{http.MethodPost, "/mcp", http.StatusUnauthorized},
		// The login starts without credentials; the provider is down here, so
		// it says so instead of redirecting.
		{http.MethodGet, auth.LoginPath, http.StatusServiceUnavailable},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
		if rec.Code != tt.status {
			t.Errorf("%s %s: status %d, want %d", tt.method, tt.path, rec.Code, tt.status)
		}
		if tt.status == http.StatusUnauthorized && !strings.Contains(rec.Header().Get("WWW-Authenticate"), "resource_metadata=") {
			t.Errorf("%s %s: no resource_metadata in WWW-Authenticate", tt.method, tt.path)
		}
	}
}

// lastro never runs open: without a provider, and auth not turned off, the
// app does not start.
func TestNoProviderNoStart(t *testing.T) {
	if _, err := newAuth(config.Config{Env: "prod"}, quietApp().Logger); err == nil {
		t.Fatal("want an error without a sign-in provider")
	}
}

// With auth off (dev) there is no metadata, and the answer is a 404, not the
// web app: that is how `lastro connect` tells sign-in is off.
func TestAuthOffNoMetadata(t *testing.T) {
	h := handlerWith(t, config.Auth{Disabled: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, auth.MetadataPath, nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
}
