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
// When authToken is non-empty, index.html responses are rewritten to inject
// a <script> tag that sets window.__SAMVERK_TOKEN__ so the SPA can
// authenticate against the API.
func spaHandler(authToken string) http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("embedded static filesystem: " + err.Error())
	}

	// Pre-read index.html to inject runtime config (token + version).
	raw, readErr := fs.ReadFile(sub, "index.html")
	if readErr != nil {
		panic("read embedded index.html: " + readErr.Error())
	}

	// Build injection script with version info (always) and auth token (when configured).
	var scriptParts []string
	scriptParts = append(scriptParts,
		`window.__SAMVERK_VERSION__="`+html.EscapeString(version.Version)+`"`,
		`window.__SAMVERK_COMMIT__="`+html.EscapeString(version.GitCommit)+`"`,
	)
	if authToken != "" {
		scriptParts = append(scriptParts, `window.__SAMVERK_TOKEN__="`+html.EscapeString(authToken)+`"`)
	}
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
			_, _ = w.Write(injectedIndex)
			return
		}

		fileServer.ServeHTTP(w, r)
	})
}
