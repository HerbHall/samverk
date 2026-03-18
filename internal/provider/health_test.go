package provider_test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/provider"
)

// mockProvider implements provider.Provider for testing.
type mockProvider struct {
	name    string
	healthy bool
}

func (m *mockProvider) Chat(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
	return nil, nil
}

func (m *mockProvider) Healthy(_ context.Context) bool {
	return m.healthy
}

func (m *mockProvider) Name() string {
	return m.name
}

func TestHealthMonitor_GetHealth(t *testing.T) {
	reg := provider.NewRegistry(zap.NewNop())
	p := &mockProvider{name: "test-ollama", healthy: true}
	reg.Register("test-ollama", "ollama", p, "qwen2.5:7b")

	hm := provider.NewHealthMonitor(reg, time.Hour, zap.NewNop())

	// Before Start, GetHealth returns zero value.
	h := hm.GetHealth("test-ollama")
	if h.Healthy {
		t.Fatal("expected unhealthy before start")
	}

	// Start runs an initial probe synchronously before the goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hm.Start(ctx)

	// Give a moment for the initial probe (it runs inline in Start before the goroutine).
	time.Sleep(50 * time.Millisecond)

	h = hm.GetHealth("test-ollama")
	if !h.Healthy {
		t.Fatal("expected healthy after start")
	}
	if h.LastChecked.IsZero() {
		t.Fatal("expected LastChecked to be set")
	}
	if h.LastHealthy.IsZero() {
		t.Fatal("expected LastHealthy to be set")
	}
}

func TestHealthMonitor_UnhealthyProvider(t *testing.T) {
	reg := provider.NewRegistry(zap.NewNop())
	p := &mockProvider{name: "bad-provider", healthy: false}
	reg.Register("bad-provider", "ollama", p, "model")

	hm := provider.NewHealthMonitor(reg, time.Hour, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hm.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	h := hm.GetHealth("bad-provider")
	if h.Healthy {
		t.Fatal("expected unhealthy")
	}
	if h.LastHealthy.IsZero() == false {
		t.Fatal("expected LastHealthy to remain zero for never-healthy provider")
	}
}

func TestHealthMonitor_ChainHealthy(t *testing.T) {
	reg := provider.NewRegistry(zap.NewNop())
	reg.Register("healthy-one", "ollama", &mockProvider{name: "healthy-one", healthy: true}, "m1")
	reg.Register("unhealthy-one", "ollama", &mockProvider{name: "unhealthy-one", healthy: false}, "m2")

	hm := provider.NewHealthMonitor(reg, time.Hour, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hm.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	if !hm.ChainHealthy([]string{"unhealthy-one", "healthy-one"}) {
		t.Fatal("chain should be healthy when at least one provider is healthy")
	}
	if hm.ChainHealthy([]string{"unhealthy-one"}) {
		t.Fatal("chain should be unhealthy when all providers are unhealthy")
	}
	if hm.ChainHealthy([]string{"nonexistent"}) {
		t.Fatal("chain should be unhealthy for unknown providers")
	}
}

func TestHealthMonitor_AllHealth(t *testing.T) {
	reg := provider.NewRegistry(zap.NewNop())
	reg.Register("p1", "ollama", &mockProvider{name: "p1", healthy: true}, "m1")
	reg.Register("p2", "claude", &mockProvider{name: "p2", healthy: false}, "m2")

	hm := provider.NewHealthMonitor(reg, time.Hour, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hm.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	all := hm.AllHealth()
	if len(all) != 2 {
		t.Fatalf("expected 2 health entries, got %d", len(all))
	}

	healthMap := make(map[string]bool)
	for i := range all {
		healthMap[all[i].Name] = all[i].Healthy
	}
	if !healthMap["p1"] {
		t.Fatal("p1 should be healthy")
	}
	if healthMap["p2"] {
		t.Fatal("p2 should be unhealthy")
	}
}

func TestHealthMonitor_UnknownProvider(t *testing.T) {
	reg := provider.NewRegistry(zap.NewNop())
	hm := provider.NewHealthMonitor(reg, time.Hour, zap.NewNop())

	h := hm.GetHealth("nonexistent")
	if h.Healthy {
		t.Fatal("unknown provider should be unhealthy")
	}
	if h.Name != "nonexistent" {
		t.Fatalf("expected name 'nonexistent', got %q", h.Name)
	}
}
