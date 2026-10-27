package portfolio

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGroupEndpointValidatesFirst(t *testing.T) {
	mux := http.NewServeMux()
	New(nil, nil, nil).RegisterRoutes(mux, slog.New(slog.NewTextHandler(io.Discard, nil)))
	path := "/api/v1/position-groups/0123456789abcdef"
	tests := []struct{ body, want string }{
		{`{"reserve": "amount"}`, "emergency fund"},
		{`{"reserve": "amount", "reserveCents": -100}`, "emergency fund"},
		{`{"reserve": "half"}`, "emergency fund"},
		{`{"emergency": true}`, "a JSON object"},
		{`not json`, "a JSON object"},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, path, strings.NewReader(tt.body)))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tt.want) {
			t.Errorf("%s: got %d %s", tt.body, rec.Code, rec.Body.String())
		}
	}
}
