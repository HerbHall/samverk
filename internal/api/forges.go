package api

import (
	"context"
	"net/http"
	"time"
)

// forgeDTO is the JSON representation of a configured forge.
type forgeDTO struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	URL     string `json:"url"`
	Healthy bool   `json:"healthy"`
}

// handleListForges returns configured forges with health status.
// It derives forges from the project registry, deduplicating by URL+type.
// Returns an empty array when no registry is configured.
func (a *API) handleListForges(w http.ResponseWriter, _ *http.Request) {
	if a.projectRegistry == nil {
		writeJSON(w, http.StatusOK, []forgeDTO{})
		return
	}

	projects := a.projectRegistry.List()

	// Deduplicate forges by type+URL combination.
	type forgeKey struct {
		forgeType string
		forgeURL  string
	}
	seen := make(map[forgeKey]string) // key -> display name (first project wins)
	for _, p := range projects {
		if p.ForgeType == "" {
			continue
		}
		key := forgeKey{forgeType: p.ForgeType, forgeURL: p.ForgeURL}
		if _, exists := seen[key]; !exists {
			seen[key] = p.ForgeType
		}
	}

	// For each unique forge, check health and build the DTO.
	dtos := make([]forgeDTO, 0, len(seen))
	for key, name := range seen {
		healthy := checkForgeHealth(key.forgeURL)
		dtos = append(dtos, forgeDTO{
			Name:    name,
			Type:    key.forgeType,
			URL:     key.forgeURL,
			Healthy: healthy,
		})
	}

	writeJSON(w, http.StatusOK, dtos)
}

// checkForgeHealth attempts an HTTP GET to the forge URL with a 3-second timeout.
// Returns true if the response is 2xx, false otherwise.
func checkForgeHealth(url string) bool {
	if url == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return false
	}

	client := &http.Client{
		Timeout: 3 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			// Allow redirects (default behaviour); return nil to follow them.
			return nil
		},
	}
	resp, err := client.Do(req) //nolint:gosec // G704: URL is from trusted server.yaml forge config
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
