package server

import (
	"bytes"
	"embed"
	"html"
	"io/fs"
	"net/http"
	"strings"
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

	// Pre-read index.html for token injection (only when token is configured).
	var injectedIndex []byte
	if authToken != "" {
		raw, readErr := fs.ReadFile(sub, "index.html")
		if readErr != nil {
			panic("read embedded index.html: " + readErr.Error())
		}
		safe := html.EscapeString(authToken)
		tag := []byte(`<script>window.__SAMVERK_TOKEN__="` + safe + `";</script>`)
		injectedIndex = bytes.Replace(raw, []byte("</head>"), append(tag, []byte("</head>")...), 1)
	}

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

		// If we have an injected index, serve it directly instead of
		// going through FileServer so the token tag is included.
		if servingIndex && injectedIndex != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(injectedIndex)
			return
		}

		if !servingIndex {
			fileServer.ServeHTTP(w, r)
			return
		}

		// No token configured -- let FileServer handle index.html as-is.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
