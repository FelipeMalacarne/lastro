// Package auth decides who is making a request (ADR-0008).
//
// lastro is a resource server: it verifies tokens and never issues them.
// Three ways in, each on only when configured:
//
//   - the web app's session cookie, set by the BFF login against the OIDC provider;
//   - a bearer token from that provider (the lastro CLI, MCP clients, a phone app);
//   - the Cloudflare Access JWT, while Access is still in front.
//
// Only the owner gets in, and what a client may do is its Permission.
package auth

import (
	"context"
	"net/http"
)

// Permission is what a client may do. Reads are GET, HEAD and OPTIONS;
// everything else writes.
type Permission int

const (
	PermRead Permission = iota + 1
	PermWrite
)

// Allows reports whether the permission covers an HTTP method.
func (p Permission) Allows(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return p >= PermRead
	}
	return p >= PermWrite
}

func (p Permission) String() string {
	switch p {
	case PermRead:
		return "read"
	case PermWrite:
		return "write"
	}
	return "none"
}

// Identity is who is making the request, and through which client.
type Identity struct {
	Email   string
	Subject string
	// ClientID is the OAuth client the token was issued to; "web" for a
	// session and "cloudflare-access" for an Access JWT.
	ClientID   string
	Permission Permission
}

type ctxKey struct{}

func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// Disabled verifies nothing and marks every request as coming from the dev user.
// config.Load refuses this outside LASTRO_ENV=dev.
func Disabled() func(http.Handler) http.Handler {
	dev := Identity{Email: "dev@localhost", Subject: "dev", ClientID: "dev", Permission: PermWrite}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), dev)))
		})
	}
}
