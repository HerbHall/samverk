package provider

import (
	"context"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ErrNoHealthyProvider is returned when no provider in the routing chain is healthy.
var ErrNoHealthyProvider = errors.New("no healthy provider available")

// ProviderConfig describes a single provider entry in YAML config.
type ProviderConfig struct {
	Type         string `yaml:"type"`          // "claude", "openai", "ollama"
	APIKeyEnv    string `yaml:"api_key_env"`   // env var name for API key
	BaseURL      string `yaml:"base_url"`      // override base URL (for ollama or custom endpoints)
	DefaultModel string `yaml:"default_model"` // default model for this provider
}

// RegistryConfig is the top-level YAML structure for provider configuration.
type RegistryConfig struct {
	Providers map[string]ProviderConfig `yaml:"providers"`
	Routing   map[string][]string       `yaml:"routing"` // agent_type -> provider names
}

// ProviderInfo describes a registered provider for listing.
type ProviderInfo struct {
	Name    string
	Type    string
	Model   string
	Healthy bool
}

// ProviderFactory constructs a Provider from a ProviderConfig.
// It is set by the application's wiring layer (e.g., cmd/samverk) to avoid
// the registry importing sub-packages directly in tests.
type ProviderFactory func(name string, cfg ProviderConfig) (Provider, error)

// Registry holds constructed providers and routing rules.
type Registry struct {
	providers map[string]Provider  // name -> Provider instance
	models    map[string]string    // name -> default model
	types     map[string]string    // name -> provider type
	routing   map[string][]string  // agent_type -> provider names
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
		models:    make(map[string]string),
		types:     make(map[string]string),
		routing:   make(map[string][]string),
	}
}

// Register adds a named provider to the registry.
func (r *Registry) Register(name, providerType string, p Provider, model string) {
	r.providers[name] = p
	r.models[name] = model
	r.types[name] = providerType
}

// SetRouting sets the routing table that maps agent types to provider chains.
func (r *Registry) SetRouting(routing map[string][]string) {
	r.routing = routing
}

// Get returns the first healthy provider from the routing chain for the given
// agent type. If the agent type is not configured, it falls back to "default".
// Returns (provider, model, error).
func (r *Registry) Get(ctx context.Context, agentType string) (Provider, string, error) {
	chain, ok := r.routing[agentType]
	if !ok {
		chain, ok = r.routing["default"]
		if !ok {
			return nil, "", ErrNoHealthyProvider
		}
	}

	for _, name := range chain {
		p, exists := r.providers[name]
		if !exists {
			continue
		}
		if p.Healthy(ctx) {
			return p, r.models[name], nil
		}
	}

	return nil, "", ErrNoHealthyProvider
}

// List returns info about all registered providers with their health status.
func (r *Registry) List(ctx context.Context) []ProviderInfo {
	infos := make([]ProviderInfo, 0, len(r.providers))
	for name, p := range r.providers {
		infos = append(infos, ProviderInfo{
			Name:    name,
			Type:    r.types[name],
			Model:   r.models[name],
			Healthy: p.Healthy(ctx),
		})
	}
	return infos
}

// LoadRegistryConfig reads and parses a YAML config file.
func LoadRegistryConfig(path string) (*RegistryConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is from trusted config, not user input
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg RegistryConfig
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	return &cfg, nil
}

// LoadRegistry reads a YAML config file, constructs providers using the
// given factory, and returns a fully wired Registry.
func LoadRegistry(path string, factory ProviderFactory) (*Registry, error) {
	cfg, err := LoadRegistryConfig(path)
	if err != nil {
		return nil, err
	}

	reg := NewRegistry()

	for name, pcfg := range cfg.Providers {
		p, err := factory(name, pcfg)
		if err != nil {
			return nil, fmt.Errorf("create provider %q: %w", name, err)
		}
		reg.Register(name, pcfg.Type, p, pcfg.DefaultModel)
	}

	if cfg.Routing != nil {
		reg.SetRouting(cfg.Routing)
	}

	return reg, nil
}
