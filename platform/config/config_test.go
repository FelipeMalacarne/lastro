package config

import (
	"log/slog"
	"strings"
	"testing"
)

func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func TestLoadDev(t *testing.T) {
	cfg, err := LoadFrom(env(map[string]string{
		"LASTRO_ENV":             "dev",
		"LASTRO_DATABASE_URL":    "postgres://x",
		"LASTRO_AUTH_DISABLED":   "true",
		"LASTRO_LOG_LEVEL":       "debug",
		"LASTRO_PLUGGY_ITEM_IDS": " a, b ,,c ",
	}))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !cfg.Auth.Disabled || cfg.LogLevel != slog.LevelDebug || cfg.HTTPAddr != ":8080" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if got := strings.Join(cfg.Pluggy.ItemIDs, "|"); got != "a|b|c" {
		t.Fatalf("ItemIDs = %q", got)
	}
}

func TestAuthCannotBeDisabledOutsideDev(t *testing.T) {
	_, err := LoadFrom(env(map[string]string{
		"LASTRO_DATABASE_URL":  "postgres://x",
		"LASTRO_AUTH_DISABLED": "true",
	}))
	if err == nil || !strings.Contains(err.Error(), "LASTRO_ENV=dev") {
		t.Fatalf("want an error about auth disabled outside dev, got %v", err)
	}
}

func TestProdRequiresAccessConfig(t *testing.T) {
	_, err := LoadFrom(env(map[string]string{"LASTRO_DATABASE_URL": "postgres://x"}))
	if err == nil || !strings.Contains(err.Error(), "LASTRO_CF_ACCESS_AUD") {
		t.Fatalf("want an error asking for Cloudflare Access config, got %v", err)
	}
}

func TestPluggyValidate(t *testing.T) {
	if err := (Pluggy{}).Validate(); err == nil {
		t.Fatal("want an error for empty Pluggy config")
	}
	if err := (Pluggy{ClientID: "id", ClientSecret: "s", ItemIDs: []string{"x"}}).Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
