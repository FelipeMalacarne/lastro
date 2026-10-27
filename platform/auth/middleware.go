package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// Authenticator requires an identity on every request it guards. Each way in
// is optional: nil means not configured.
type Authenticator struct {
	Logger *slog.Logger
	Web    *Web      // session cookie, and the redirect to the login
	OIDC   *Provider // bearer tokens
	CF     *Verifier // Cloudflare Access, while it is still in front
	// PublicURL is lastro's own origin; unsafe requests carrying a session
	// must come from it.
	PublicURL string
}

// Require guards the API and the web app: a session, a bearer token or an
// Access JWT, and a permission that covers the method.
func (a *Authenticator) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := a.identify(r, true)
		if err != nil {
			a.deny(w, r, err, a.resourceMetadataURL(""), true)
			return
		}
		if !id.Permission.Allows(r.Method) {
			writeJSON(w, http.StatusForbidden, map[string]string{"code": "forbidden", "message": "this client can only read"})
			return
		}
		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
	})
}

// RequireBearer guards MCP: only bearer tokens, from any client of the owner.
// MCP sends every call as a POST, so the method says nothing here; its tools
// are read-only, and a tool that writes checks the permission itself.
func (a *Authenticator) RequireBearer(resource string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, err := a.bearer(r)
			if err == nil && id.Subject == "" {
				err = errNoCredentials
			}
			if err != nil {
				a.deny(w, r, err, a.resourceMetadataURL(resource), false)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}

var errNoCredentials = errors.New("auth: no credentials")

// identify tries the session, then a bearer token, then Access. A bearer
// token that is present but invalid fails the request: it never falls
// through to the next way in.
func (a *Authenticator) identify(r *http.Request, allowSession bool) (Identity, error) {
	if allowSession && a.Web != nil {
		if id, ok := a.Web.Session(r); ok {
			if PermRead.Allows(r.Method) || a.sameOrigin(r) {
				return id, nil
			}
			return Identity{}, errors.New("auth: cross-site request with a session")
		}
	}
	if id, err := a.bearer(r); err != nil || id.Subject != "" {
		return id, err
	}
	if a.CF != nil {
		if t := a.CF.token(r); t != "" {
			return a.CF.Verify(r.Context(), t)
		}
	}
	return Identity{}, errNoCredentials
}

// bearer verifies the Authorization header. No header is not an error: it
// returns an empty identity.
func (a *Authenticator) bearer(r *http.Request) (Identity, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return Identity{}, nil
	}
	scheme, token, ok := strings.Cut(h, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return Identity{}, errors.New("auth: malformed authorization header")
	}
	if a.OIDC == nil {
		return Identity{}, errors.New("auth: bearer tokens are not accepted here")
	}
	return a.OIDC.VerifyAccessToken(r.Context(), strings.TrimSpace(token))
}

// sameOrigin checks an unsafe request carrying a session came from lastro
// itself. SameSite=Lax already keeps the cookie off cross-site requests;
// this is the second lock. Browsers send Origin on every unsafe request.
func (a *Authenticator) sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") == "" || r.Header.Get("Sec-Fetch-Site") == "same-origin"
	}
	want, err := url.Parse(a.PublicURL)
	if err != nil || want.Host == "" {
		return false
	}
	return origin == want.Scheme+"://"+want.Host
}

func (a *Authenticator) deny(w http.ResponseWriter, r *http.Request, err error, metadataURL string, browser bool) {
	if !errors.Is(err, errNoCredentials) {
		a.Logger.WarnContext(r.Context(), "credentials rejected", "err", err, "path", r.URL.Path)
	}
	// A browser opening a page goes to the login instead of a bare 401.
	if browser && a.Web != nil && r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/api/") &&
		strings.Contains(r.Header.Get("Accept"), "text/html") {
		http.Redirect(w, r, LoginPath+"?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
		return
	}
	if metadataURL != "" {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+metadataURL+`"`)
	}
	body := map[string]string{"code": "unauthorized", "message": "sign in to continue"}
	if browser && a.Web != nil {
		body["login"] = LoginPath
	}
	writeJSON(w, http.StatusUnauthorized, body)
}

func (a *Authenticator) resourceMetadataURL(resource string) string {
	if a.OIDC == nil || a.PublicURL == "" {
		return ""
	}
	return strings.TrimRight(a.PublicURL, "/") + MetadataPath + resource
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
