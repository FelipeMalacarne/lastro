package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const (
	// SessionCookie holds the web app's sealed session.
	SessionCookie = "lastro_session"
	loginCookie   = "lastro_login"

	sessionTTL = 14 * 24 * time.Hour
	loginTTL   = 10 * time.Minute

	// LoginPath starts the web login; CallbackPath and LogoutPath finish and end it.
	LoginPath    = "/auth/login"
	CallbackPath = "/auth/callback"
	LogoutPath   = "/auth/logout"
)

// WebConfig is the web app as an OAuth client of the provider: a confidential
// client, since the server holds the secret and the browser never sees a token.
type WebConfig struct {
	PublicURL    string // e.g. https://lastro.ftm.dev.br
	ClientID     string
	ClientSecret string
	Scopes       []string // beyond openid, email and profile
}

// Web is the BFF login: Authorization Code with PKCE against the provider,
// ending in a sealed session cookie.
type Web struct {
	p      *Provider
	cfg    WebConfig
	sealer *Sealer
	secure bool
	logger *slog.Logger
	client *http.Client
}

type session struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
}

type loginState struct {
	State    string `json:"state"`
	Nonce    string `json:"nonce"`
	Verifier string `json:"verifier"`
	Next     string `json:"next"`
}

func NewWeb(p *Provider, cfg WebConfig, sealer *Sealer, logger *slog.Logger) (*Web, error) {
	cfg.PublicURL = strings.TrimRight(cfg.PublicURL, "/")
	if p == nil || sealer == nil || cfg.ClientID == "" || cfg.PublicURL == "" {
		return nil, errors.New("auth: the web login needs a provider, a session key, a client ID and the public URL")
	}
	return &Web{
		p:      p,
		cfg:    cfg,
		sealer: sealer,
		secure: strings.HasPrefix(cfg.PublicURL, "https://"),
		logger: logger,
		client: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (w *Web) oauth(ctx context.Context) (*oauth2.Config, Metadata, error) {
	m, err := w.p.Metadata(ctx)
	if err != nil {
		return nil, Metadata{}, err
	}
	if m.AuthorizationEndpoint == "" {
		return nil, Metadata{}, errors.New("auth: the provider has no authorization endpoint")
	}
	return &oauth2.Config{
		ClientID:     w.cfg.ClientID,
		ClientSecret: w.cfg.ClientSecret,
		Endpoint:     oauth2.Endpoint{AuthURL: m.AuthorizationEndpoint, TokenURL: m.TokenEndpoint},
		RedirectURL:  w.cfg.PublicURL + CallbackPath,
		Scopes:       append([]string{"openid", "email", "profile"}, w.cfg.Scopes...),
	}, m, nil
}

// Session returns the identity in the request's session cookie, if any.
func (w *Web) Session(r *http.Request) (Identity, bool) {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return Identity{}, false
	}
	var s session
	if err := w.sealer.Open(SessionCookie, c.Value, &s); err != nil {
		return Identity{}, false
	}
	return Identity{Email: s.Email, Subject: s.Sub, ClientID: "web", Permission: PermWrite}, true
}

// Login sends the browser to the provider. GET /auth/login?next=/budget
func (w *Web) Login(rw http.ResponseWriter, r *http.Request) {
	cfg, _, err := w.oauth(r.Context())
	if err != nil {
		w.logger.ErrorContext(r.Context(), "web login: provider unavailable", "err", err)
		http.Error(rw, "The sign-in provider is unavailable. Try again in a minute.", http.StatusServiceUnavailable)
		return
	}
	st := loginState{State: random(), Nonce: random(), Verifier: oauth2.GenerateVerifier(), Next: safeNext(r.URL.Query().Get("next"))}
	v, err := w.sealer.Seal(loginCookie, st, loginTTL)
	if err != nil {
		w.logger.ErrorContext(r.Context(), "web login: seal state", "err", err)
		http.Error(rw, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(rw, w.cookie(loginCookie, v, "/auth/", loginTTL))
	http.Redirect(rw, r, cfg.AuthCodeURL(st.State,
		oauth2.S256ChallengeOption(st.Verifier),
		oauth2.SetAuthURLParam("nonce", st.Nonce),
	), http.StatusFound)
}

// Callback finishes the login: the state matches, the code is exchanged with
// the PKCE verifier and the ID token belongs to the owner. GET /auth/callback
func (w *Web) Callback(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fail := func(status int, msg string, err error) {
		w.logger.WarnContext(ctx, "web login failed", "reason", msg, "err", err)
		http.SetCookie(rw, w.cookie(loginCookie, "", "/auth/", -1))
		http.Error(rw, "Sign-in failed: "+msg+". Start again at "+LoginPath+".", status)
	}

	c, err := r.Cookie(loginCookie)
	if err != nil {
		fail(http.StatusBadRequest, "the sign-in expired or was started elsewhere", err)
		return
	}
	var st loginState
	if err := w.sealer.Open(loginCookie, c.Value, &st); err != nil {
		fail(http.StatusBadRequest, "the sign-in expired or was started elsewhere", err)
		return
	}
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		fail(http.StatusUnauthorized, "the provider said "+e, errors.New(q.Get("error_description")))
		return
	}
	if q.Get("state") == "" || q.Get("state") != st.State {
		fail(http.StatusBadRequest, "the sign-in does not match this browser", nil)
		return
	}

	cfg, _, err := w.oauth(ctx)
	if err != nil {
		fail(http.StatusServiceUnavailable, "the sign-in provider is unavailable", err)
		return
	}
	tok, err := cfg.Exchange(context.WithValue(ctx, oauth2.HTTPClient, w.client), q.Get("code"), oauth2.VerifierOption(st.Verifier))
	if err != nil {
		fail(http.StatusUnauthorized, "the code was refused", err)
		return
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		fail(http.StatusUnauthorized, "the provider sent no ID token", nil)
		return
	}
	id, err := w.p.verifyIDToken(ctx, raw, w.cfg.ClientID, st.Nonce)
	if errors.Is(err, ErrNotOwner) {
		fail(http.StatusForbidden, "this account is not the owner's", err)
		return
	}
	if err != nil {
		fail(http.StatusUnauthorized, "the ID token is not valid", err)
		return
	}

	v, err := w.sealer.Seal(SessionCookie, session{Sub: id.Subject, Email: id.Email}, sessionTTL)
	if err != nil {
		fail(http.StatusInternalServerError, "internal error", err)
		return
	}
	http.SetCookie(rw, w.cookie(loginCookie, "", "/auth/", -1))
	http.SetCookie(rw, w.cookie(SessionCookie, v, "/", sessionTTL))
	http.Redirect(rw, r, st.Next, http.StatusFound)
}

// Logout clears the session and ends the provider's too, when it can.
// POST /auth/logout
func (w *Web) Logout(rw http.ResponseWriter, r *http.Request) {
	http.SetCookie(rw, w.cookie(SessionCookie, "", "/", -1))
	to := "/"
	if m, err := w.p.Metadata(r.Context()); err == nil && m.EndSessionEndpoint != "" {
		q := url.Values{"client_id": {w.cfg.ClientID}, "post_logout_redirect_uri": {w.cfg.PublicURL + "/"}}
		to = m.EndSessionEndpoint + "?" + q.Encode()
	}
	http.Redirect(rw, r, to, http.StatusSeeOther)
}

func (w *Web) cookie(name, value, path string, ttl time.Duration) *http.Cookie {
	c := &http.Cookie{Name: name, Value: value, Path: path, HttpOnly: true, Secure: w.secure, SameSite: http.SameSiteLaxMode}
	if ttl < 0 {
		c.MaxAge = -1
	} else {
		c.MaxAge = int(ttl / time.Second)
	}
	return c
}

// safeNext keeps the redirect after login on this site: a path, never
// another host ("//evil.example" or "/\evil.example").
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	if u, err := url.Parse(next); err != nil || u.Host != "" || u.Scheme != "" {
		return "/"
	}
	return next
}

func random() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("auth: random: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
