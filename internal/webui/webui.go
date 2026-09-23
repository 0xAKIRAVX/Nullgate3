// Package webui embeds the compiled panel UI (Next.js static export) into
// the api binary so ONE Railway service serves both the UI and the API
// same-origin — no separate web service needed on the free plan.
//
// Build flow (see root Dockerfile):
//
//	web/ --next build (STATIC_EXPORT=1)--> out/ --COPY--> internal/webui/dist/
//
// In the git repo, dist/ holds only .gitkeep so `go build ./...` works without
// a UI build; the Docker build overlays the real export output on top of it.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the embedded UI filesystem rooted at dist/.
// When the UI was not built (local go test/build), the FS still exists but
// contains only .gitkeep — the api serves the old 404 behaviour instead.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return dist // unreachable in practice
	}
	return sub
}

// HasIndex reports whether a real UI was embedded (index.html present).
func HasIndex() bool {
	f, err := Dist().Open("index.html")
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
