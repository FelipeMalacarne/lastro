// Package config loads the app configuration from environment variables.
//
// Every variable uses the LASTRO_ prefix. See .env.example for the full list.
package config

import (
	"cmp"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Env         string // "dev" or "prod"
	HTTPAddr    string
	DatabaseURL string
	LogLevel    slog.Level
	Auth        Auth
	Ollama      Ollama
	Pluggy      Pluggy
	// BrapiToken reads B3 quotes from brapi.dev; without one only a few
	// tickers answer.
	BrapiToken string
	// Telegram pushes alerts (ADR-0007); off unless both are set.
	Telegram Telegram
}

// Telegram is a bot token and the chat it writes to.
type Telegram struct {
	BotToken string
	ChatID   string
}

// Auth configures who gets in (ADR-0008): the OIDC provider, or nobody
// checked in dev.
type Auth struct {
	Disabled bool
	// PublicURL is how lastro is reached, e.g. "https://lastro.ftm.dev.br".
	PublicURL string
	OIDC      OIDC
	// SessionKey seals the web session cookie: 32 bytes, base64. Rotating it
	// signs every session out.
	SessionKey []byte
}

// OIDC is the identity provider and lastro's clients in it.
type OIDC struct {
	Issuer string
	// Audience is what access tokens carry in aud; in Zitadel, the project ID.
	Audience     string
	OwnerSubject string
	// Scopes are the ones clients ask for beyond openid, e.g. Zitadel's
	// project audience scope.
	Scopes []string
	// WebClientID and WebClientSecret are the web app's (the BFF login). Empty
	// turns the web login off.
	WebClientID     string
	WebClientSecret string
	// CLIClientID is the public client of `lastro connect`; it writes.
	CLIClientID string
	// WriteClients may write too. Every other client, MCP among them, reads.
	WriteClients []string
}

func (o OIDC) Enabled() bool { return o.Issuer != "" }

// Writers are the clients that may write: the CLI and WriteClients.
func (o OIDC) Writers() []string {
	if o.CLIClientID == "" {
		return o.WriteClients
	}
	return append([]string{o.CLIClientID}, o.WriteClients...)
}

// Ollama configures the local LLM. An empty URL or model disables it.
type Ollama struct {
	URL   string
	Model string
}

// Pluggy holds the Dashboard credentials and the Meu Pluggy item IDs.
type Pluggy struct {
	ClientID     string
	ClientSecret string
	ItemIDs      []string
}

func (c Config) IsDev() bool { return c.Env == "dev" }

// Validate checks that Pluggy is configured. Only sync needs it.
func (p Pluggy) Validate() error {
	var errs []error
	if p.ClientID == "" || p.ClientSecret == "" {
		errs = append(errs, errors.New("set LASTRO_PLUGGY_CLIENT_ID and LASTRO_PLUGGY_CLIENT_SECRET"))
	}
	if len(p.ItemIDs) == 0 {
		errs = append(errs, errors.New("set LASTRO_PLUGGY_ITEM_IDS (the Meu Pluggy API does not list items)"))
	}
	if len(errs) > 0 {
		return fmt.Errorf("config: %w", errors.Join(errs...))
	}
	return nil
}

// LoadPluggy reads and validates only the Pluggy settings, for commands that
// talk to Pluggy without a database (lastrod fixtures).
func LoadPluggy() (Pluggy, error) {
	p := pluggyFrom(func(key, def string) string { return cmp.Or(strings.TrimSpace(os.Getenv(key)), def) })
	return p, p.Validate()
}

func pluggyFrom(get func(key, def string) string) Pluggy {
	return Pluggy{
		ClientID:     get("LASTRO_PLUGGY_CLIENT_ID", ""),
		ClientSecret: get("LASTRO_PLUGGY_CLIENT_SECRET", ""),
		ItemIDs:      splitList(get("LASTRO_PLUGGY_ITEM_IDS", "")),
	}
}

// Load reads the configuration from the process environment.
func Load() (Config, error) { return LoadFrom(os.Getenv) }

// LoadFrom reads the configuration from a lookup function. Useful in tests.
func LoadFrom(getenv func(string) string) (Config, error) {
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}

	cfg := Config{
		Env:         get("LASTRO_ENV", "prod"),
		HTTPAddr:    get("LASTRO_HTTP_ADDR", ":8080"),
		DatabaseURL: get("LASTRO_DATABASE_URL", ""),
		Auth: Auth{
			PublicURL: strings.TrimRight(get("LASTRO_PUBLIC_URL", ""), "/"),
			OIDC: OIDC{
				Issuer:          strings.TrimRight(get("LASTRO_OIDC_ISSUER", ""), "/"),
				Audience:        get("LASTRO_OIDC_AUDIENCE", ""),
				OwnerSubject:    get("LASTRO_OIDC_OWNER_SUBJECT", ""),
				Scopes:          strings.Fields(get("LASTRO_OIDC_SCOPES", "")),
				WebClientID:     get("LASTRO_OIDC_WEB_CLIENT_ID", ""),
				WebClientSecret: get("LASTRO_OIDC_WEB_CLIENT_SECRET", ""),
				CLIClientID:     get("LASTRO_OIDC_CLI_CLIENT_ID", ""),
				WriteClients:    splitList(get("LASTRO_OIDC_WRITE_CLIENTS", "")),
			},
		},
		Ollama: Ollama{
			URL:   get("LASTRO_OLLAMA_URL", ""),
			Model: get("LASTRO_OLLAMA_MODEL", ""),
		},
		Pluggy:     pluggyFrom(get),
		BrapiToken: get("LASTRO_BRAPI_TOKEN", ""),
		Telegram:   Telegram{BotToken: get("LASTRO_TELEGRAM_BOT_TOKEN", ""), ChatID: get("LASTRO_TELEGRAM_CHAT_ID", "")},
	}

	var errs []error
	if cfg.Env != "dev" && cfg.Env != "prod" {
		errs = append(errs, fmt.Errorf("LASTRO_ENV must be dev or prod, got %q", cfg.Env))
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(get("LASTRO_LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("invalid LASTRO_LOG_LEVEL: %w", err))
	}
	disabled, err := strconv.ParseBool(get("LASTRO_AUTH_DISABLED", "false"))
	if err != nil {
		errs = append(errs, fmt.Errorf("invalid LASTRO_AUTH_DISABLED: %w", err))
	}
	cfg.Auth.Disabled = disabled

	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("LASTRO_DATABASE_URL is required"))
	}
	if cfg.Auth.Disabled && !cfg.IsDev() {
		errs = append(errs, errors.New("LASTRO_AUTH_DISABLED is only allowed with LASTRO_ENV=dev"))
	}
	if !cfg.Auth.Disabled {
		errs = append(errs, cfg.Auth.validate(get("LASTRO_SESSION_KEY", ""))...)
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("config: %w", errors.Join(errs...))
	}
	return cfg, nil
}

// validate checks the provider is configured and complete. It also decodes
// the session key.
func (a *Auth) validate(sessionKey string) []error {
	if !a.OIDC.Enabled() {
		return []error{errors.New("set LASTRO_OIDC_ISSUER, or LASTRO_AUTH_DISABLED=true in dev")}
	}
	var errs []error
	if a.OIDC.Audience == "" || a.OIDC.OwnerSubject == "" {
		errs = append(errs, errors.New("with LASTRO_OIDC_ISSUER, set LASTRO_OIDC_AUDIENCE and LASTRO_OIDC_OWNER_SUBJECT"))
	}
	if !strings.HasPrefix(a.PublicURL, "https://") && !strings.HasPrefix(a.PublicURL, "http://") {
		errs = append(errs, errors.New("with LASTRO_OIDC_ISSUER, set LASTRO_PUBLIC_URL (e.g. https://lastro.ftm.dev.br)"))
	}
	if a.OIDC.WebClientID != "" {
		key, err := base64.StdEncoding.DecodeString(sessionKey)
		if err != nil || len(key) != 32 {
			errs = append(errs, errors.New("with LASTRO_OIDC_WEB_CLIENT_ID, set LASTRO_SESSION_KEY to 32 random bytes in base64 (openssl rand -base64 32)"))
		}
		a.SessionKey = key
	}
	return errs
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
