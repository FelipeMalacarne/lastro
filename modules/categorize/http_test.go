package categorize

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lastro/platform/httpx"
)

func TestCategoryEndpointsValidateFirst(t *testing.T) {
	mux := http.NewServeMux()
	New(nil, nil, nil).RegisterRoutes(mux, slog.New(slog.NewTextHandler(io.Discard, nil)))
	id := "/api/v1/categories/6f1c2d3e-4a5b-4c6d-8e7f-901a2b3c4d5e"
	tests := []struct {
		method, path, body string
		want               string
	}{
		{http.MethodPost, "/api/v1/categories", `{"name": "Pets"}`, "name and color are required"},
		{http.MethodPost, "/api/v1/categories", `{"name": "Pets", "color": "purple"}`, "colour"},
		{http.MethodPost, "/api/v1/categories", `{"name": "  ", "color": "note-5"}`, "category name"},
		{http.MethodPost, "/api/v1/categories", `{"name": "Pets", "color": "note-5", "icon": "dog"}`, "a JSON object"},
		{http.MethodPost, "/api/v1/categories", `{"name": "Pets", "color": "note-5"} {}`, "a single JSON object"},
		{http.MethodPost, "/api/v1/categories", `not json`, "a JSON object"},
		{http.MethodPatch, "/api/v1/categories/42", `{}`, "id must be a category id"},
		{http.MethodPatch, id, `{"color": 5}`, "a JSON object"},
		{http.MethodDelete, "/api/v1/categories/groceries", ``, "id must be a category id"},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tt.want) {
			t.Errorf("%s %s %s: got %d %s", tt.method, tt.path, tt.body, rec.Code, rec.Body.String())
		}
	}
}

func TestAPIError(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{ErrInvalid, http.StatusBadRequest},
		{ErrNotFound, http.StatusNotFound},
		{ErrNameTaken, http.StatusConflict},
		{ErrArchived, http.StatusConflict},
	}
	for _, tt := range tests {
		var he *httpx.Error
		if !errors.As(apiError(tt.err), &he) || he.Status != tt.want {
			t.Errorf("%v: got %v", tt.err, apiError(tt.err))
		}
	}
	other := errors.New("boom")
	var he *httpx.Error
	if err := apiError(other); !errors.Is(err, other) || errors.As(err, &he) {
		t.Error("anything else stays an internal error")
	}
}
