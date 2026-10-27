package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// fakeIdP answers the device flow and refreshes; each token it issues is
// "access-N".
type fakeIdP struct {
	srv *httptest.Server
	mu  sync.Mutex
	n   int
	// grants are the grant types the token endpoint saw.
	grants []string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	f := &fakeIdP{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer": f.srv.URL, "token_endpoint": f.srv.URL + "/token", "device_authorization_endpoint": f.srv.URL + "/device",
		})
	})
	mux.HandleFunc("POST /device", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("client_id") != "cli-1" || !strings.Contains(r.PostForm.Get("scope"), "offline_access") ||
			!strings.Contains(r.PostForm.Get("scope"), "project-aud") {
			http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dev-1", "user_code": "ABCD-EFGH", "verification_uri": f.srv.URL + "/activate",
			"verification_uri_complete": f.srv.URL + "/activate?user_code=ABCD-EFGH", "expires_in": 300, "interval": 1,
		})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.n++
		n := f.n
		f.grants = append(f.grants, r.PostForm.Get("grant_type"))
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-" + string(rune('0'+n)), "token_type": "Bearer", "refresh_token": "refresh-" + string(rune('0'+n)), "expires_in": 3600,
		})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// fakeServer is a lastro server that names the IdP and checks the token.
func fakeServer(t *testing.T, idp *fakeIdP) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resource": "x", "authorization_servers": []string{idp.srv.URL},
			"scopes_supported": []string{"project-aud"}, "lastro_cli_client_id": "cli-1",
		})
	})
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !strings.HasPrefix(tok, "access-") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"unauthorized","message":"sign in to continue"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(Me{Email: "me@example.com", Client: "cli-1", Permission: "write", Version: tok})
	})
	mux.HandleFunc("POST /api/v1/sync", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"conflict","message":"a sync is already running"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestConnectWithDeviceFlow(t *testing.T) {
	ctx := context.Background()
	idp := newFakeIdP(t)
	srv := fakeServer(t, idp)

	l, err := StartLogin(ctx, srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if l.Device == nil || l.Device.UserCode != "ABCD-EFGH" || !strings.Contains(l.Device.URL, "user_code=") {
		t.Fatalf("device: %+v", l.Device)
	}
	cfg, err := l.Finish(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != srv.URL || cfg.Auth == nil || cfg.Auth.Token.AccessToken != "access-1" || cfg.Auth.ClientID != "cli-1" {
		t.Fatalf("config: %+v", cfg)
	}

	me, err := New(ctx, cfg, func(Config) error { t.Fatal("nothing to save yet"); return nil }).Me(ctx)
	if err != nil || me.Email != "me@example.com" {
		t.Fatalf("me: %+v, %v", me, err)
	}
}

func TestExpiredTokenIsRefreshedAndSaved(t *testing.T) {
	ctx := context.Background()
	idp := newFakeIdP(t)
	srv := fakeServer(t, idp)
	cfg := Config{URL: srv.URL, Auth: &Auth{
		Issuer: idp.srv.URL, ClientID: "cli-1", TokenURL: idp.srv.URL + "/token",
		Token: &oauth2.Token{AccessToken: "stale", RefreshToken: "refresh-0", Expiry: time.Now().Add(-time.Minute)},
	}}

	var saved Config
	me, err := New(ctx, cfg, func(c Config) error { saved = c; return nil }).Me(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if me.Version != "access-1" {
		t.Fatalf("the request carried %q, want the refreshed token", me.Version)
	}
	if saved.Auth == nil || saved.Auth.Token.RefreshToken != "refresh-1" || saved.URL != srv.URL {
		t.Fatalf("saved %+v: the rotated refresh token must be kept", saved)
	}
	if len(idp.grants) != 1 || idp.grants[0] != "refresh_token" {
		t.Fatalf("grants %v", idp.grants)
	}
}

func TestConnectToDevServer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Me{Email: "dev@localhost"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	l, err := StartLogin(context.Background(), srv.URL)
	if err != nil || l.Device != nil {
		t.Fatalf("a server with auth off needs no sign-in: %+v, %v", l, err)
	}
	cfg, err := l.Finish(context.Background())
	if err != nil || cfg.Auth != nil {
		t.Fatalf("config %+v, %v", cfg, err)
	}
}

func TestNotLastro(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if _, err := StartLogin(context.Background(), srv.URL); err == nil {
		t.Fatal("want an error for a server that is not lastro")
	}
}

func TestNormalizeURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://lastro.example/":    "https://lastro.example",
		"https://lastro.example?x=1": "https://lastro.example",
		"http://localhost:8080":      "http://localhost:8080",
		"http://127.0.0.1:8080/":     "http://127.0.0.1:8080",
	} {
		if got, err := NormalizeURL(in); err != nil || got != want {
			t.Errorf("NormalizeURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"http://lastro.example", "lastro.example", "ftp://lastro.example", ""} {
		if _, err := NormalizeURL(in); err == nil {
			t.Errorf("NormalizeURL(%q): want an error", in)
		}
	}
}

func TestSyncAlreadyRunning(t *testing.T) {
	srv := fakeServer(t, newFakeIdP(t))
	_, err := New(context.Background(), Config{URL: srv.URL}, nil).StartSync(context.Background())
	if !errors.Is(err, ErrSyncRunning) {
		t.Fatalf("want ErrSyncRunning, got %v", err)
	}
}

func TestUnauthorizedSaysWhatToDo(t *testing.T) {
	srv := fakeServer(t, newFakeIdP(t))
	_, err := New(context.Background(), Config{URL: srv.URL}, nil).Me(context.Background())
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != http.StatusUnauthorized || !strings.Contains(err.Error(), "lastro connect") {
		t.Fatalf("got %v", err)
	}
}

func TestConfigFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := Load(); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("before connect: %v", err)
	}
	c := Config{URL: "https://lastro.example", Auth: &Auth{ClientID: "cli-1", Token: &oauth2.Token{AccessToken: "a", RefreshToken: "r"}}}
	if err := Save(c); err != nil {
		t.Fatal(err)
	}
	p, _ := Path()
	if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("config file mode: %v, %v", fi.Mode().Perm(), err)
	}
	got, err := Load()
	if err != nil || got.Auth.Token.RefreshToken != "r" {
		t.Fatalf("load: %+v, %v", got, err)
	}
	if err := Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("after disconnect: %v", err)
	}
}
