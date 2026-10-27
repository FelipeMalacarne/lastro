// Package client talks to a lastro server through its HTTP API (ADR-0010).
//
// Its wire types are its own, written for the endpoints it calls: it never
// imports the server's modules. An integration test runs it against the real
// handler, so a renamed field fails there.
package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
)

// ErrNotConnected is returned before `lastro connect`.
var ErrNotConnected = errors.New("not connected: run lastro connect <url>")

// Config is what `lastro connect` keeps: the server and, when it asks for
// one, how to keep a token fresh.
type Config struct {
	URL  string `json:"url"`
	Auth *Auth  `json:"auth,omitempty"`
}

// Auth is the OAuth client and its token. Without it the server takes
// requests as they are (a dev server with auth off).
type Auth struct {
	Issuer   string        `json:"issuer"`
	ClientID string        `json:"clientId"`
	TokenURL string        `json:"tokenUrl"`
	Scopes   []string      `json:"scopes"`
	Token    *oauth2.Token `json:"token"`
}

// Path is where the config lives: ~/.config/lastro/config.json on Linux.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find the config directory: %w", err)
	}
	return filepath.Join(dir, "lastro", "config.json"), nil
}

// Load reads the config, or returns ErrNotConnected.
func Load() (Config, error) {
	p, err := Path()
	if err != nil {
		return Config{}, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, ErrNotConnected
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("read %s: %w", p, err)
	}
	if c.URL == "" {
		return Config{}, ErrNotConnected
	}
	return c, nil
}

// Save writes the config readable only by me: it holds a refresh token.
func Save(c Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Remove forgets the server and its token.
func Remove() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
