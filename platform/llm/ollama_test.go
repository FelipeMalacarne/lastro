package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req generateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if r.URL.Path != "/api/generate" || req.Model != "modelo" || req.Format != "json" || req.Stream {
			t.Errorf("unexpected request: %s %+v", r.URL.Path, req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"response": `{"category":"groceries"}`, "done": true})
	}))
	defer srv.Close()

	var out struct {
		Category string `json:"category"`
	}
	if err := NewOllama(srv.URL+"/", "modelo").GenerateJSON(context.Background(), "sys", "prompt", &out); err != nil {
		t.Fatalf("error: %v", err)
	}
	if out.Category != "groceries" {
		t.Fatalf("category = %q", out.Category)
	}
}

func TestDisabled(t *testing.T) {
	if _, err := NewOllama("", "").Generate(context.Background(), "", "hi"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("want ErrDisabled, got %v", err)
	}
}

func TestPing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]string{{"name": "llama3.1:latest"}, {"name": "qwen2.5:7b"}}})
	}))
	defer srv.Close()

	tests := []struct {
		model string
		want  error
	}{
		{"llama3.1", nil},
		{"qwen2.5:7b", nil},
		{"mistral", ErrModelMissing},
	}
	for _, tt := range tests {
		if err := NewOllama(srv.URL, tt.model).Ping(context.Background()); !errors.Is(err, tt.want) {
			t.Errorf("Ping(%s) = %v, want %v", tt.model, err, tt.want)
		}
	}
	if err := NewOllama("", "").Ping(context.Background()); !errors.Is(err, ErrDisabled) {
		t.Errorf("disabled: %v", err)
	}
}
