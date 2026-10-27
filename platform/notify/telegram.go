// Package notify pushes messages to me outside the app. Telegram is the only
// channel (ADR-0007), and it is off until configured.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrDisabled means no channel is configured.
var ErrDisabled = errors.New("notify: no channel (set LASTRO_TELEGRAM_BOT_TOKEN and LASTRO_TELEGRAM_CHAT_ID)")

// Telegram sends through a bot to one chat.
type Telegram struct {
	base  string
	token string
	chat  string
	http  *http.Client
}

const telegramURL = "https://api.telegram.org"

// NewTelegram returns a sender; without a token or chat it is disabled.
func NewTelegram(token, chat string) *Telegram {
	return &Telegram{base: telegramURL, token: token, chat: chat, http: &http.Client{Timeout: 15 * time.Second}}
}

func (t *Telegram) Enabled() bool { return t != nil && t.token != "" && t.chat != "" }

// Send posts a plain-text message.
func (t *Telegram) Send(ctx context.Context, text string) error {
	if !t.Enabled() {
		return ErrDisabled
	}
	body, err := json.Marshal(map[string]any{"chat_id": t.chat, "text": text, "disable_web_page_preview": true})
	if err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.base+"/bot"+t.token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return errors.New("notify: telegram: bad request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.http.Do(req)
	if err != nil {
		// The URL holds the token: keep it out of the error.
		return errors.New("notify: telegram: request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("notify: telegram: status %d: %s", resp.StatusCode, strings.ReplaceAll(strings.TrimSpace(string(msg)), t.token, "***"))
	}
	return nil
}
