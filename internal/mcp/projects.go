package mcp

import (
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/herbhall/samverk/internal/forge"
	"gopkg.in/yaml.v3"
)

// Project represents a registered project with its forge connection.
type Project struct {
	Name      string                    `json:"name" yaml:"name"`
	Owner     string                    `json:"owner" yaml:"owner"`
	Repo      string                    `json:"repo" yaml:"repo"`
	Tracker   forge.IssueTracker        `json:"-" yaml:"-"`
	Reader    forge.RepoReader          `json:"-" yaml:"-"`
	PRManager forge.PullRequestManager  `json:"-" yaml:"-"`
}

// ProjectRegistry manages the set of available projects.
type ProjectRegistry struct {
	mu       sync.RWMutex
	projects map[string]*Project
	active   string // name of the currently active project
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

// ActiveName returns the name of the currently active project.
func (r *ProjectRegistry) ActiveName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.active
}

// ProjectConfig is the YAML-serializable configuration for a project.
type ProjectConfig struct {
	Name  string `yaml:"name"`
	Owner string `yaml:"owner"`
	Repo  string `yaml:"repo"`
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
	for i, p := range cfg.Projects {
		if p.Name == "" {
			return nil, fmt.Errorf("project at index %d: name is required", i)
		}
		if p.Owner == "" {
			return nil, fmt.Errorf("project %q: owner is required", p.Name)
		}
		if p.Repo == "" {
			return nil, fmt.Errorf("project %q: repo is required", p.Name)
		}
	}

	return cfg.Projects, nil
}
