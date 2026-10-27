package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// HeaderName is the header Cloudflare Access injects with the JWT.
	HeaderName = "Cf-Access-Jwt-Assertion"
	// CookieName is the cookie Access uses in the browser.
	CookieName = "CF_Authorization"
)

// Verifier validates Cloudflare Access tokens for one team and one
// application (ADR-0004). It stays while Access is still in front; whoever
// gets through Access is the owner, so its tokens write.
type Verifier struct {
	issuer   string
	audience string
	keys     *keySet
	now      func() time.Time
}

// NewVerifier creates a verifier. teamDomain looks like
// "your-team.cloudflareaccess.com" and audience is the application's AUD tag.
func NewVerifier(teamDomain, audience string) (*Verifier, error) {
	d := strings.TrimSpace(teamDomain)
	d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
	d = strings.TrimRight(d, "/")
	if d == "" || strings.TrimSpace(audience) == "" {
		return nil, errors.New("auth: team domain and audience are required")
	}
	issuer := "https://" + d
	return &Verifier{
		issuer:   issuer,
		audience: strings.TrimSpace(audience),
		keys:     newKeySet(issuer+"/cdn-cgi/access/certs", &http.Client{Timeout: 5 * time.Second}),
		now:      time.Now,
	}, nil
}

// Verify checks the token's signature, issuer, audience and validity window.
func (v *Verifier) Verify(ctx context.Context, token string) (Identity, error) {
	c, err := parseToken(ctx, v.keys, token)
	if err != nil {
		return Identity{}, err
	}
	switch {
	case c.Iss != v.issuer:
		return Identity{}, fmt.Errorf("%w: issuer %q", ErrInvalidToken, c.Iss)
	case !contains(c.Aud, v.audience):
		return Identity{}, fmt.Errorf("%w: audience", ErrInvalidToken)
	}
	if err := c.checkTime(v.now()); err != nil {
		return Identity{}, err
	}
	return Identity{Email: c.Email, Subject: c.Sub, ClientID: "cloudflare-access", Permission: PermWrite}, nil
}

// token returns the Access JWT a request carries, from the header or the cookie.
func (v *Verifier) token(r *http.Request) string {
	if t := r.Header.Get(HeaderName); t != "" {
		return t
	}
	if c, err := r.Cookie(CookieName); err == nil {
		return c.Value
	}
	return ""
}
