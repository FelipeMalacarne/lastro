package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrNotOwner is a valid token for someone other than the owner: the provider
// serves other people and other apps, and none of them get into lastro.
var ErrNotOwner = errors.New("auth: not the owner")

// OIDCConfig is the provider lastro trusts and who may use it.
type OIDCConfig struct {
	Issuer string
	// Audience must be in an access token's aud: Zitadel puts the project ID there.
	Audience string
	// OwnerSubject is the owner's sub; nobody else gets in.
	OwnerSubject string
	// WriteClients may write. Every other client only reads.
	WriteClients []string
}

// Metadata is what lastro reads from the provider's discovery document.
type Metadata struct {
	Issuer                      string `json:"issuer"`
	AuthorizationEndpoint       string `json:"authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
	JWKSURI                     string `json:"jwks_uri"`
	EndSessionEndpoint          string `json:"end_session_endpoint"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
}

// Provider verifies tokens from one OIDC provider. Discovery happens on first
// use, not at startup, so lastro starts even while the provider is down.
type Provider struct {
	cfg    OIDCConfig
	client *http.Client
	now    func() time.Time

	mu          sync.Mutex
	meta        Metadata
	fetchedAt   time.Time
	lastAttempt time.Time
	keys        *keySet
}

func NewProvider(cfg OIDCConfig) (*Provider, error) {
	cfg.Issuer = strings.TrimRight(strings.TrimSpace(cfg.Issuer), "/")
	if cfg.Issuer == "" || cfg.Audience == "" || cfg.OwnerSubject == "" {
		return nil, errors.New("auth: issuer, audience and owner subject are required")
	}
	return &Provider{cfg: cfg, client: &http.Client{Timeout: 5 * time.Second}, now: time.Now}, nil
}

// Issuer is the provider's issuer URL, as tokens carry it.
func (p *Provider) Issuer() string { return p.cfg.Issuer }

// Metadata returns the discovery document, cached for an hour. A failed
// refresh keeps the last good one; refreshes are limited to one per minute.
func (p *Provider) Metadata(ctx context.Context) (Metadata, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.metadataLocked(ctx)
}

func (p *Provider) metadataLocked(ctx context.Context) (Metadata, error) {
	now := p.now()
	known := p.meta.Issuer != ""
	if known && now.Sub(p.fetchedAt) < keyTTL {
		return p.meta, nil
	}
	if now.Sub(p.lastAttempt) < minRefresh {
		if known {
			return p.meta, nil
		}
		return Metadata{}, errors.New("auth: provider discovery failed recently")
	}
	p.lastAttempt = now
	m, err := p.discover(ctx)
	if err != nil {
		if known {
			return p.meta, nil
		}
		return Metadata{}, err
	}
	p.meta, p.fetchedAt = m, now
	if p.keys == nil || p.keys.url != m.JWKSURI {
		p.keys = newKeySet(m.JWKSURI, p.client)
		p.keys.now = p.now
	}
	return m, nil
}

func (p *Provider) discover(ctx context.Context) (Metadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.Issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return Metadata{}, fmt.Errorf("auth: discovery: %w", err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return Metadata{}, fmt.Errorf("auth: discovery: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Metadata{}, fmt.Errorf("auth: discovery: status %d", resp.StatusCode)
	}
	var m Metadata
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&m); err != nil {
		return Metadata{}, fmt.Errorf("auth: discovery: %w", err)
	}
	// The document must speak for the configured issuer (OIDC Discovery §4.3).
	if strings.TrimRight(m.Issuer, "/") != p.cfg.Issuer {
		return Metadata{}, fmt.Errorf("auth: discovery: issuer %q, want %q", m.Issuer, p.cfg.Issuer)
	}
	if m.JWKSURI == "" || m.TokenEndpoint == "" {
		return Metadata{}, errors.New("auth: discovery: jwks_uri and token_endpoint are required")
	}
	return m, nil
}

// parse verifies the signature and the issuer, the checks every token shares.
func (p *Provider) parse(ctx context.Context, token string) (tokenClaims, error) {
	p.mu.Lock()
	_, err := p.metadataLocked(ctx)
	keys := p.keys
	p.mu.Unlock()
	if err != nil {
		return tokenClaims{}, err
	}
	c, err := parseToken(ctx, keys, token)
	if err != nil {
		return tokenClaims{}, err
	}
	if strings.TrimRight(c.Iss, "/") != p.cfg.Issuer {
		return tokenClaims{}, fmt.Errorf("%w: issuer %q", ErrInvalidToken, c.Iss)
	}
	if err := c.checkTime(p.now()); err != nil {
		return tokenClaims{}, err
	}
	if c.Sub != p.cfg.OwnerSubject {
		return tokenClaims{}, ErrNotOwner
	}
	return c, nil
}

// VerifyAccessToken checks a bearer token: signature, issuer, the audience
// lastro is known by, the validity window and the owner. The permission
// comes from the client it was issued to.
func (p *Provider) VerifyAccessToken(ctx context.Context, token string) (Identity, error) {
	c, err := p.parse(ctx, token)
	if err != nil {
		return Identity{}, err
	}
	if !contains(c.Aud, p.cfg.Audience) {
		return Identity{}, fmt.Errorf("%w: audience", ErrInvalidToken)
	}
	perm := PermRead
	if contains(p.cfg.WriteClients, c.client()) {
		perm = PermWrite
	}
	return Identity{Email: c.Email, Subject: c.Sub, ClientID: c.client(), Permission: perm}, nil
}

// verifyIDToken checks the ID token the web login gets back: issued to
// clientID, for this login's nonce, to the owner.
func (p *Provider) verifyIDToken(ctx context.Context, token, clientID, nonce string) (Identity, error) {
	c, err := p.parse(ctx, token)
	if err != nil {
		return Identity{}, err
	}
	switch {
	case !contains(c.Aud, clientID):
		return Identity{}, fmt.Errorf("%w: audience", ErrInvalidToken)
	case c.Nonce == "" || c.Nonce != nonce:
		return Identity{}, fmt.Errorf("%w: nonce", ErrInvalidToken)
	}
	return Identity{Email: c.Email, Subject: c.Sub, ClientID: clientID, Permission: PermWrite}, nil
}
