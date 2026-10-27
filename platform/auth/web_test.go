package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const publicURL = "https://lastro.example"

func newTestWeb(t *testing.T, idp *fakeIdP) *Web {
	t.Helper()
	w, err := NewWeb(idp.provider(t), WebConfig{PublicURL: publicURL, ClientID: webID, ClientSecret: "s3cret"}, testSealer(t), quiet())
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// login runs GET /auth/login and returns the provider redirect and the state cookie.
func login(t *testing.T, w *Web, next string) (*url.URL, *http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	w.Login(rec, httptest.NewRequest(http.MethodGet, LoginPath+"?next="+url.QueryEscape(next), nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("login: status %d", rec.Code)
	}
	to, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	var c *http.Cookie
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == loginCookie {
			c = ck
		}
	}
	if c == nil {
		t.Fatal("login: no state cookie")
	}
	return to, c
}

func callback(w *Web, query string, c *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, CallbackPath+"?"+query, nil)
	if c != nil {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	w.Callback(rec, req)
	return rec
}

func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookie && c.MaxAge > 0 {
			return c
		}
	}
	return nil
}

func TestWebLogin(t *testing.T) {
	idp := newFakeIdP(t)
	w := newTestWeb(t, idp)
	to, c := login(t, w, "/budget?month=2026-10")

	q := to.Query()
	if !strings.HasSuffix(to.Path, "/oauth/v2/authorize") || q.Get("client_id") != webID ||
		q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" ||
		q.Get("redirect_uri") != publicURL+CallbackPath || q.Get("nonce") == "" || q.Get("state") == "" {
		t.Fatalf("authorize URL: %s", to)
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("state cookie flags: %+v", c)
	}

	var verifier string
	idp.token = func(r *http.Request) (int, map[string]any) {
		_ = r.ParseForm()
		verifier = r.PostForm.Get("code_verifier")
		claims := map[string]any{
			"iss": idp.srv.URL, "sub": owner, "aud": webID, "email": "me@example.com", "nonce": q.Get("nonce"),
			"exp": time.Now().Add(time.Hour).Unix(),
		}
		return http.StatusOK, map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": idp.s.sign(t, rs256, claims)}
	}

	rec := callback(w, "code=abc&state="+q.Get("state"), c)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/budget?month=2026-10" {
		t.Fatalf("callback: status %d, location %q, body %s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	if verifier == "" {
		t.Fatal("the code exchange carried no PKCE verifier")
	}
	sc := sessionCookie(rec)
	if sc == nil {
		t.Fatal("no session cookie")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(sc)
	id, ok := w.Session(req)
	if !ok || id.Subject != owner || id.Email != "me@example.com" || id.Permission != PermWrite {
		t.Fatalf("session: %+v, %v", id, ok)
	}
}

func TestWebCallbackRejects(t *testing.T) {
	idTokenFor := func(idp *fakeIdP, t *testing.T, claims map[string]any) func(*http.Request) (int, map[string]any) {
		return func(*http.Request) (int, map[string]any) {
			return http.StatusOK, map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": idp.s.sign(t, rs256, claims)}
		}
	}
	tests := []struct {
		name   string
		status int
		run    func(t *testing.T, idp *fakeIdP, w *Web) *httptest.ResponseRecorder
	}{
		{"no state cookie", http.StatusBadRequest, func(t *testing.T, _ *fakeIdP, w *Web) *httptest.ResponseRecorder {
			to, _ := login(t, w, "/")
			return callback(w, "code=abc&state="+to.Query().Get("state"), nil)
		}},
		{"state from another login", http.StatusBadRequest, func(t *testing.T, _ *fakeIdP, w *Web) *httptest.ResponseRecorder {
			_, c := login(t, w, "/")
			return callback(w, "code=abc&state=forged", c)
		}},
		{"provider error", http.StatusUnauthorized, func(t *testing.T, _ *fakeIdP, w *Web) *httptest.ResponseRecorder {
			to, c := login(t, w, "/")
			return callback(w, "error=access_denied&state="+to.Query().Get("state"), c)
		}},
		{"code refused", http.StatusUnauthorized, func(t *testing.T, _ *fakeIdP, w *Web) *httptest.ResponseRecorder {
			to, c := login(t, w, "/")
			return callback(w, "code=bad&state="+to.Query().Get("state"), c)
		}},
		{"someone else", http.StatusForbidden, func(t *testing.T, idp *fakeIdP, w *Web) *httptest.ResponseRecorder {
			to, c := login(t, w, "/")
			idp.token = idTokenFor(idp, t, map[string]any{"iss": idp.srv.URL, "sub": "intruder", "aud": webID, "nonce": to.Query().Get("nonce"), "exp": time.Now().Add(time.Hour).Unix()})
			return callback(w, "code=abc&state="+to.Query().Get("state"), c)
		}},
		{"wrong nonce", http.StatusUnauthorized, func(t *testing.T, idp *fakeIdP, w *Web) *httptest.ResponseRecorder {
			to, c := login(t, w, "/")
			idp.token = idTokenFor(idp, t, map[string]any{"iss": idp.srv.URL, "sub": owner, "aud": webID, "nonce": "replayed", "exp": time.Now().Add(time.Hour).Unix()})
			return callback(w, "code=abc&state="+to.Query().Get("state"), c)
		}},
		{"ID token for another client", http.StatusUnauthorized, func(t *testing.T, idp *fakeIdP, w *Web) *httptest.ResponseRecorder {
			to, c := login(t, w, "/")
			idp.token = idTokenFor(idp, t, map[string]any{"iss": idp.srv.URL, "sub": owner, "aud": "other", "nonce": to.Query().Get("nonce"), "exp": time.Now().Add(time.Hour).Unix()})
			return callback(w, "code=abc&state="+to.Query().Get("state"), c)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idp := newFakeIdP(t)
			w := newTestWeb(t, idp)
			rec := tt.run(t, idp, w)
			if rec.Code != tt.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, tt.status, rec.Body)
			}
			if sessionCookie(rec) != nil {
				t.Fatal("a failed login set a session")
			}
		})
	}
}

func TestWebLogout(t *testing.T) {
	idp := newFakeIdP(t)
	w := newTestWeb(t, idp)
	rec := httptest.NewRecorder()
	w.Logout(rec, httptest.NewRequest(http.MethodPost, LogoutPath, nil))
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), idp.srv.URL+"/oidc/v1/end_session?") {
		t.Fatalf("logout: status %d, location %q", rec.Code, rec.Header().Get("Location"))
	}
	cleared := false
	for _, c := range rec.Result().Cookies() {
		cleared = cleared || (c.Name == SessionCookie && c.MaxAge < 0)
	}
	if !cleared {
		t.Fatal("logout did not clear the session")
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"/budget?month=2026-10": "/budget?month=2026-10",
		"":                      "/",
		"https://evil.example":  "/",
		"//evil.example":        "/",
		"/\\evil.example":       "/",
		"budget":                "/",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}
