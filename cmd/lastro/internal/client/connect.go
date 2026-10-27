package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// resourceMetadata is what the server says about signing in (RFC 9728).
type resourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
	CLIClientID          string   `json:"lastro_cli_client_id"`
}

type providerMetadata struct {
	Issuer                      string `json:"issuer"`
	TokenEndpoint               string `json:"token_endpoint"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
}

// Device is a sign-in waiting for me in the browser.
type Device struct {
	URL      string // open this…
	UserCode string // …and check this code matches
	resp     *oauth2.DeviceAuthResponse
	cfg      *oauth2.Config
	auth     Auth
}

// Login is `lastro connect` in two steps: Start finds out whether the server
// needs a sign-in and starts one; Finish waits for it.
type Login struct {
	URL    string
	HTTP   *http.Client
	Device *Device // nil: the server needs no sign-in
}

// NormalizeURL checks a server URL. Tokens only travel over HTTPS, except to
// this machine.
func NormalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", fmt.Errorf("%q is not a server URL, like https://lastro.example", raw)
	}
	if u.Scheme == "http" && !loopback(u.Hostname()) {
		return "", fmt.Errorf("%s: use https; plain http only reaches this machine", raw)
	}
	u.Path, u.RawQuery, u.Fragment = strings.TrimRight(u.Path, "/"), "", ""
	return u.String(), nil
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// StartLogin reads the server's metadata and, if it needs a sign-in, starts
// the device flow (RFC 8628) with the provider it names.
func StartLogin(ctx context.Context, serverURL string) (*Login, error) {
	base, err := NormalizeURL(serverURL)
	if err != nil {
		return nil, err
	}
	hc := &http.Client{Timeout: 15 * time.Second}
	l := &Login{URL: base, HTTP: hc}

	var rm resourceMetadata
	found, err := getJSON(ctx, hc, base+"/.well-known/oauth-protected-resource", &rm)
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", base, err)
	}
	if !found {
		// No metadata: either auth is off (dev), or this is not lastro.
		var me map[string]any
		ok, err := getJSON(ctx, hc, base+"/api/v1/me", &me)
		if err != nil || !ok {
			return nil, fmt.Errorf("%s does not look like a lastro server (no sign-in metadata, and /api/v1/me did not answer)", base)
		}
		return l, nil
	}
	if len(rm.AuthorizationServers) == 0 || rm.CLIClientID == "" {
		return nil, errors.New("the server names no sign-in provider or CLI client: set LASTRO_OIDC_CLI_CLIENT_ID on it")
	}
	issuer := strings.TrimRight(rm.AuthorizationServers[0], "/")
	if !strings.HasPrefix(issuer, "https://") && !loopback(hostOf(issuer)) {
		return nil, fmt.Errorf("the sign-in provider %s is not on https", issuer)
	}
	var pm providerMetadata
	switch ok, err := getJSON(ctx, hc, issuer+"/.well-known/openid-configuration", &pm); {
	case err != nil:
		return nil, fmt.Errorf("read the provider %s: %w", issuer, err)
	case !ok:
		return nil, fmt.Errorf("the provider %s has no discovery document", issuer)
	}
	if strings.TrimRight(pm.Issuer, "/") != issuer {
		return nil, fmt.Errorf("the provider at %s says it is %s", issuer, pm.Issuer)
	}
	if pm.DeviceAuthorizationEndpoint == "" {
		return nil, fmt.Errorf("the provider %s does not offer the device flow", issuer)
	}

	scopes := append([]string{"openid", "email", "offline_access"}, rm.ScopesSupported...)
	cfg := &oauth2.Config{
		ClientID: rm.CLIClientID,
		Endpoint: oauth2.Endpoint{DeviceAuthURL: pm.DeviceAuthorizationEndpoint, TokenURL: pm.TokenEndpoint, AuthStyle: oauth2.AuthStyleInParams},
		Scopes:   scopes,
	}
	resp, err := cfg.DeviceAuth(context.WithValue(ctx, oauth2.HTTPClient, hc))
	if err != nil {
		return nil, fmt.Errorf("start the sign-in: %w", err)
	}
	open := resp.VerificationURIComplete
	if open == "" {
		open = resp.VerificationURI
	}
	l.Device = &Device{URL: open, UserCode: resp.UserCode, resp: resp, cfg: cfg,
		auth: Auth{Issuer: issuer, ClientID: rm.CLIClientID, TokenURL: pm.TokenEndpoint, Scopes: scopes}}
	return l, nil
}

// Finish waits for the sign-in in the browser and returns the config to save.
func (l *Login) Finish(ctx context.Context) (Config, error) {
	c := Config{URL: l.URL}
	if l.Device == nil {
		return c, nil
	}
	tok, err := l.Device.cfg.DeviceAccessToken(context.WithValue(ctx, oauth2.HTTPClient, l.HTTP), l.Device.resp)
	if err != nil {
		return Config{}, fmt.Errorf("sign-in: %w", err)
	}
	a := l.Device.auth
	a.Token = tok
	c.Auth = &a
	return c, nil
}

// getJSON reads url into out; a 404 is not an error, it returns false.
func getJSON(ctx context.Context, hc *http.Client, url string, out any) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnauthorized {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	return true, json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
