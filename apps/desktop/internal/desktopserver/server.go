// Package desktopserver serves the shared UI over Wails' in-process asset
// transport. It opens no TCP listener and contains no gateway business logic.
package desktopserver

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

type Server struct {
	assets  http.Handler
	api     http.Handler
	desktop http.Handler
	index   []byte
	secret  string
}

func New(assets fs.FS, api http.Handler, desktop http.Handler) (*Server, error) {
	index, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		return nil, err
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	secret := hex.EncodeToString(nonce[:])
	index = bytes.Replace(index, []byte("</head>"), []byte(`<meta name="opensurge-desktop-session" content="`+secret+`"><script type="module" src="/wails/runtime.js"></script></head>`), 1)
	return &Server{assets: http.FileServer(http.FS(assets)), api: api, desktop: desktop, index: index, secret: secret}, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-src 'none'; frame-ancestors 'none'; form-action 'none'; base-uri 'none'")
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/desktop/") {
		proof := r.Header.Get("X-OpenSurge-Desktop")
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/events" {
			proof = r.URL.Query().Get("desktop_session")
		}
		origin := r.Header.Get("Origin")
		if subtle.ConstantTimeCompare([]byte(proof), []byte(s.secret)) != 1 ||
			(origin != "" && origin != "null" && origin != "wails://localhost") {
			http.Error(w, "desktop request rejected", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/desktop/") {
			if s.desktop == nil {
				http.NotFound(w, r)
				return
			}
			s.desktop.ServeHTTP(w, r)
		} else {
			s.api.ServeHTTP(w, r)
		}
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if path.Ext(r.URL.Path) != "" {
		s.assets.ServeHTTP(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodHead {
		_, _ = w.Write(s.index)
	}
}
