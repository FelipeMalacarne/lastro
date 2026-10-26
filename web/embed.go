// Package web serves the frontend (React + Vite) embedded in the binary.
//
// The embed lives here, next to dist, because go:embed does not accept paths
// with "..". `npm run build` produces dist and the Go build embeds it.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the frontend files. Paths that are not files (SPA routes
// such as /budget) get index.html.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // impossible: the embed pattern guarantees the dist directory
	}
	files := http.FileServerFS(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" || p == "." {
			serveIndex(w, sub)
			return
		}
		if strings.HasPrefix(path.Base(p), ".") {
			serveIndex(w, sub)
			return
		}
		if _, err := fs.Stat(sub, p); err != nil {
			serveIndex(w, sub)
			return
		}
		files.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, sub fs.FS) {
	b, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		http.Error(w, "frontend not built: run `task web:build`", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}
