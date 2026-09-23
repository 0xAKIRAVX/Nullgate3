package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"nullgate/api/internal/webui"
)

// uiHandler serves the embedded static panel UI with SPA fallback.
//
// It is registered as the "GET /" catch-all on the same mux as the API, so
// every explicit route (/api/*, /sub/, builtin Xray paths) keeps precedence —
// ServeMux always picks the longest registered pattern first.
//
// Routing rules:
//   - exact file exists            → serve it (immutable cache for /_next/)
//   - directory with index.html    → serve that index
//   - extension-less unknown path  → index.html (client router takes over)
//   - unknown path WITH extension  → 404 (never serve HTML for missing assets)
func (s *Server) uiHandler() http.Handler {
	sub := webui.Dist()
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !webui.HasIndex() {
			// binary built without a UI (dev builds) — keep the old 404 behaviour
			http.NotFound(w, r)
			return
		}

		p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}

		if f, err := sub.Open(p); err == nil {
			st, statErr := f.Stat()
			_ = f.Close()
			if statErr == nil && !st.IsDir() {
				cacheAsset(w, p)
				fileServer.ServeHTTP(w, r)
				return
			}
			// directory → try its index.html (e.g. /sub-less nested routes)
			if idx := path.Join(p, "index.html"); idx != "" {
				if ff, err2 := sub.Open(idx); err2 == nil {
					_ = ff.Close()
					r2 := *r
					r2.URL.Path = "/" + idx
					cacheAsset(w, idx)
					fileServer.ServeHTTP(w, &r2)
					return
				}
			}
		}

		// not found → SPA fallback or 404 for asset-like paths
		if ext := path.Ext(r.URL.Path); ext != "" && ext != ".html" {
			http.NotFound(w, r)
			return
		}
		idx, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(idx)
	})
}

// cacheAsset overrides the global no-store for hashed build assets and fonts.
func cacheAsset(w http.ResponseWriter, p string) {
	switch {
	case strings.HasPrefix(p, "_next/"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	case strings.HasPrefix(p, "fonts/"):
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
}
