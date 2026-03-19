package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	internalmcp "github.com/herbhall/samverk/internal/mcp"
)

func TestListProjects_NoRegistry(t *testing.T) {
	a := &API{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	w := httptest.NewRecorder()

	a.handleListProjects(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []projectDTO
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty slice, got %d items", len(result))
	}
}

func TestListProjects_MultiProject(t *testing.T) {
	reg := internalmcp.NewProjectRegistry()
	_ = reg.Register(&internalmcp.Project{Name: "alpha", Owner: "org", Repo: "alpha", Phase: "development", Tags: []string{"backend"}})
	_ = reg.Register(&internalmcp.Project{Name: "beta", Owner: "org", Repo: "beta", Phase: "planning"})
	_ = reg.Register(&internalmcp.Project{Name: "gamma", Owner: "org", Repo: "gamma", Phase: "deployed", Tags: []string{"frontend", "oss"}})

	a := &API{projectRegistry: reg}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	w := httptest.NewRecorder()

	a.handleListProjects(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []projectDTO
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(result))
	}

	// List() sorts by name, so expect: alpha, beta, gamma.
	names := []string{result[0].Name, result[1].Name, result[2].Name}
	want := []string{"alpha", "beta", "gamma"}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("project[%d]: got name %q, want %q", i, n, want[i])
		}
	}
}

func TestListProjects_ActiveFlag(t *testing.T) {
	reg := internalmcp.NewProjectRegistry()
	_ = reg.Register(&internalmcp.Project{Name: "first", Owner: "org", Repo: "first", Phase: "development"})
	_ = reg.Register(&internalmcp.Project{Name: "second", Owner: "org", Repo: "second", Phase: "development"})
	_ = reg.Register(&internalmcp.Project{Name: "third", Owner: "org", Repo: "third", Phase: "development"})
	// Make "second" the active project.
	_ = reg.SetActive("second")

	a := &API{projectRegistry: reg}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	w := httptest.NewRecorder()

	a.handleListProjects(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []projectDTO
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	activeCount := 0
	for _, p := range result {
		if p.Active {
			activeCount++
			if p.Name != "second" {
				t.Errorf("expected active project to be %q, got %q", "second", p.Name)
			}
		}
	}
	if activeCount != 1 {
		t.Errorf("expected exactly 1 active project, got %d", activeCount)
	}
}
