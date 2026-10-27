// Package config loads the app configuration from environment variables.
//
// Every variable uses the LASTRO_ prefix. See .env.example for the full list.
package config

import (
	"cmp"
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

// Auth configures Cloudflare Access verification.
type Auth struct {
	Disabled   bool
	TeamDomain string // e.g. "your-team.cloudflareaccess.com"
	Audience   string // AUD tag of the Access application
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
// talk to Pluggy without a database (lastro fixtures).
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
			TeamDomain: get("LASTRO_CF_ACCESS_TEAM_DOMAIN", ""),
			Audience:   get("LASTRO_CF_ACCESS_AUD", ""),
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
	if !cfg.Auth.Disabled && (cfg.Auth.TeamDomain == "" || cfg.Auth.Audience == "") {
		errs = append(errs, errors.New("set LASTRO_CF_ACCESS_TEAM_DOMAIN and LASTRO_CF_ACCESS_AUD, or LASTRO_AUTH_DISABLED=true in dev"))
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("config: %w", errors.Join(errs...))
	}
	return cfg, nil
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
