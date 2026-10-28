package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandler(t *testing.T) {
	h := handler(fstest.MapFS{
		"index.html":          {Data: []byte("<!doctype html><title>lastro</title>")},
		"manifest.json":       {Data: []byte(`{"name":"lastro"}`)},
		"icons/icon-192.png":  {Data: []byte("png")},
		"assets/index-a1.js":  {Data: []byte("console.log(1)")},
		"assets/index-b2.css": {Data: []byte("body{}")},
	})

	tests := []struct {
		name        string
		path        string
		body        string
		contentType string
		cache       string
	}{
		{"the root is the app", "/", "<title>lastro</title>", "text/html", "no-cache"},
		{"a screen is the app", "/budget", "<title>lastro</title>", "text/html", "no-cache"},
		{"a dotfile is not served", "/.gitkeep", "<title>lastro</title>", "text/html", "no-cache"},
		{"a built script is kept for good, privately", "/assets/index-a1.js", "console.log(1)", "text/javascript", "private, max-age=31536000, immutable"},
		{"a built stylesheet too", "/assets/index-b2.css", "body{}", "text/css", "private, max-age=31536000, immutable"},
		{"the manifest is JSON and not kept", "/manifest.json", `"name":"lastro"`, "application/json", ""},
		{"an icon is not kept", "/icons/icon-192.png", "png", "image/png", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tt.body) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.body)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, tt.contentType) {
				t.Errorf("Content-Type = %q, want %q", ct, tt.contentType)
			}
			if cc := rec.Header().Get("Cache-Control"); cc != tt.cache {
				t.Errorf("Cache-Control = %q, want %q", cc, tt.cache)
			}
		})
	}
}
