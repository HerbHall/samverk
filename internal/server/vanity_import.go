package server

import (
	"fmt"
	"net/http"
	"strings"
)

// VanityImportResolver looks up a project's Git clone URL by name.
// Used by the vanity import handler to serve go-import meta tags.
type VanityImportResolver interface {
	// CloneURL returns the Git clone URL for a registered project.
	// Returns empty string if the project is not found.
	CloneURL(projectName string) string
}

// vanityImportDomain is the canonical domain used in go-import meta tags.
const vanityImportDomain = "samverk.dev"

// handleVanityImport serves go-import meta tags for Go module resolution.
// When a request has ?go-get=1, it returns an HTML page with the meta tag
// pointing to the project's current forge. Without the query param, the
// request is not handled (returns false) so the caller can fall through
// to the SPA handler.
func handleVanityImport(resolver VanityImportResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("go-get") != "1" {
			// Not a go-get request; let the SPA handle it.
			http.NotFound(w, r)
			return
		}

		// Extract project name from path: /samverk -> "samverk", /synapset -> "synapset"
		name := strings.TrimPrefix(r.URL.Path, "/")
		// Strip any sub-path: samverk/internal/version -> samverk
		if idx := strings.Index(name, "/"); idx >= 0 {
			name = name[:idx]
		}

		if name == "" {
			http.NotFound(w, r)
			return
		}

		cloneURL := resolver.CloneURL(name)
		if cloneURL == "" {
			http.NotFound(w, r)
			return
		}

		importPath := fmt.Sprintf("%s/%s", vanityImportDomain, name)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		//nolint:gosec // G705: values are from internal ProjectRegistry config, not user input
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
<meta name="go-import" content="%s git %s">
<meta http-equiv="refresh" content="0; url=https://pkg.go.dev/%s">
</head>
<body>Redirecting to <a href="https://pkg.go.dev/%s">pkg.go.dev/%s</a>...</body>
</html>
`, importPath, cloneURL, importPath, importPath, importPath)
	}
}
