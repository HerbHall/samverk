package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	internalmcp "github.com/herbhall/samverk/internal/mcp"
)

func TestListForges_NoRegistry(t *testing.T) {
	a := &API{}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/forges", http.NoBody)
	w := httptest.NewRecorder()

	a.handleListForges(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []forgeDTO
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty slice, got %d items", len(result))
	}
}

func TestListForges_Deduplication(t *testing.T) {
	// Set up a test HTTP server to act as the forge.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	reg := internalmcp.NewProjectRegistry()
	// Two projects on the same Gitea instance — should produce one forge entry.
	_ = reg.Register(&internalmcp.Project{
		Name: "proj1", Owner: "org", Repo: "proj1",
		Phase: "development", ForgeType: "gitea", ForgeURL: ts.URL,
	})
	_ = reg.Register(&internalmcp.Project{
		Name: "proj2", Owner: "org", Repo: "proj2",
		Phase: "development", ForgeType: "gitea", ForgeURL: ts.URL,
	})
	// One GitHub project — different key.
	_ = reg.Register(&internalmcp.Project{
		Name: "proj3", Owner: "org", Repo: "proj3",
		Phase: "development", ForgeType: "github", ForgeURL: ts.URL,
	})

	a := &API{projectRegistry: reg}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/forges", http.NoBody)
	w := httptest.NewRecorder()

	a.handleListForges(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []forgeDTO
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// gitea@ts.URL and github@ts.URL are distinct — 2 entries.
	if len(result) != 2 {
		t.Fatalf("expected 2 deduplicated forges, got %d", len(result))
	}
}

func TestListForges_HealthyForge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	reg := internalmcp.NewProjectRegistry()
	_ = reg.Register(&internalmcp.Project{
		Name: "p", Owner: "org", Repo: "p",
		Phase: "development", ForgeType: "gitea", ForgeURL: srv.URL,
	})

	a := &API{projectRegistry: reg}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/forges", http.NoBody)
	w := httptest.NewRecorder()
	a.handleListForges(w, req)

	var result []forgeDTO
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 forge, got %d", len(result))
	}
	if !result[0].Healthy {
		t.Errorf("expected forge to be healthy")
	}
	if result[0].Type != "gitea" {
		t.Errorf("type = %q, want %q", result[0].Type, "gitea")
	}
}

func TestListForges_UnhealthyForge(t *testing.T) {
	// Use a URL that will refuse connections (port 1 is typically closed/refused).
	reg := internalmcp.NewProjectRegistry()
	_ = reg.Register(&internalmcp.Project{
		Name: "p", Owner: "org", Repo: "p",
		Phase: "development", ForgeType: "gitea", ForgeURL: "http://127.0.0.1:1",
	})

	a := &API{projectRegistry: reg}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/forges", http.NoBody)
	w := httptest.NewRecorder()
	a.handleListForges(w, req)

	var result []forgeDTO
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 forge, got %d", len(result))
	}
	if result[0].Healthy {
		t.Errorf("expected forge to be unhealthy for refused connection")
	}
}

func TestListForges_ProjectsWithNoForgeType(t *testing.T) {
	// Projects without ForgeType should be skipped in forge listing.
	reg := internalmcp.NewProjectRegistry()
	_ = reg.Register(&internalmcp.Project{
		Name: "legacy", Owner: "org", Repo: "legacy",
		Phase: "development",
		// No ForgeType/ForgeURL set.
	})

	a := &API{projectRegistry: reg}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/forges", http.NoBody)
	w := httptest.NewRecorder()
	a.handleListForges(w, req)

	var result []forgeDTO
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 forges for projects with no ForgeType, got %d", len(result))
	}
}
