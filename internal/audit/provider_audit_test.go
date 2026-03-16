package audit

import (
	"context"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/provider"
)

// mockProvider implements provider.Provider for testing.
type mockProvider struct {
	name    string
	healthy bool
	delay   time.Duration
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Healthy(ctx context.Context) bool {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return false
		}
	}
	return m.healthy
}

func (m *mockProvider) Chat(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
	return nil, nil
}

func TestRunHealthy(t *testing.T) {
	infos := []provider.ProviderInfo{
		{Name: "test-ollama", Type: "ollama", Model: "qwen2.5:7b"},
	}
	provs := []provider.Provider{
		&mockProvider{name: "test-ollama", healthy: true},
	}

	pa := New(infos, provs)
	results, err := pa.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.ProviderName != "test-ollama" {
		t.Errorf("provider name = %q, want %q", r.ProviderName, "test-ollama")
	}
	if r.ProviderType != "ollama" {
		t.Errorf("provider type = %q, want %q", r.ProviderType, "ollama")
	}
	if r.Model != "qwen2.5:7b" {
		t.Errorf("model = %q, want %q", r.Model, "qwen2.5:7b")
	}
	if !r.Healthy {
		t.Error("expected healthy = true")
	}
	if r.Error != "" {
		t.Errorf("expected no error, got %q", r.Error)
	}
	if r.Latency < 0 {
		t.Errorf("expected non-negative latency, got %v", r.Latency)
	}
}

func TestRunUnhealthy(t *testing.T) {
	infos := []provider.ProviderInfo{
		{Name: "dead-provider", Type: "claude", Model: "claude-sonnet"},
	}
	provs := []provider.Provider{
		&mockProvider{name: "dead-provider", healthy: false},
	}

	pa := New(infos, provs)
	results, err := pa.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := results[0]
	if r.Healthy {
		t.Error("expected healthy = false")
	}
	if r.Error == "" {
		t.Error("expected error message for unhealthy provider")
	}
}

func TestRunTimeout(t *testing.T) {
	infos := []provider.ProviderInfo{
		{Name: "slow-provider", Type: "ollama", Model: "llama3"},
	}
	provs := []provider.Provider{
		&mockProvider{name: "slow-provider", healthy: true, delay: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	pa := New(infos, provs)
	results, err := pa.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := results[0]
	if r.Healthy {
		t.Error("expected healthy = false due to context timeout")
	}
}

func TestRunMultipleProviders(t *testing.T) {
	infos := []provider.ProviderInfo{
		{Name: "healthy-one", Type: "ollama", Model: "qwen2.5:7b"},
		{Name: "dead-one", Type: "claude", Model: "claude-sonnet"},
		{Name: "healthy-two", Type: "openai", Model: "gpt-4o"},
	}
	provs := []provider.Provider{
		&mockProvider{name: "healthy-one", healthy: true},
		&mockProvider{name: "dead-one", healthy: false},
		&mockProvider{name: "healthy-two", healthy: true},
	}

	pa := New(infos, provs)
	results, err := pa.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	tests := []struct {
		name    string
		healthy bool
	}{
		{"healthy-one", true},
		{"dead-one", false},
		{"healthy-two", true},
	}
	for i, tt := range tests {
		if results[i].ProviderName != tt.name {
			t.Errorf("result[%d].ProviderName = %q, want %q", i, results[i].ProviderName, tt.name)
		}
		if results[i].Healthy != tt.healthy {
			t.Errorf("result[%d].Healthy = %v, want %v", i, results[i].Healthy, tt.healthy)
		}
	}
}
