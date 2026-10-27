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

func TestOIDC(t *testing.T) {
	base := map[string]string{
		"LASTRO_DATABASE_URL":       "postgres://x",
		"LASTRO_PUBLIC_URL":         "https://lastro.example/",
		"LASTRO_OIDC_ISSUER":        "https://auth.example/",
		"LASTRO_OIDC_AUDIENCE":      "project-1",
		"LASTRO_OIDC_OWNER_SUBJECT": "owner-1",
		"LASTRO_OIDC_SCOPES":        "urn:zitadel:iam:org:project:id:project-1:aud offline_access",
		"LASTRO_OIDC_CLI_CLIENT_ID": "cli",
		"LASTRO_OIDC_WRITE_CLIENTS": "app",
	}
	cfg, err := LoadFrom(env(base))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	a := cfg.Auth
	if a.PublicURL != "https://lastro.example" || a.OIDC.Issuer != "https://auth.example" || len(a.OIDC.Scopes) != 2 {
		t.Fatalf("unexpected auth config: %+v", a)
	}
	if got := strings.Join(a.OIDC.Writers(), "|"); got != "cli|app" {
		t.Fatalf("Writers = %q", got)
	}

	tests := []struct {
		name string
		kv   map[string]string
		want string
	}{
		{"no owner", map[string]string{"LASTRO_OIDC_OWNER_SUBJECT": ""}, "LASTRO_OIDC_OWNER_SUBJECT"},
		{"no public URL", map[string]string{"LASTRO_PUBLIC_URL": ""}, "LASTRO_PUBLIC_URL"},
		{"web login without a session key", map[string]string{"LASTRO_OIDC_WEB_CLIENT_ID": "web"}, "LASTRO_SESSION_KEY"},
		{"short session key", map[string]string{"LASTRO_OIDC_WEB_CLIENT_ID": "web", "LASTRO_SESSION_KEY": "c2hvcnQ="}, "LASTRO_SESSION_KEY"},
		{"half of Access", map[string]string{"LASTRO_CF_ACCESS_AUD": "aud"}, "LASTRO_CF_ACCESS_TEAM_DOMAIN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv := map[string]string{}
			for k, v := range base {
				kv[k] = v
			}
			for k, v := range tt.kv {
				kv[k] = v
			}
			if _, err := LoadFrom(env(kv)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want an error naming %s, got %v", tt.want, err)
			}
		})
	}

	kv := map[string]string{"LASTRO_OIDC_WEB_CLIENT_ID": "web", "LASTRO_SESSION_KEY": "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="}
	for k, v := range base {
		kv[k] = v
	}
	if cfg, err := LoadFrom(env(kv)); err != nil || len(cfg.Auth.SessionKey) != 32 {
		t.Fatalf("session key: %d bytes, %v", len(cfg.Auth.SessionKey), err)
	}
}
