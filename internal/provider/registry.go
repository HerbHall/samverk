package provider

import (
	"context"
	"errors"
	"fmt"
	"os"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// ErrNoHealthyProvider is returned when no provider in the routing chain is healthy.
var ErrNoHealthyProvider = errors.New("no healthy provider available")

// ProviderConfig describes a single provider entry in YAML config.
type ProviderConfig struct {
	Type           string `yaml:"type"`            // "claude", "openai", "ollama", "claude-cli"
	APIKeyEnv      string `yaml:"api_key_env"`     // env var name for API key
	BaseURL        string `yaml:"base_url"`        // override base URL (for ollama or custom endpoints)
	DefaultModel   string `yaml:"default_model"`   // default model for this provider
	TimeoutSeconds int    `yaml:"timeout_seconds"` // per-provider timeout; 0 means use provider default
	AccountURL     string `yaml:"account_url"`     // billing/credits page URL
	AllowedTools   string `yaml:"allowed_tools"`   // claude-cli: comma-separated tool list (e.g. "Bash,Read,Edit,Write,Glob,Grep")
	MaxTurns       int    `yaml:"max_turns"`       // claude-cli: max agentic turns per session; 0 means no limit
	WoLMAC         string `yaml:"wol_mac"`         // optional Wake-on-LAN MAC address (e.g. "AA:BB:CC:DD:EE:FF")
	GPU            bool   `yaml:"gpu"`             // true if the provider host has GPU access for model inference
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
	providers  map[string]Provider  // name -> Provider instance
	models     map[string]string    // name -> default model
	types      map[string]string    // name -> provider type
	routing    map[string][]string  // agent_type -> provider names
	gpuEnabled map[string]bool      // name -> true if provider host has GPU
	logger     *zap.Logger
}

// NewRegistry creates an empty provider registry.
func NewRegistry(logger *zap.Logger) *Registry {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Registry{
		providers:  make(map[string]Provider),
		models:     make(map[string]string),
		types:      make(map[string]string),
		routing:    make(map[string][]string),
		gpuEnabled: make(map[string]bool),
		logger:     logger,
	}
}

// Register adds a named provider to the registry.
func (r *Registry) Register(name, providerType string, p Provider, model string) {
	r.providers[name] = p
	r.models[name] = model
	r.types[name] = providerType
	r.logger.Info("provider registered",
		zap.String("name", name),
		zap.String("type", providerType),
		zap.String("model", model),
	)
}

// SetRouting sets the routing table that maps agent types to provider chains.
func (r *Registry) SetRouting(routing map[string][]string) {
	r.routing = routing
}

// SetGPUFlags configures which providers have GPU acceleration available.
// Providers not in the map default to GPU-disabled. Called by LoadRegistry
// after all providers are registered.
func (r *Registry) SetGPUFlags(flags map[string]bool) {
	r.gpuEnabled = flags
}

// IsGPUEnabled returns whether the named provider has GPU acceleration.
func (r *Registry) IsGPUEnabled(name string) bool {
	return r.gpuEnabled[name]
}

// Get returns the first healthy provider from the routing chain for the given
// agent type. If the agent type is not configured, it falls back to "default".
//
// GPU preference: when at least one GPU-enabled provider in the chain is healthy,
// non-GPU providers are skipped. If no GPU provider is healthy, chain order is
// used as-is. This preserves operator-defined chain priority while avoiding
// CPU-only inference when a GPU alternative is available.
//
// Returns (provider, model, error).
func (r *Registry) Get(ctx context.Context, agentType string) (Provider, string, error) {
	chain, ok := r.routing[agentType]
	if !ok {
		chain, ok = r.routing["default"]
		if !ok {
			r.logger.Error("no routing chain found",
				zap.String("agent_type", agentType),
			)
			return nil, "", ErrNoHealthyProvider
		}
		r.logger.Info("routing chain fallback to default",
			zap.String("agent_type", agentType),
		)
	}

	hasHealthyGPU := r.chainHasHealthyGPU(ctx, chain)

	for _, name := range chain {
		p, exists := r.providers[name]
		if !exists {
			r.logger.Warn("provider in chain not registered",
				zap.String("agent_type", agentType),
				zap.String("provider", name),
			)
			continue
		}
		if hasHealthyGPU && !r.gpuEnabled[name] {
			r.logger.Debug("skipping non-GPU provider (GPU alternative available)",
				zap.String("agent_type", agentType),
				zap.String("provider", name),
			)
			continue
		}
		if p.Healthy(ctx) {
			r.logger.Info("provider selected",
				zap.String("agent_type", agentType),
				zap.String("provider", name),
				zap.String("model", r.models[name]),
				zap.Bool("gpu", r.gpuEnabled[name]),
			)
			return p, r.models[name], nil
		}
		r.logger.Warn("provider unhealthy, trying next",
			zap.String("agent_type", agentType),
			zap.String("provider", name),
		)
	}

	r.logger.Error("all providers in chain unhealthy",
		zap.String("agent_type", agentType),
		zap.Strings("chain", chain),
	)
	return nil, "", ErrNoHealthyProvider
}

// chainHasHealthyGPU checks whether at least one GPU-enabled provider in the
// chain is currently healthy. Used to decide whether non-GPU providers should
// be skipped during routing.
func (r *Registry) chainHasHealthyGPU(ctx context.Context, chain []string) bool {
	for _, name := range chain {
		if !r.gpuEnabled[name] {
			continue
		}
		if p, ok := r.providers[name]; ok && p.Healthy(ctx) {
			return true
		}
	}
	return false
}

// GetAfter returns the first healthy provider in the routing chain that comes
// after the named provider. This enables timeout failover: when a provider
// times out, the pool can retry with the next one in the chain without
// re-checking providers already tried.
//
// GPU preference applies only to providers after the cursor. If no GPU
// provider after the cursor is healthy, non-GPU providers are used as fallback.
//
// Returns ErrNoHealthyProvider if no subsequent healthy provider exists.
func (r *Registry) GetAfter(ctx context.Context, agentType, afterProvider string) (p Provider, model string, err error) {
	chain, ok := r.routing[agentType]
	if !ok {
		chain, ok = r.routing["default"]
		if !ok {
			return nil, "", ErrNoHealthyProvider
		}
	}

	// Build the tail of the chain after the cursor.
	tail := chainAfter(chain, afterProvider)

	hasHealthyGPU := r.chainHasHealthyGPU(ctx, tail)

	for _, name := range tail {
		prov, exists := r.providers[name]
		if !exists {
			continue
		}
		if hasHealthyGPU && !r.gpuEnabled[name] {
			r.logger.Debug("failover: skipping non-GPU provider",
				zap.String("agent_type", agentType),
				zap.String("provider", name),
			)
			continue
		}
		if prov.Healthy(ctx) {
			r.logger.Info("failover provider selected",
				zap.String("agent_type", agentType),
				zap.String("after", afterProvider),
				zap.String("provider", name),
				zap.String("model", r.models[name]),
				zap.Bool("gpu", r.gpuEnabled[name]),
			)
			return prov, r.models[name], nil
		}
		r.logger.Warn("failover provider unhealthy, trying next",
			zap.String("agent_type", agentType),
			zap.String("provider", name),
		)
	}

	return nil, "", ErrNoHealthyProvider
}

// chainAfter returns the portion of the chain after the named provider.
func chainAfter(chain []string, after string) []string {
	for i, name := range chain {
		if name == after && i+1 < len(chain) {
			return chain[i+1:]
		}
	}
	return nil
}

// Routing returns a copy of the routing table mapping chain names to provider names.
func (r *Registry) Routing() map[string][]string {
	out := make(map[string][]string, len(r.routing))
	for k, v := range r.routing {
		chain := make([]string, len(v))
		copy(chain, v)
		out[k] = chain
	}
	return out
}

// NameOf returns the registry name for a given Provider instance, or empty
// string if not found. Used by the pool to identify which provider timed
// out for failover via GetAfter.
func (r *Registry) NameOf(p Provider) string {
	for name, prov := range r.providers {
		if prov == p {
			return name
		}
	}
	return ""
}

// ProviderByName returns the raw Provider instance for the given name,
// or nil if the name is not registered.
func (r *Registry) ProviderByName(name string) Provider {
	return r.providers[name]
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

// codeGenChainNames lists routing chain names that should never contain
// Ollama providers. Ollama models produce bad output on tool-use formatted
// prompts -- they overwrite CLAUDE.md instead of implementing features.
// Note: "default" was removed in #113 after ValidateWorkspaceOutput runtime
// protection was verified sufficient (qwen3-coder:30b promoted to code-gen).
var codeGenChainNames = map[string]bool{
	"complex":  true,
	"frontend": true,
	"local":    true,
}

// ValidateRoutingConfig checks that Ollama providers are not present in
// code-gen routing chains. Returns a list of warnings for each violation.
// This is a soft validation -- it logs warnings but does not prevent startup.
func ValidateRoutingConfig(cfg *RegistryConfig, logger *zap.Logger) []string {
	if cfg == nil || logger == nil {
		return nil
	}

	// Build set of ollama provider names.
	ollamaProviders := make(map[string]bool)
	for name := range cfg.Providers {
		if cfg.Providers[name].Type == "ollama" {
			ollamaProviders[name] = true
		}
	}

	if len(ollamaProviders) == 0 {
		return nil
	}

	var warnings []string
	for chainName, chain := range cfg.Routing {
		if !codeGenChainNames[chainName] {
			continue
		}
		for _, providerName := range chain {
			if ollamaProviders[providerName] {
				msg := fmt.Sprintf(
					"routing chain %q contains Ollama provider %q; Ollama restricted to triage-only pending validation (see #57)",
					chainName, providerName,
				)
				warnings = append(warnings, msg)
				logger.Warn("ollama in code-gen chain",
					zap.String("chain", chainName),
					zap.String("provider", providerName),
				)
			}
		}
	}
	return warnings
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
func LoadRegistry(path string, factory ProviderFactory, logger *zap.Logger) (*Registry, error) {
	cfg, err := LoadRegistryConfig(path)
	if err != nil {
		return nil, err
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	logger.Info("loading provider registry",
		zap.String("path", path),
		zap.Int("provider_count", len(cfg.Providers)),
	)

	reg := NewRegistry(logger)

	for name := range cfg.Providers {
		pcfg := cfg.Providers[name]
		p, err := factory(name, pcfg)
		if err != nil {
			return nil, fmt.Errorf("create provider %q: %w", name, err)
		}
		reg.Register(name, pcfg.Type, p, pcfg.DefaultModel)
	}

	// Wire GPU flags from config into the registry.
	gpuFlags := make(map[string]bool, len(cfg.Providers))
	for name := range cfg.Providers {
		gpuFlags[name] = cfg.Providers[name].GPU
	}
	reg.SetGPUFlags(gpuFlags)

	if cfg.Routing != nil {
		reg.SetRouting(cfg.Routing)
		logger.Info("routing table loaded",
			zap.Int("chain_count", len(cfg.Routing)),
		)
		// Warn about Ollama providers in code-gen chains (soft validation).
		ValidateRoutingConfig(cfg, logger)
	}

	return reg, nil
}
