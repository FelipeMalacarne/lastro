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
