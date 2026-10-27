package notify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTelegram(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/botbad/sendMessage" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Unauthorized for bad"}`))
			return
		}
		if r.URL.Path != "/botsecret/sendMessage" {
			t.Errorf("path %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	tg := NewTelegram("secret", "42")
	tg.base = srv.URL
	if err := tg.Send(context.Background(), "Restaurants is over its limit"); err != nil {
		t.Fatal(err)
	}
	if got["chat_id"] != "42" || got["text"] != "Restaurants is over its limit" {
		t.Errorf("sent %v", got)
	}

	bad := NewTelegram("bad", "42")
	bad.base = srv.URL
	if err := bad.Send(context.Background(), "x"); err == nil || strings.Contains(err.Error(), "bad") {
		t.Errorf("the token never shows in an error: %v", err)
	}
	if err := NewTelegram("", "").Send(context.Background(), "x"); !errors.Is(err, ErrDisabled) {
		t.Errorf("disabled: %v", err)
	}
}
