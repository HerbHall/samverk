package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
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

func TestGetAfter_ReturnsNextHealthy(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	p1 := &mockProvider{name: "ollama-nzxt", healthy: true}
	p2 := &mockProvider{name: "claude-cli", healthy: true}
	p3 := &mockProvider{name: "claude-api", healthy: true}

	reg.Register("ollama-nzxt", "ollama", p1, "qwen3-coder:30b")
	reg.Register("claude-cli", "claude-cli", p2, "claude-sonnet-4-6")
	reg.Register("claude-api", "claude", p3, "claude-sonnet-4-6")
	reg.SetRouting(map[string][]string{
		"default": {"ollama-nzxt", "claude-cli", "claude-api"},
	})

	got, model, err := reg.GetAfter(ctx, "default", "ollama-nzxt")
	if err != nil {
		t.Fatalf("GetAfter: unexpected error: %v", err)
	}
	if got != p2 {
		t.Errorf("GetAfter: expected claude-cli, got %s", got.Name())
	}
	if model != "claude-sonnet-4-6" {
		t.Errorf("GetAfter: model = %q, want %q", model, "claude-sonnet-4-6")
	}
}

func TestGetAfter_SkipsUnhealthyNext(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	p1 := &mockProvider{name: "first", healthy: true}
	p2 := &mockProvider{name: "second", healthy: false}
	p3 := &mockProvider{name: "third", healthy: true}

	reg.Register("first", "ollama", p1, "model-1")
	reg.Register("second", "claude-cli", p2, "model-2")
	reg.Register("third", "claude", p3, "model-3")
	reg.SetRouting(map[string][]string{
		"default": {"first", "second", "third"},
	})

	got, model, err := reg.GetAfter(ctx, "default", "first")
	if err != nil {
		t.Fatalf("GetAfter: unexpected error: %v", err)
	}
	if got != p3 {
		t.Errorf("GetAfter: expected third (skip unhealthy second), got %s", got.Name())
	}
	if model != "model-3" {
		t.Errorf("GetAfter: model = %q, want %q", model, "model-3")
	}
}

func TestGetAfter_NoNextProvider(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	p1 := &mockProvider{name: "only", healthy: true}
	reg.Register("only", "claude-cli", p1, "model-1")
	reg.SetRouting(map[string][]string{
		"default": {"only"},
	})

	_, _, err := reg.GetAfter(ctx, "default", "only")
	if !errors.Is(err, ErrNoHealthyProvider) {
		t.Errorf("GetAfter: expected ErrNoHealthyProvider when last in chain, got %v", err)
	}
}

func TestGetAfter_ProviderNotInChain(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	p1 := &mockProvider{name: "a", healthy: true}
	reg.Register("a", "ollama", p1, "model-a")
	reg.SetRouting(map[string][]string{
		"default": {"a"},
	})

	_, _, err := reg.GetAfter(ctx, "default", "nonexistent")
	if !errors.Is(err, ErrNoHealthyProvider) {
		t.Errorf("GetAfter: expected ErrNoHealthyProvider for unknown provider, got %v", err)
	}
}

func TestNameOf_Found(t *testing.T) {
	reg := NewRegistry(nil)
	p := &mockProvider{name: "claude-cli", healthy: true}
	reg.Register("my-claude", "claude-cli", p, "model")

	got := reg.NameOf(p)
	if got != "my-claude" {
		t.Errorf("NameOf: got %q, want %q", got, "my-claude")
	}
}

func TestNameOf_NotFound(t *testing.T) {
	reg := NewRegistry(nil)
	p := &mockProvider{name: "unknown", healthy: true}

	got := reg.NameOf(p)
	if got != "" {
		t.Errorf("NameOf: got %q, want empty", got)
	}
}

func TestValidateRoutingConfig_OllamaInCodeGenChain(t *testing.T) {
	t.Parallel()

	// "default" was removed from codeGenChainNames in #113 -- ollama is now
	// allowed there. "complex" remains blocked.
	cfg := &RegistryConfig{
		Providers: map[string]ProviderConfig{
			"claude-sonnet": {Type: "claude-cli", DefaultModel: "claude-sonnet-4-6"},
			"ollama-coder":  {Type: "ollama", DefaultModel: "qwen2.5-coder:14b"},
		},
		Routing: map[string][]string{
			"default": {"ollama-coder", "claude-sonnet"},
			"complex": {"ollama-coder", "claude-sonnet"},
			"triage":  {"ollama-coder", "claude-sonnet"},
		},
	}

	logger, _ := zap.NewDevelopment()
	warnings := ValidateRoutingConfig(cfg, logger)

	// Should warn about ollama in "complex" but not in "default" or "triage".
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "complex") {
		t.Errorf("warning should mention 'complex' chain, got: %s", warnings[0])
	}
	if !strings.Contains(warnings[0], "ollama-coder") {
		t.Errorf("warning should mention 'ollama-coder', got: %s", warnings[0])
	}
}

func TestValidateRoutingConfig_NoOllamaProviders(t *testing.T) {
	t.Parallel()

	cfg := &RegistryConfig{
		Providers: map[string]ProviderConfig{
			"claude-sonnet": {Type: "claude-cli", DefaultModel: "claude-sonnet-4-6"},
		},
		Routing: map[string][]string{
			"default": {"claude-sonnet"},
		},
	}

	logger, _ := zap.NewDevelopment()
	warnings := ValidateRoutingConfig(cfg, logger)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %d: %v", len(warnings), warnings)
	}
}

func TestValidateRoutingConfig_OllamaInTriageOnly(t *testing.T) {
	t.Parallel()

	cfg := &RegistryConfig{
		Providers: map[string]ProviderConfig{
			"claude-sonnet": {Type: "claude-cli", DefaultModel: "claude-sonnet-4-6"},
			"ollama-coder":  {Type: "ollama", DefaultModel: "qwen2.5-coder:7b"},
		},
		Routing: map[string][]string{
			"default": {"claude-sonnet"},
			"triage":  {"ollama-coder", "claude-sonnet"},
			"qc":      {"ollama-coder", "claude-sonnet"},
		},
	}

	logger, _ := zap.NewDevelopment()
	warnings := ValidateRoutingConfig(cfg, logger)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for triage/qc-only ollama, got %d: %v", len(warnings), warnings)
	}
}

// --- GPU-aware routing tests ---

func TestGet_GPUPreference_SkipsCPUWhenGPUHealthy(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	cpu := &mockProvider{name: "cpu-ollama", healthy: true}
	gpu := &mockProvider{name: "gpu-ollama", healthy: true}

	reg.Register("cpu-ollama", "ollama", cpu, "qwen2.5-coder:7b")
	reg.Register("gpu-ollama", "ollama", gpu, "qwen3-coder:30b")
	reg.SetRouting(map[string][]string{
		"default": {"cpu-ollama", "gpu-ollama"},
	})
	reg.SetGPUFlags(map[string]bool{
		"cpu-ollama": false,
		"gpu-ollama": true,
	})

	got, model, err := reg.Get(ctx, "default")
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if got != gpu {
		t.Errorf("Get: expected GPU provider, got CPU provider")
	}
	if model != "qwen3-coder:30b" {
		t.Errorf("Get: model = %q, want %q", model, "qwen3-coder:30b")
	}
}

func TestGet_GPUPreference_FallsThroughWhenGPUUnhealthy(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	cpu := &mockProvider{name: "cpu-ollama", healthy: true}
	gpu := &mockProvider{name: "gpu-ollama", healthy: false}

	reg.Register("cpu-ollama", "ollama", cpu, "qwen2.5-coder:7b")
	reg.Register("gpu-ollama", "ollama", gpu, "qwen3-coder:30b")
	reg.SetRouting(map[string][]string{
		"default": {"cpu-ollama", "gpu-ollama"},
	})
	reg.SetGPUFlags(map[string]bool{
		"cpu-ollama": false,
		"gpu-ollama": true,
	})

	got, model, err := reg.Get(ctx, "default")
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if got != cpu {
		t.Errorf("Get: expected CPU fallback, got different provider")
	}
	if model != "qwen2.5-coder:7b" {
		t.Errorf("Get: model = %q, want %q", model, "qwen2.5-coder:7b")
	}
}

func TestGet_NoGPUInChain_ExistingBehaviorUnchanged(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	first := &mockProvider{name: "first", healthy: true}
	second := &mockProvider{name: "second", healthy: true}

	reg.Register("first", "ollama", first, "model-a")
	reg.Register("second", "claude", second, "model-b")
	reg.SetRouting(map[string][]string{
		"default": {"first", "second"},
	})
	// No GPU flags set — all default to false.

	got, _, err := reg.Get(ctx, "default")
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if got != first {
		t.Errorf("Get: expected first provider (chain order), got second")
	}
}

func TestGet_AllGPUChain_FirstHealthyGPUWins(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	gpu1 := &mockProvider{name: "gpu1", healthy: false}
	gpu2 := &mockProvider{name: "gpu2", healthy: true}
	gpu3 := &mockProvider{name: "gpu3", healthy: true}

	reg.Register("gpu1", "ollama", gpu1, "model-1")
	reg.Register("gpu2", "ollama", gpu2, "model-2")
	reg.Register("gpu3", "ollama", gpu3, "model-3")
	reg.SetRouting(map[string][]string{
		"default": {"gpu1", "gpu2", "gpu3"},
	})
	reg.SetGPUFlags(map[string]bool{
		"gpu1": true,
		"gpu2": true,
		"gpu3": true,
	})

	got, model, err := reg.Get(ctx, "default")
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if got != gpu2 {
		t.Errorf("Get: expected gpu2 (first healthy GPU), got different")
	}
	if model != "model-2" {
		t.Errorf("Get: model = %q, want %q", model, "model-2")
	}
}

func TestGetAfter_GPUPreference_RespectseCursor(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	gpu1 := &mockProvider{name: "gpu1", healthy: true}
	cpu := &mockProvider{name: "cpu", healthy: true}
	gpu2 := &mockProvider{name: "gpu2", healthy: true}

	reg.Register("gpu1", "ollama", gpu1, "model-1")
	reg.Register("cpu", "ollama", cpu, "model-cpu")
	reg.Register("gpu2", "ollama", gpu2, "model-2")
	reg.SetRouting(map[string][]string{
		"default": {"gpu1", "cpu", "gpu2"},
	})
	reg.SetGPUFlags(map[string]bool{
		"gpu1": true,
		"cpu":  false,
		"gpu2": true,
	})

	// After gpu1 (cursor past it) — should pick gpu2, skipping cpu.
	got, model, err := reg.GetAfter(ctx, "default", "gpu1")
	if err != nil {
		t.Fatalf("GetAfter: unexpected error: %v", err)
	}
	if got != gpu2 {
		t.Errorf("GetAfter: expected gpu2 (GPU after cursor), got different")
	}
	if model != "model-2" {
		t.Errorf("GetAfter: model = %q, want %q", model, "model-2")
	}
}

func TestGetAfter_GPUPreference_FallbackToCPUAfterCursor(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)

	gpu1 := &mockProvider{name: "gpu1", healthy: true}
	cpu := &mockProvider{name: "cpu", healthy: true}
	gpu2 := &mockProvider{name: "gpu2", healthy: false}

	reg.Register("gpu1", "ollama", gpu1, "model-1")
	reg.Register("cpu", "ollama", cpu, "model-cpu")
	reg.Register("gpu2", "ollama", gpu2, "model-2")
	reg.SetRouting(map[string][]string{
		"default": {"gpu1", "cpu", "gpu2"},
	})
	reg.SetGPUFlags(map[string]bool{
		"gpu1": true,
		"cpu":  false,
		"gpu2": true,
	})

	// After gpu1, gpu2 unhealthy — should fall back to cpu.
	got, model, err := reg.GetAfter(ctx, "default", "gpu1")
	if err != nil {
		t.Fatalf("GetAfter: unexpected error: %v", err)
	}
	if got != cpu {
		t.Errorf("GetAfter: expected cpu fallback, got different")
	}
	if model != "model-cpu" {
		t.Errorf("GetAfter: model = %q, want %q", model, "model-cpu")
	}
}

func TestIsGPUEnabled(t *testing.T) {
	reg := NewRegistry(nil)
	reg.SetGPUFlags(map[string]bool{
		"ollama-nzxt": true,
		"claude-api":  false,
	})

	if !reg.IsGPUEnabled("ollama-nzxt") {
		t.Error("IsGPUEnabled: expected true for ollama-nzxt")
	}
	if reg.IsGPUEnabled("claude-api") {
		t.Error("IsGPUEnabled: expected false for claude-api")
	}
	if reg.IsGPUEnabled("nonexistent") {
		t.Error("IsGPUEnabled: expected false for nonexistent provider")
	}
}

func TestChainAfter(t *testing.T) {
	chain := []string{"a", "b", "c", "d"}

	tail := chainAfter(chain, "b")
	if len(tail) != 2 || tail[0] != "c" || tail[1] != "d" {
		t.Errorf("chainAfter(b): got %v, want [c d]", tail)
	}

	tail = chainAfter(chain, "d")
	if tail != nil {
		t.Errorf("chainAfter(d): got %v, want nil (last element)", tail)
	}

	tail = chainAfter(chain, "nonexistent")
	if tail != nil {
		t.Errorf("chainAfter(nonexistent): got %v, want nil", tail)
	}
}

func TestValidateRoutingConfig_NilInputs(t *testing.T) {
	t.Parallel()

	// Nil config.
	warnings := ValidateRoutingConfig(nil, zap.NewNop())
	if warnings != nil {
		t.Errorf("expected nil for nil config, got %v", warnings)
	}

	// Nil logger.
	warnings = ValidateRoutingConfig(&RegistryConfig{}, nil)
	if warnings != nil {
		t.Errorf("expected nil for nil logger, got %v", warnings)
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
