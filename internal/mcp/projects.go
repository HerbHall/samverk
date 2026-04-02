package mcp

import (
	"fmt"
	"os"
	"sort"
	"sync"

	"samverk.dev/samverk/internal/forge"
	"gopkg.in/yaml.v3"
)

// ValidPhases defines the allowed lifecycle phases for a project.
var ValidPhases = map[string]bool{
	"research":    true,
	"planning":    true,
	"design":      true,
	"development": true,
	"deployed":    true,
	"maintenance": true,
	"inactive":    true,
}

// Project represents a registered project with its forge connection.
type Project struct {
	Name      string                   `json:"name" yaml:"name"`
	Owner     string                   `json:"owner" yaml:"owner"`
	Repo      string                   `json:"repo" yaml:"repo"`
	Phase     string                   `json:"phase" yaml:"phase"`
	Tags      []string                 `json:"tags,omitempty" yaml:"tags"`
	ForgeType string                   `json:"forge_type,omitempty" yaml:"-"` // "github" or "gitea"
	ForgeURL  string                   `json:"forge_url,omitempty" yaml:"-"`  // base URL of the forge
	RepoDir   string                   `json:"repo_dir,omitempty" yaml:"-"`   // local git clone path for worktree isolation
	Tracker   forge.IssueTracker       `json:"-" yaml:"-"`
	Reader    forge.RepoReader         `json:"-" yaml:"-"`
	Writer    forge.RepoWriter         `json:"-" yaml:"-"`
	PRManager forge.PullRequestManager `json:"-" yaml:"-"`
}

// ProjectRegistry manages the set of available projects.
type ProjectRegistry struct {
	mu         sync.RWMutex
	projects   map[string]*Project
	active     string // name of the currently active project
	configPath string // path to server.yaml for phase persistence (empty = no persistence)
}

// NewProjectRegistry creates a new empty project registry.
func NewProjectRegistry() *ProjectRegistry {
	return &ProjectRegistry{
		projects: make(map[string]*Project),
	}
}

// Register adds a project to the registry. Returns an error if the name is
// empty or already registered. The first registered project becomes active.
func (r *ProjectRegistry) Register(p *Project) error {
	if p.Name == "" {
		return fmt.Errorf("project name must not be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.projects[p.Name]; exists {
		return fmt.Errorf("project %q is already registered", p.Name)
	}

	r.projects[p.Name] = p

	// First registered project becomes active automatically.
	if r.active == "" {
		r.active = p.Name
	}

	return nil
}

// SetActive switches the active project context.
// Returns an error if the named project is not registered.
func (r *ProjectRegistry) SetActive(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.projects[name]; !exists {
		return fmt.Errorf("project %q not found", name)
	}

	r.active = name
	return nil
}

// Active returns the currently active project.
// Returns an error if no projects are registered or no project is active.
func (r *ProjectRegistry) Active() (*Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.projects) == 0 {
		return nil, fmt.Errorf("no projects registered")
	}

	p, exists := r.projects[r.active]
	if !exists {
		return nil, fmt.Errorf("active project %q not found in registry", r.active)
	}

	return p, nil
}

// List returns all registered projects sorted by name.
func (r *ProjectRegistry) List() []*Project {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Project, 0, len(r.projects))
	for _, p := range r.projects {
		result = append(result, p)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

// Get retrieves a project by name.
func (r *ProjectRegistry) Get(name string) (*Project, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.projects[name]
	return p, exists
}

// CloneURL returns the Git clone URL for a registered project by name.
// Returns empty string if the project is not found.
// This method satisfies the server.VanityImportResolver interface.
func (r *ProjectRegistry) CloneURL(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.projects[name]
	if !exists || p.ForgeURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s.git", p.ForgeURL, p.Owner, p.Repo)
}

// TrackerFor returns the IssueTracker for the project matching the given
// owner/repo pair. Returns nil, false if no matching project is registered.
// This method satisfies the dispatcher.ProjectResolver interface.
func (r *ProjectRegistry) TrackerFor(owner, repo string) (forge.IssueTracker, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.projects {
		if p.Owner == owner && p.Repo == repo {
			return p.Tracker, true
		}
	}
	return nil, false
}

// PhaseFor returns the lifecycle phase for the project matching the given
// owner/repo pair. Returns ("", false) if no matching project is registered.
// This method satisfies the dispatcher.ProjectResolver interface.
func (r *ProjectRegistry) PhaseFor(owner, repo string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.projects {
		if p.Owner == owner && p.Repo == repo {
			return p.Phase, true
		}
	}
	return "", false
}

// RepoDirFor returns the local git clone path for the project matching the
// given owner/repo pair. Returns ("", false) if no matching project is
// registered or the project has no repo_dir configured.
// This method satisfies the dispatcher.ProjectResolver interface.
func (r *ProjectRegistry) RepoDirFor(owner, repo string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.projects {
		if p.Owner == owner && p.Repo == repo {
			if p.RepoDir == "" {
				return "", false
			}
			return p.RepoDir, true
		}
	}
	return "", false
}

// SetConfigPath records the path to the server.yaml file so that SetPhase
// can persist phase changes back to disk. Call this after loading configs.
func (r *ProjectRegistry) SetConfigPath(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configPath = path
}

// SetPhase updates the lifecycle phase of the named project in memory and, if
// a config path is set, persists the change to the server.yaml file.
// Returns an error if the project is not found or the phase is invalid.
func (r *ProjectRegistry) SetPhase(name, phase string) error {
	if !ValidPhases[phase] {
		return fmt.Errorf("invalid phase %q: must be one of research, planning, design, development, deployed, maintenance, inactive", phase)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.projects[name]
	if !exists {
		return fmt.Errorf("project %q not found", name)
	}

	oldPhase := p.Phase
	p.Phase = phase

	if r.configPath == "" || oldPhase == phase {
		return nil
	}

	if err := r.persistPhase(name, phase); err != nil {
		// Roll back in-memory update on persistence failure.
		p.Phase = oldPhase
		return fmt.Errorf("persist phase change for %q: %w", name, err)
	}

	return nil
}

// persistPhase reads the server.yaml file, updates the named project's phase,
// and writes the file back. Must be called with r.mu held.
func (r *ProjectRegistry) persistPhase(name, phase string) error {
	data, err := os.ReadFile(r.configPath) //nolint:gosec // G304: path is from trusted CLI flag
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var cfg projectsFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	updated := false
	for i := range cfg.Projects {
		if cfg.Projects[i].Name == name {
			cfg.Projects[i].Phase = phase
			updated = true
			break
		}
	}
	if !updated {
		return fmt.Errorf("project %q not found in config file", name)
	}

	out, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(r.configPath, out, 0o600); err != nil { //nolint:gosec // G306: config file
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// ActiveName returns the name of the currently active project.
func (r *ProjectRegistry) ActiveName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.active
}

// ProjectConfig is the YAML-serializable configuration for a project.
// Forge may be "github" (default) or "gitea". Gitea projects require GiteaURL
// and optionally GiteaToken (falls back to GITEA_TOKEN environment variable).
type ProjectConfig struct {
	Name       string   `yaml:"name"`
	Owner      string   `yaml:"owner"`
	Repo       string   `yaml:"repo"`
	Forge      string   `yaml:"forge"`       // "github" (default) or "gitea"
	GiteaURL   string   `yaml:"gitea_url"`   // required when forge = "gitea"
	GiteaToken string   `yaml:"gitea_token"` // optional; falls back to GITEA_TOKEN env var
	Phase      string   `yaml:"phase"`       // lifecycle phase; defaults to "development"
	Tags       []string `yaml:"tags"`        // optional classification tags
	RepoDir    string   `yaml:"repo_dir"`    // local git clone path for worktree isolation; empty disables
}

// projectsFileConfig is the top-level YAML structure for the projects config file.
type projectsFileConfig struct {
	Projects []ProjectConfig `yaml:"projects"`
}

// LoadProjectConfig reads and parses a YAML projects configuration file.
func LoadProjectConfig(path string) ([]ProjectConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is from trusted CLI flag
	if err != nil {
		return nil, fmt.Errorf("reading project config %s: %w", path, err)
	}

	var cfg projectsFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing project config %s: %w", path, err)
	}

	// Validate each project entry.
	for i := range cfg.Projects {
		p := &cfg.Projects[i]
		if p.Name == "" {
			return nil, fmt.Errorf("project at index %d: name is required", i)
		}
		if p.Owner == "" {
			return nil, fmt.Errorf("project %q: owner is required", p.Name)
		}
		if p.Repo == "" {
			return nil, fmt.Errorf("project %q: repo is required", p.Name)
		}
		if p.Forge != "" && p.Forge != "github" && p.Forge != "gitea" {
			return nil, fmt.Errorf("project %q: forge must be \"github\" or \"gitea\", got %q", p.Name, p.Forge)
		}
		if p.Forge == "gitea" && p.GiteaURL == "" {
			return nil, fmt.Errorf("project %q: gitea_url is required when forge = \"gitea\"", p.Name)
		}

		// Default phase to "development" if not specified.
		if p.Phase == "" {
			p.Phase = "development"
		} else if !ValidPhases[p.Phase] {
			return nil, fmt.Errorf("project %q: invalid phase %q", p.Name, p.Phase)
		}
	}

	return cfg.Projects, nil
}
