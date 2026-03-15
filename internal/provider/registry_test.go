package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// mockProvider is a test double implementing Provider.
type mockProvider struct {
	name    string
	healthy bool
}

func (m *mockProvider) Chat(_ context.Context, _ ChatRequest) (*ChatResponse, error) {
	return &ChatResponse{}, nil
}

func (m *mockProvider) Healthy(_ context.Context) bool { return m.healthy }
func (m *mockProvider) Name() string                   { return m.name }

func TestNewRegistry_RegisterAndGet(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	p := &mockProvider{name: "test-claude", healthy: true}
	reg.Register("claude", "claude", p, "claude-sonnet-4-20250514")
	reg.SetRouting(map[string][]string{
		"default": {"claude"},
	})

	got, model, err := reg.Get(ctx, "default")
	if err != nil {
		t.Fatalf("Get(default): unexpected error: %v", err)
	}
	if got != p {
		t.Errorf("Get(default): got different provider instance")
	}
	if model != "claude-sonnet-4-20250514" {
		t.Errorf("Get(default): model = %q, want %q", model, "claude-sonnet-4-20250514")
	}
}

func TestGet_FallbackToHealthy(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	unhealthy := &mockProvider{name: "claude", healthy: false}
	healthy := &mockProvider{name: "openai", healthy: true}

	reg.Register("claude", "claude", unhealthy, "claude-sonnet-4-20250514")
	reg.Register("openai", "openai", healthy, "gpt-4o")
	reg.SetRouting(map[string][]string{
		"default": {"claude", "openai"},
	})

	got, model, err := reg.Get(ctx, "default")
	if err != nil {
		t.Fatalf("Get(default): unexpected error: %v", err)
	}
	if got != healthy {
		t.Errorf("Get(default): expected fallback to healthy provider")
	}
	if model != "gpt-4o" {
		t.Errorf("Get(default): model = %q, want %q", model, "gpt-4o")
	}
}

func TestGet_NoHealthyProvider(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	reg.Register("claude", "claude", &mockProvider{name: "claude", healthy: false}, "model-a")
	reg.Register("openai", "openai", &mockProvider{name: "openai", healthy: false}, "model-b")
	reg.SetRouting(map[string][]string{
		"default": {"claude", "openai"},
	})

	_, _, err := reg.Get(ctx, "default")
	if !errors.Is(err, ErrNoHealthyProvider) {
		t.Errorf("Get(default): err = %v, want ErrNoHealthyProvider", err)
	}
}

func TestGet_FallbackToDefaultRouting(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	p := &mockProvider{name: "ollama", healthy: true}
	reg.Register("ollama", "ollama", p, "qwen2.5-coder:14b")
	reg.SetRouting(map[string][]string{
		"default": {"ollama"},
	})

	got, model, err := reg.Get(ctx, "unknown_agent_type")
	if err != nil {
		t.Fatalf("Get(unknown_agent_type): unexpected error: %v", err)
	}
	if got != p {
		t.Errorf("Get(unknown_agent_type): expected fallback to default routing")
	}
	if model != "qwen2.5-coder:14b" {
		t.Errorf("Get(unknown_agent_type): model = %q, want %q", model, "qwen2.5-coder:14b")
	}
}

func TestGet_SpecificAgentTypeRouting(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	claude := &mockProvider{name: "claude", healthy: true}
	ollama := &mockProvider{name: "ollama", healthy: true}

	reg.Register("claude", "claude", claude, "claude-sonnet-4-20250514")
	reg.Register("ollama", "ollama", ollama, "qwen2.5-coder:14b")
	reg.SetRouting(map[string][]string{
		"default":       {"claude", "ollama"},
		"quick_triage":  {"ollama"},
	})

	got, model, err := reg.Get(ctx, "quick_triage")
	if err != nil {
		t.Fatalf("Get(quick_triage): unexpected error: %v", err)
	}
	if got != ollama {
		t.Errorf("Get(quick_triage): expected ollama, got %s", got.Name())
	}
	if model != "qwen2.5-coder:14b" {
		t.Errorf("Get(quick_triage): model = %q, want %q", model, "qwen2.5-coder:14b")
	}
}

func TestGet_NoRoutingConfigured(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	reg.Register("claude", "claude", &mockProvider{name: "claude", healthy: true}, "model-a")
	// No routing set at all.

	_, _, err := reg.Get(ctx, "anything")
	if !errors.Is(err, ErrNoHealthyProvider) {
		t.Errorf("Get(anything) with no routing: err = %v, want ErrNoHealthyProvider", err)
	}
}

func TestGet_SkipsMissingProviderName(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	p := &mockProvider{name: "openai", healthy: true}
	reg.Register("openai", "openai", p, "gpt-4o")
	reg.SetRouting(map[string][]string{
		"default": {"nonexistent", "openai"},
	})

	got, _, err := reg.Get(ctx, "default")
	if err != nil {
		t.Fatalf("Get(default): unexpected error: %v", err)
	}
	if got != p {
		t.Errorf("Get(default): expected openai after skipping nonexistent")
	}
}

func TestList_ReturnsAllProviders(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	reg.Register("claude", "claude", &mockProvider{name: "claude", healthy: true}, "claude-sonnet-4-20250514")
	reg.Register("openai", "openai", &mockProvider{name: "openai", healthy: false}, "gpt-4o")
	reg.Register("ollama", "ollama", &mockProvider{name: "ollama", healthy: true}, "qwen2.5-coder:14b")

	infos := reg.List(ctx)
	if len(infos) != 3 {
		t.Fatalf("List(): got %d providers, want 3", len(infos))
	}

	byName := make(map[string]ProviderInfo)
	for _, info := range infos {
		byName[info.Name] = info
	}

	tests := []struct {
		name    string
		wantTyp string
		wantMod string
		wantOK  bool
	}{
		{"claude", "claude", "claude-sonnet-4-20250514", true},
		{"openai", "openai", "gpt-4o", false},
		{"ollama", "ollama", "qwen2.5-coder:14b", true},
	}

	for _, tt := range tests {
		info, ok := byName[tt.name]
		if !ok {
			t.Errorf("List(): missing provider %q", tt.name)
			continue
		}
		if info.Type != tt.wantTyp {
			t.Errorf("List()[%s].Type = %q, want %q", tt.name, info.Type, tt.wantTyp)
		}
		if info.Model != tt.wantMod {
			t.Errorf("List()[%s].Model = %q, want %q", tt.name, info.Model, tt.wantMod)
		}
		if info.Healthy != tt.wantOK {
			t.Errorf("List()[%s].Healthy = %v, want %v", tt.name, info.Healthy, tt.wantOK)
		}
	}
}

func TestLoadRegistryConfig(t *testing.T) {
	content := `providers:
  claude:
    type: claude
    api_key_env: ANTHROPIC_API_KEY
    default_model: claude-sonnet-4-20250514
  openai:
    type: openai
    api_key_env: OPENAI_API_KEY
    default_model: gpt-4o
  ollama:
    type: ollama
    base_url: http://192.168.1.207:11434
    default_model: qwen2.5-coder:14b

routing:
  default: [claude, openai, ollama]
  code_review: [claude]
  quick_triage: [ollama, openai]
`
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := LoadRegistryConfig(path)
	if err != nil {
		t.Fatalf("LoadRegistryConfig: %v", err)
	}

	if len(cfg.Providers) != 3 {
		t.Errorf("Providers count = %d, want 3", len(cfg.Providers))
	}

	claude, ok := cfg.Providers["claude"]
	if !ok {
		t.Fatal("missing claude provider config")
	}
	if claude.Type != "claude" {
		t.Errorf("claude.Type = %q, want %q", claude.Type, "claude")
	}
	if claude.APIKeyEnv != "ANTHROPIC_API_KEY" {
		t.Errorf("claude.APIKeyEnv = %q, want %q", claude.APIKeyEnv, "ANTHROPIC_API_KEY")
	}
	if claude.DefaultModel != "claude-sonnet-4-20250514" {
		t.Errorf("claude.DefaultModel = %q, want %q", claude.DefaultModel, "claude-sonnet-4-20250514")
	}

	ollama, ok := cfg.Providers["ollama"]
	if !ok {
		t.Fatal("missing ollama provider config")
	}
	if ollama.BaseURL != "http://192.168.1.207:11434" {
		t.Errorf("ollama.BaseURL = %q, want %q", ollama.BaseURL, "http://192.168.1.207:11434")
	}

	if len(cfg.Routing) != 3 {
		t.Errorf("Routing count = %d, want 3", len(cfg.Routing))
	}

	defaultRoute := cfg.Routing["default"]
	if len(defaultRoute) != 3 {
		t.Errorf("default routing len = %d, want 3", len(defaultRoute))
	}

	codeReview := cfg.Routing["code_review"]
	if len(codeReview) != 1 || codeReview[0] != "claude" {
		t.Errorf("code_review routing = %v, want [claude]", codeReview)
	}
}

func TestLoadRegistryConfig_FileNotFound(t *testing.T) {
	_, err := LoadRegistryConfig("/nonexistent/providers.yaml")
	if err == nil {
		t.Fatal("LoadRegistryConfig: expected error for missing file")
	}
}

func TestLoadRegistryConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{invalid yaml"), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	_, err := LoadRegistryConfig(path)
	if err == nil {
		t.Fatal("LoadRegistryConfig: expected error for invalid YAML")
	}
}

func TestLoadRegistry_WithFactory(t *testing.T) {
	content := `providers:
  test-provider:
    type: ollama
    base_url: http://localhost:11434
    default_model: test-model

routing:
  default: [test-provider]
`
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	factoryCalled := false
	factory := func(name string, cfg ProviderConfig) (Provider, error) {
		factoryCalled = true
		if name != "test-provider" {
			t.Errorf("factory name = %q, want %q", name, "test-provider")
		}
		if cfg.Type != "ollama" {
			t.Errorf("factory cfg.Type = %q, want %q", cfg.Type, "ollama")
		}
		if cfg.BaseURL != "http://localhost:11434" {
			t.Errorf("factory cfg.BaseURL = %q, want %q", cfg.BaseURL, "http://localhost:11434")
		}
		return &mockProvider{name: name, healthy: true}, nil
	}

	reg, err := LoadRegistry(path, factory, nil)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if !factoryCalled {
		t.Error("LoadRegistry: factory was not called")
	}

	ctx := context.Background()
	p, model, err := reg.Get(ctx, "default")
	if err != nil {
		t.Fatalf("Get(default): %v", err)
	}
	if p.Name() != "test-provider" {
		t.Errorf("Get(default): provider name = %q, want %q", p.Name(), "test-provider")
	}
	if model != "test-model" {
		t.Errorf("Get(default): model = %q, want %q", model, "test-model")
	}
}

func TestLoadRegistry_FactoryError(t *testing.T) {
	content := `providers:
  broken:
    type: unknown
    default_model: x

routing:
  default: [broken]
`
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	factory := func(_ string, _ ProviderConfig) (Provider, error) {
		return nil, errors.New("unsupported provider type")
	}

	_, err := LoadRegistry(path, factory, nil)
	if err == nil {
		t.Fatal("LoadRegistry: expected error from factory")
	}
}
