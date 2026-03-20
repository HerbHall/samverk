package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

// dataSourceDTO describes one data store tracked by the data sources endpoint.
type dataSourceDTO struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	SizeBytes   int64  `json:"size_bytes"`
	RecordCount int64  `json:"record_count"`
	Path        string `json:"path,omitempty"`
	Healthy     bool   `json:"healthy"`
}

// SetDataPaths configures the file-system paths for the two SQLite databases so
// the /api/v1/data/sources endpoint can report their sizes. Call before serving.
// Pass empty strings to omit individual databases from the response.
func (a *API) SetDataPaths(primaryDB, logsDB string) {
	a.primaryDBPath = primaryDB
	a.logsDBPath = logsDB
}

// handleDataSources returns health and storage information for all data stores.
func (a *API) handleDataSources(w http.ResponseWriter, r *http.Request) {
	var sources []dataSourceDTO

	// Primary SQLite database.
	if a.primaryDBPath != "" {
		src := sqliteSource("samverk-db", a.primaryDBPath)
		if a.store != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			src.RecordCount = countSessions(ctx, a)
		}
		sources = append(sources, src)
	}

	// Logs SQLite database.
	if a.logsDBPath != "" {
		src := sqliteSource("logs-db", a.logsDBPath)
		if a.logStore != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			src.RecordCount = countLogEntries(ctx, a)
		}
		sources = append(sources, src)
	}

	// Synapset vector database (proxied via /api/v1/synapset/status).
	if a.synapsetProxy != nil {
		src := synapsetSource(r.Context(), a.synapsetProxy)
		sources = append(sources, src)
	}

	if sources == nil {
		sources = []dataSourceDTO{}
	}

	writeJSON(w, http.StatusOK, sources)
}

// sqliteSource builds a dataSourceDTO for a local SQLite file using os.Stat for size.
func sqliteSource(name, path string) dataSourceDTO {
	src := dataSourceDTO{
		Name:    name,
		Type:    "sqlite",
		Path:    path,
		Healthy: false,
	}
	fi, err := os.Stat(path)
	if err == nil {
		src.SizeBytes = fi.Size()
		src.Healthy = true
	}
	return src
}

// countSessions returns the total number of sessions in the primary store.
// Returns 0 on error rather than failing the whole request.
func countSessions(ctx context.Context, a *API) int64 {
	sessions, err := a.store.ListSessions(ctx, models.SessionStatus(""))
	if err != nil {
		return 0
	}
	return int64(len(sessions))
}

// countLogEntries returns the total number of log entries. Returns 0 on error.
func countLogEntries(ctx context.Context, a *API) int64 {
	if a.logStore == nil {
		return 0
	}
	count, err := a.logStore.Count(ctx)
	if err != nil {
		return 0
	}
	return count
}

// synapsetSource fetches size and record count from the Synapset status endpoint.
func synapsetSource(ctx context.Context, proxy *SynapsetProxy) dataSourceDTO {
	src := dataSourceDTO{
		Name:    "synapset",
		Type:    "vector-db",
		Healthy: false,
	}

	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, proxy.baseURL+"/api/status", http.NoBody) //nolint:gosec // G704: URL is from trusted baseURL config
	if err != nil {
		return src
	}
	req.Header.Set("Accept", "application/json")

	resp, err := proxy.client.Do(req) //nolint:gosec // G107: URL is built from configured baseURL, not user input
	if err != nil {
		return src
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return src
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return src
	}

	var status struct {
		DBSizeBytes int64 `json:"db_size_bytes"`
		MemoryCount int64 `json:"memory_count"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return src
	}

	src.SizeBytes = status.DBSizeBytes
	src.RecordCount = status.MemoryCount
	src.Healthy = true
	return src
}


