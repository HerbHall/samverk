package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/herbhall/samverk/internal/forge"
	internalmcp "github.com/herbhall/samverk/internal/mcp"
	"github.com/herbhall/samverk/pkg/models"
)

func TestListProjects_NoRegistry(t *testing.T) {
	a := &API{}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/projects", http.NoBody)
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
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/projects", http.NoBody)
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
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/projects", http.NoBody)
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

func TestListProjects_IssueCounts(t *testing.T) {
	reg := internalmcp.NewProjectRegistry()

	// Tracker with 3 open issues: 1 needs-human, 1 claimed, 1 plain.
	mt := &projectsMockTracker{
		issues: []*forge.Issue{
			{Number: 1, Labels: []string{models.LabelStatusNeedsHuman}},
			{Number: 2, Labels: []string{models.LabelStatusClaimed}},
			{Number: 3, Labels: []string{}},
		},
	}
	_ = reg.Register(&internalmcp.Project{Name: "proj", Owner: "org", Repo: "proj", Phase: "development", Tracker: mt})

	a := &API{projectRegistry: reg}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/projects", http.NoBody)
	w := httptest.NewRecorder()

	a.handleListProjects(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []projectDTO
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result))
	}
	got := result[0]
	if got.OpenIssues != 3 {
		t.Errorf("open_issues = %d, want 3", got.OpenIssues)
	}
	if got.NeedsHuman != 1 {
		t.Errorf("needs_human = %d, want 1", got.NeedsHuman)
	}
	if got.InProgress != 1 {
		t.Errorf("in_progress = %d, want 1", got.InProgress)
	}
}

func TestListProjects_IssueCountsTrackerError(t *testing.T) {
	reg := internalmcp.NewProjectRegistry()
	mt := &projectsMockTracker{listErr: fmt.Errorf("rate limited")}
	_ = reg.Register(&internalmcp.Project{Name: "proj", Owner: "org", Repo: "proj", Phase: "development", Tracker: mt})

	a := &API{projectRegistry: reg}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/projects", http.NoBody)
	w := httptest.NewRecorder()

	a.handleListProjects(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even on tracker error, got %d", w.Code)
	}
	var result []projectDTO
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result[0].OpenIssues != 0 {
		t.Errorf("expected 0 open_issues on error, got %d", result[0].OpenIssues)
	}
}

func TestListProjects_ForgeFields(t *testing.T) {
	reg := internalmcp.NewProjectRegistry()
	_ = reg.Register(&internalmcp.Project{
		Name:      "samverk",
		Owner:     "org",
		Repo:      "samverk",
		Phase:     "development",
		ForgeType: "gitea",
		ForgeURL:  "http://gitea.example.com",
	})

	a := &API{projectRegistry: reg}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/projects", http.NoBody)
	w := httptest.NewRecorder()

	a.handleListProjects(w, req)

	var result []projectDTO
	_ = json.NewDecoder(w.Body).Decode(&result)
	if len(result) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result))
	}
	if result[0].Forge != "gitea" {
		t.Errorf("forge = %q, want %q", result[0].Forge, "gitea")
	}
	if result[0].ForgeURL != "http://gitea.example.com" {
		t.Errorf("forge_url = %q, want %q", result[0].ForgeURL, "http://gitea.example.com")
	}
}

// projectsMockTracker is a minimal forge.IssueTracker for projects tests.
type projectsMockTracker struct {
	issues  []*forge.Issue
	listErr error
}

func (m *projectsMockTracker) ListIssues(_ context.Context, _ *forge.ListOptions) ([]*forge.Issue, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.issues, nil
}

func (m *projectsMockTracker) GetIssue(_ context.Context, _ int) (*forge.Issue, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *projectsMockTracker) CreateIssue(_ context.Context, _ *forge.CreateIssueRequest) (*forge.Issue, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *projectsMockTracker) UpdateIssue(_ context.Context, _ int, _ *forge.UpdateIssueRequest) (*forge.Issue, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *projectsMockTracker) AddComment(_ context.Context, _ int, _ string) (*forge.Comment, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *projectsMockTracker) ListComments(_ context.Context, _ int) ([]*forge.Comment, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *projectsMockTracker) SetLabels(_ context.Context, _ int, _ []string) error {
	return fmt.Errorf("not implemented")
}
func (m *projectsMockTracker) AddLabel(_ context.Context, _ int, _ string) error {
	return fmt.Errorf("not implemented")
}
func (m *projectsMockTracker) RemoveLabel(_ context.Context, _ int, _ string) error {
	return fmt.Errorf("not implemented")
}
func (m *projectsMockTracker) Assign(_ context.Context, _ int, _ string) error {
	return fmt.Errorf("not implemented")
}
func (m *projectsMockTracker) Unassign(_ context.Context, _ int, _ string) error {
	return fmt.Errorf("not implemented")
}
func (m *projectsMockTracker) Watch(_ context.Context, _ func(forge.Event)) error {
	return fmt.Errorf("not implemented")
}
func (m *projectsMockTracker) SearchIssues(_ context.Context, _ *forge.SearchOptions) ([]*forge.Issue, error) {
	return nil, fmt.Errorf("not implemented")
}
