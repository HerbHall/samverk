package server

import (
	"bytes"
	"embed"
	"html"
	"io/fs"
	"net/http"
	"strings"

	"github.com/herbhall/samverk/internal/version"
)

//go:embed all:static
var staticFS embed.FS

// spaHandler serves the embedded SPA files. For paths that don't match a
// static file, it serves index.html to support client-side routing.
//
// Only version info is injected into index.html. Auth tokens are never
// included in the HTML source -- the SPA authenticates via session cookies.
func spaHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("embedded static filesystem: " + err.Error())
	}

	// Pre-read index.html to inject runtime config (version only).
	raw, readErr := fs.ReadFile(sub, "index.html")
	if readErr != nil {
		panic("read embedded index.html: " + readErr.Error())
	}

	// Build injection script with version info only -- no auth token.
	scriptParts := make([]string, 0, 2)
	scriptParts = append(scriptParts,
		`window.__SAMVERK_VERSION__="`+html.EscapeString(version.Version)+`"`,
		`window.__SAMVERK_COMMIT__="`+html.EscapeString(version.GitCommit)+`"`,
	)
	tag := []byte(`<script>` + strings.Join(scriptParts, ";") + `;</script>`)
	injectedIndex := bytes.Replace(raw, []byte("</head>"), append(tag, []byte("</head>")...), 1)

	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the actual file first.
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		servingIndex := path == "index.html"
		if _, statErr := fs.Stat(sub, path); statErr != nil {
			// File not found -- serve index.html for client-side routing.
			servingIndex = true
		}

		// Serve injected index directly so runtime config is included.
		if servingIndex {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			// index.html must never be cached: it references hashed asset filenames
			// that change on every build. A stale index.html causes 404s on assets
			// and a blank page until the user force-refreshes.
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			_, _ = w.Write(injectedIndex)
			return
		}

		// Vite embeds a content-hash in every asset filename (e.g. index-BulwOWDY.js).
		// These are safe to cache indefinitely: the filename changes whenever the
		// content changes, so there is no risk of a stale asset being served.
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}

		fileServer.ServeHTTP(w, r)
	})
}
