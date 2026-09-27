package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed dist
var assets embed.FS

// FS exposes the shared frontend build to the service and desktop host.
func FS() fs.FS {
	root, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	return root
}

func Handler() http.Handler {
	root := FS()
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if requested != "." && requested != "" {
			if _, err := fs.Stat(root, requested); err == nil {
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(root, "index.html")
		if err != nil {
			http.Error(w, "Web UI is unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
}
