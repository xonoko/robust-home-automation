// Package webui serves the web UI built from web/ (`make web`), embedded in the binary.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/xonoko/robust-home-automation/internal/api"
)

// dist is filled by the web build (vite outDir). Only .gitkeep is committed.
//
//go:embed all:dist
var dist embed.FS

// Handler serves static files and falls back to index.html for client-side
// routes. Unknown API paths get a 404 instead of the UI.
func Handler() http.Handler {
	root, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // the embed directive guarantees the directory exists
	}
	return handler(root)
}

func handler(root fs.FS) http.Handler {
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if api.IsAPIPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" {
			if _, err := fs.Stat(root, name); err == nil {
				files.ServeHTTP(w, r)
				return
			}
		}
		// Client-side route or root: serve index.html.
		if _, err := fs.Stat(root, "index.html"); err != nil {
			http.Error(w, "web UI not built: run `make web`", http.StatusNotFound)
			return
		}
		r = r.Clone(r.Context())
		r.URL.Path = "/"
		files.ServeHTTP(w, r)
	})
}
