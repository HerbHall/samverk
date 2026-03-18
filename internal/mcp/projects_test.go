package mcp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	internalmcp "github.com/herbhall/samverk/internal/mcp"
)

// newHTTPTestServer wraps an http.Handler in an httptest.Server with cleanup.
func newHTTPTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

func TestProjectRegistry_RegisterAndList(t *testing.T) {
	tests := []struct {
		name     string
		projects []*internalmcp.Project
		wantLen  int
		wantErr  bool
	}{
		{
			name: "register single project",
			projects: []*internalmcp.Project{
				{Name: "alpha", Owner: "org", Repo: "alpha"},
			},
			wantLen: 1,
		},
		{
			name: "register multiple projects",
			projects: []*internalmcp.Project{
				{Name: "alpha", Owner: "org", Repo: "alpha"},
				{Name: "beta", Owner: "org", Repo: "beta"},
				{Name: "gamma", Owner: "org", Repo: "gamma"},
			},
			wantLen: 3,
		},
		{
			name: "duplicate name returns error",
			projects: []*internalmcp.Project{
				{Name: "alpha", Owner: "org", Repo: "alpha"},
				{Name: "alpha", Owner: "org", Repo: "alpha-2"},
			},
			wantErr: true,
		},
		{
			name: "empty name returns error",
			projects: []*internalmcp.Project{
				{Name: "", Owner: "org", Repo: "alpha"},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := internalmcp.NewProjectRegistry()
			var lastErr error
			for _, p := range tc.projects {
				if err := reg.Register(p); err != nil {
					lastErr = err
				}
			}

			if tc.wantErr {
				if lastErr == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if lastErr != nil {
				t.Fatalf("unexpected error: %v", lastErr)
			}

			list := reg.List()
			if len(list) != tc.wantLen {
				t.Errorf("want %d projects, got %d", tc.wantLen, len(list))
			}

			// Verify list is sorted by name.
			for i := 1; i < len(list); i++ {
				if list[i-1].Name >= list[i].Name {
					t.Errorf("list not sorted: %q >= %q at index %d", list[i-1].Name, list[i].Name, i)
				}
			}
		})
	}
}

func TestProjectRegistry_SetActive(t *testing.T) {
	reg := internalmcp.NewProjectRegistry()
	_ = reg.Register(&internalmcp.Project{Name: "alpha", Owner: "org", Repo: "alpha"})
	_ = reg.Register(&internalmcp.Project{Name: "beta", Owner: "org", Repo: "beta"})

	// First registered project should be active by default.
	active, err := reg.Active()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active.Name != "alpha" {
		t.Errorf("expected default active to be %q, got %q", "alpha", active.Name)
	}

	// Switch to beta.
	if err := reg.SetActive("beta"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	active, err = reg.Active()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active.Name != "beta" {
		t.Errorf("expected active to be %q, got %q", "beta", active.Name)
	}

	// Switch back to alpha.
	if err := reg.SetActive("alpha"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	active, err = reg.Active()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active.Name != "alpha" {
		t.Errorf("expected active to be %q, got %q", "alpha", active.Name)
	}
}

func TestProjectRegistry_SetActive_NotFound(t *testing.T) {
	reg := internalmcp.NewProjectRegistry()
	_ = reg.Register(&internalmcp.Project{Name: "alpha", Owner: "org", Repo: "alpha"})

	err := reg.SetActive("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown project, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %q", err.Error())
	}
}

func TestProjectRegistry_Active_NoProjects(t *testing.T) {
	reg := internalmcp.NewProjectRegistry()

	_, err := reg.Active()
	if err == nil {
		t.Fatal("expected error for empty registry, got nil")
	}
	if !strings.Contains(err.Error(), "no projects registered") {
		t.Errorf("expected 'no projects registered' in error, got %q", err.Error())
	}
}

func TestProjectRegistry_Get(t *testing.T) {
	reg := internalmcp.NewProjectRegistry()
	_ = reg.Register(&internalmcp.Project{Name: "alpha", Owner: "org", Repo: "alpha"})

	p, ok := reg.Get("alpha")
	if !ok {
		t.Fatal("expected project 'alpha' to be found")
	}
	if p.Owner != "org" {
		t.Errorf("expected owner %q, got %q", "org", p.Owner)
	}

	_, ok = reg.Get("missing")
	if ok {
		t.Error("expected project 'missing' not to be found")
	}
}

func TestLoadProjectConfig(t *testing.T) {
	content := `projects:
  - name: samverk
    owner: HerbHall
    repo: samverk
  - name: devkit
    owner: HerbHall
    repo: devkit
  - name: samverk-gitea
    owner: samverk
    repo: samverk
    forge: gitea
    gitea_url: https://gitea.herbhall.net
    gitea_token: tok123
`
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	configs, err := internalmcp.LoadProjectConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(configs) != 3 {
		t.Fatalf("expected 3 configs, got %d", len(configs))
	}

	if configs[0].Name != "samverk" {
		t.Errorf("expected first project name %q, got %q", "samverk", configs[0].Name)
	}
	if configs[0].Owner != "HerbHall" {
		t.Errorf("expected first project owner %q, got %q", "HerbHall", configs[0].Owner)
	}
	if configs[0].Repo != "samverk" {
		t.Errorf("expected first project repo %q, got %q", "samverk", configs[0].Repo)
	}
	if configs[1].Name != "devkit" {
		t.Errorf("expected second project name %q, got %q", "devkit", configs[1].Name)
	}
	// Gitea project.
	if configs[2].Forge != "gitea" {
		t.Errorf("expected Gitea forge, got %q", configs[2].Forge)
	}
	if configs[2].GiteaURL != "https://gitea.herbhall.net" {
		t.Errorf("expected gitea_url, got %q", configs[2].GiteaURL)
	}
	if configs[2].GiteaToken != "tok123" {
		t.Errorf("expected gitea_token, got %q", configs[2].GiteaToken)
	}
}

func TestLoadProjectConfig_PhaseValidation(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantPhase string
		wantTags  []string
		wantErr   string
	}{
		{
			name: "valid phase accepted",
			content: `projects:
  - name: alpha
    owner: org
    repo: alpha
    phase: deployed
`,
			wantPhase: "deployed",
		},
		{
			name: "empty phase defaults to development",
			content: `projects:
  - name: alpha
    owner: org
    repo: alpha
`,
			wantPhase: "development",
		},
		{
			name: "unknown phase rejected",
			content: `projects:
  - name: alpha
    owner: org
    repo: alpha
    phase: archived
`,
			wantErr: "invalid phase",
		},
		{
			name: "tags parsed correctly",
			content: `projects:
  - name: alpha
    owner: org
    repo: alpha
    phase: development
    tags:
      - go
      - infrastructure
`,
			wantPhase: "development",
			wantTags:  []string{"go", "infrastructure"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "server.yaml")
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatalf("writing test config: %v", err)
			}

			configs, err := internalmcp.LoadProjectConfig(path)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("expected error containing %q, got %q", tc.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(configs) == 0 {
				t.Fatal("expected at least one config")
			}
			if configs[0].Phase != tc.wantPhase {
				t.Errorf("expected phase %q, got %q", tc.wantPhase, configs[0].Phase)
			}
			if tc.wantTags != nil {
				if len(configs[0].Tags) != len(tc.wantTags) {
					t.Fatalf("expected %d tags, got %d", len(tc.wantTags), len(configs[0].Tags))
				}
				for i, tag := range tc.wantTags {
					if configs[0].Tags[i] != tag {
						t.Errorf("tag[%d]: expected %q, got %q", i, tag, configs[0].Tags[i])
					}
				}
			}
		})
	}
}

func TestLoadProjectConfig_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "invalid YAML",
			content: "{{invalid yaml",
			wantErr: "parsing project config",
		},
		{
			name: "missing name",
			content: `projects:
  - owner: HerbHall
    repo: samverk
`,
			wantErr: "name is required",
		},
		{
			name: "missing owner",
			content: `projects:
  - name: samverk
    repo: samverk
`,
			wantErr: "owner is required",
		},
		{
			name: "missing repo",
			content: `projects:
  - name: samverk
    owner: HerbHall
`,
			wantErr: "repo is required",
		},
		{
			name: "invalid forge value",
			content: `projects:
  - name: samverk
    owner: HerbHall
    repo: samverk
    forge: bitbucket
`,
			wantErr: "forge must be",
		},
		{
			name: "gitea missing gitea_url",
			content: `projects:
  - name: samverk-gitea
    owner: samverk
    repo: samverk
    forge: gitea
`,
			wantErr: "gitea_url is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "server.yaml")
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatalf("writing test config: %v", err)
			}

			_, err := internalmcp.LoadProjectConfig(path)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestLoadProjectConfig_FileNotFound(t *testing.T) {
	_, err := internalmcp.LoadProjectConfig("/nonexistent/path/server.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// --- MCP tool integration tests for list_projects and set_project ---

func TestListProjects_NoRegistry(t *testing.T) {
	// Handler without project registry returns "not configured" message.
	ts := newTestMCPServer(t, &mockTracker{}, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      100,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "list_projects",
			"arguments": map[string]any{},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in response")
	}
	if !strings.Contains(result.Content[0].Text, "not configured") {
		t.Errorf("expected 'not configured' message, got %q", result.Content[0].Text)
	}
}

func TestListProjects_WithRegistry(t *testing.T) {
	tracker := &mockTracker{}
	h := internalmcp.NewHandler(tracker, nil, nil, nil, nil)

	reg := internalmcp.NewProjectRegistry()
	_ = reg.Register(&internalmcp.Project{
		Name: "alpha", Owner: "org", Repo: "alpha", Tracker: tracker,
	})
	_ = reg.Register(&internalmcp.Project{
		Name: "beta", Owner: "org", Repo: "beta", Tracker: tracker,
	})
	h.SetProjects(reg)

	handler := internalmcp.NewHTTPHandler(h)
	ts := newHTTPTestServer(t, handler)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      101,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "list_projects",
			"arguments": map[string]any{},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	// Parse the JSON array to verify structure.
	var projects []struct {
		Name   string `json:"name"`
		Owner  string `json:"owner"`
		Repo   string `json:"repo"`
		Active bool   `json:"active"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &projects); err != nil {
		t.Fatalf("unmarshal projects list: %v\ntext: %s", err, result.Content[0].Text)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}

	// Projects should be sorted alphabetically, alpha first.
	if projects[0].Name != "alpha" {
		t.Errorf("expected first project %q, got %q", "alpha", projects[0].Name)
	}
	if !projects[0].Active {
		t.Error("expected first project to be active")
	}
	if projects[1].Name != "beta" {
		t.Errorf("expected second project %q, got %q", "beta", projects[1].Name)
	}
	if projects[1].Active {
		t.Error("expected second project to be inactive")
	}
}

func TestSetProject(t *testing.T) {
	tracker := &mockTracker{}
	h := internalmcp.NewHandler(tracker, nil, nil, nil, nil)

	reg := internalmcp.NewProjectRegistry()
	_ = reg.Register(&internalmcp.Project{
		Name: "alpha", Owner: "org", Repo: "alpha", Tracker: tracker,
	})
	_ = reg.Register(&internalmcp.Project{
		Name: "beta", Owner: "org", Repo: "beta", Tracker: tracker,
	})
	h.SetProjects(reg)

	handler := internalmcp.NewHTTPHandler(h)
	ts := newHTTPTestServer(t, handler)

	// Switch to beta.
	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      102,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "set_project",
			"arguments": map[string]any{
				"name": "beta",
			},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in response")
	}
	if !strings.Contains(result.Content[0].Text, "beta") {
		t.Errorf("expected response to mention 'beta', got %q", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "Switched") {
		t.Errorf("expected 'Switched' confirmation, got %q", result.Content[0].Text)
	}

	// Verify beta is now active via list_projects.
	respBody = postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      103,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "list_projects",
			"arguments": map[string]any{},
		},
	})

	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	var projects []struct {
		Name   string `json:"name"`
		Active bool   `json:"active"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &projects); err != nil {
		t.Fatalf("unmarshal projects: %v", err)
	}
	for _, p := range projects {
		if p.Name == "beta" && !p.Active {
			t.Error("expected beta to be active after set_project")
		}
		if p.Name == "alpha" && p.Active {
			t.Error("expected alpha to be inactive after set_project to beta")
		}
	}
}

func TestSetProject_NotFound(t *testing.T) {
	tracker := &mockTracker{}
	h := internalmcp.NewHandler(tracker, nil, nil, nil, nil)

	reg := internalmcp.NewProjectRegistry()
	_ = reg.Register(&internalmcp.Project{
		Name: "alpha", Owner: "org", Repo: "alpha", Tracker: tracker,
	})
	h.SetProjects(reg)

	handler := internalmcp.NewHTTPHandler(h)
	ts := newHTTPTestServer(t, handler)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      104,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "set_project",
			"arguments": map[string]any{
				"name": "nonexistent",
			},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	// Expect either protocol error or isError in result.
	if resp.Error != nil {
		return
	}
	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !result.IsError {
		t.Error("expected isError=true for unknown project")
	}
}

func TestSetProject_NoRegistry(t *testing.T) {
	ts := newTestMCPServer(t, &mockTracker{}, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      105,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "set_project",
			"arguments": map[string]any{
				"name": "alpha",
			},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	// Expect error since multi-project is not configured.
	if resp.Error != nil {
		return
	}
	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !result.IsError {
		t.Error("expected isError=true when registry is nil")
	}
}

func TestProjectRegistry_PhaseFor(t *testing.T) {
	reg := internalmcp.NewProjectRegistry()
	_ = reg.Register(&internalmcp.Project{Name: "alpha", Owner: "org", Repo: "alpha", Phase: "development"})
	_ = reg.Register(&internalmcp.Project{Name: "beta", Owner: "org", Repo: "beta", Phase: "maintenance"})

	tests := []struct {
		name      string
		owner     string
		repo      string
		wantPhase string
		wantFound bool
	}{
		{"found development", "org", "alpha", "development", true},
		{"found maintenance", "org", "beta", "maintenance", true},
		{"not found", "org", "missing", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			phase, found := reg.PhaseFor(tc.owner, tc.repo)
			if found != tc.wantFound {
				t.Errorf("found = %v, want %v", found, tc.wantFound)
			}
			if phase != tc.wantPhase {
				t.Errorf("phase = %q, want %q", phase, tc.wantPhase)
			}
		})
	}
}

func TestProjectRegistry_SetPhase(t *testing.T) {
	t.Run("valid transition in memory", func(t *testing.T) {
		reg := internalmcp.NewProjectRegistry()
		_ = reg.Register(&internalmcp.Project{Name: "alpha", Owner: "org", Repo: "alpha", Phase: "development"})

		if err := reg.SetPhase("alpha", "maintenance"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		phase, found := reg.PhaseFor("org", "alpha")
		if !found {
			t.Fatal("project not found after SetPhase")
		}
		if phase != "maintenance" {
			t.Errorf("phase = %q, want %q", phase, "maintenance")
		}
	})

	t.Run("invalid phase rejected", func(t *testing.T) {
		reg := internalmcp.NewProjectRegistry()
		_ = reg.Register(&internalmcp.Project{Name: "alpha", Owner: "org", Repo: "alpha", Phase: "development"})

		if err := reg.SetPhase("alpha", "bogus"); err == nil {
			t.Fatal("expected error for invalid phase, got nil")
		}
	})

	t.Run("unknown project rejected", func(t *testing.T) {
		reg := internalmcp.NewProjectRegistry()

		if err := reg.SetPhase("nonexistent", "maintenance"); err == nil {
			t.Fatal("expected error for unknown project, got nil")
		}
	})

	t.Run("persists to yaml file", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "server.yaml")

		// Write a minimal server.yaml.
		yaml := "projects:\n  - name: alpha\n    owner: org\n    repo: alpha\n    forge: github\n    phase: development\n"
		if err := os.WriteFile(cfgPath, []byte(yaml), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}

		reg := internalmcp.NewProjectRegistry()
		reg.SetConfigPath(cfgPath)
		_ = reg.Register(&internalmcp.Project{Name: "alpha", Owner: "org", Repo: "alpha", Phase: "development"})

		if err := reg.SetPhase("alpha", "inactive"); err != nil {
			t.Fatalf("SetPhase: %v", err)
		}

		// Verify the file was updated.
		data, err := os.ReadFile(cfgPath)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		if !strings.Contains(string(data), "phase: inactive") {
			t.Errorf("config file does not contain 'phase: inactive':\n%s", data)
		}
	})
}
