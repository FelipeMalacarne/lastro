package budget

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLineEndpointsValidateFirst(t *testing.T) {
	mux := http.NewServeMux()
	New(nil, nil, nil).RegisterRoutes(mux, slog.New(slog.NewTextHandler(io.Discard, nil)))
	many := `"` + strings.Repeat(`a", "`, 50) + `a"`
	tests := []struct{ method, path, body, want string }{
		{http.MethodPost, "/api/v1/budget/2026-10/lines", `{"name": "Free", "limitCents": 100000}`, "a category"},
		{http.MethodPost, "/api/v1/budget/2026-10/lines", `{"name": "Free", "categories": ["a", "b"]}`, "limitCents"},
		{http.MethodPost, "/api/v1/budget/2026-10/lines", `{"name": "Free", "categories": [` + many + `], "limitCents": 1}`, "up to 50"},
		{http.MethodPost, "/api/v1/budget/2026-10/lines", `{"name": "Free", "categories": ["a"], "limitCents": 1, "fund": true}`, "a JSON object"},
		{http.MethodPost, "/api/v1/budget/october/lines", `{}`, "YYYY-MM"},
		{http.MethodPut, "/api/v1/budget/2026-10/lines/0b7e4c1d-8a9f-4e2b-b3c4-d5e6f7a8b9c0", `{"categories": [""], "limitCents": 1}`, "a category"},
		{http.MethodPut, "/api/v1/budget/2026-10/lines/x", `{"categories": ["a"], "limitCents": 1}`, "no line id"},
		{http.MethodPut, "/api/v1/budget/2026-10/limits", `{"limits": [{"id": "x", "category": "a", "limitCents": 1}]}`, "UUID"},
		{http.MethodPut, "/api/v1/budget/2026-10/limits", `{"limits": [{"name": "Free", "limitCents": 1}]}`, "a category"},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tt.want) {
			t.Errorf("%s %s %s: got %d %s", tt.method, tt.path, tt.body, rec.Code, rec.Body.String())
		}
	}
}
