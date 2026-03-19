package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/api"
	"github.com/herbhall/samverk/internal/provider"
)

// mockHealthProvider implements provider.Provider for health monitor tests.
type mockHealthProvider struct {
	healthy bool
}

func (m *mockHealthProvider) Chat(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
	return nil, nil
}

func (m *mockHealthProvider) Healthy(_ context.Context) bool {
	return m.healthy
}

func (m *mockHealthProvider) Name() string {
	return "mock"
}

// healthTestStore extends mockStore with a configurable LatestSuccessByProvider return value.
type healthTestStore struct {
	mockStore
	lastSuccess map[string]time.Time
}

func (s *healthTestStore) LatestSuccessByProvider(_ context.Context) (map[string]time.Time, error) {
	return s.lastSuccess, nil
}

// buildHealthAPI creates an API with a health monitor and optional provider registry,
// starts the health monitor, and returns an httptest server.
func buildHealthAPI(
	t *testing.T,
	st *healthTestStore,
	providerNames []string,
	providerHealthy bool,
	reg *provider.Registry,
) *httptest.Server {
	t.Helper()

	provReg := provider.NewRegistry(zap.NewNop())
	for _, name := range providerNames {
		p := &mockHealthProvider{healthy: providerHealthy}
		provReg.Register(name, "claude", p, "claude-3")
	}

	a := api.New(nil, st, nil, zap.NewNop())

	hm := provider.NewHealthMonitor(provReg, time.Hour, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	hm.Start(ctx)
	// Brief wait for the initial probe to complete synchronously.
	time.Sleep(20 * time.Millisecond)

	a.SetHealthMonitor(hm)
	if reg != nil {
		a.SetProviderRegistry(reg)
	}

	mux := http.NewServeMux()
	a.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestProviderHealth_LastSuccessAt(t *testing.T) {
	successTime := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	st := &healthTestStore{lastSuccess: map[string]time.Time{"claude-1": successTime}}

	ts := buildHealthAPI(t, st, []string{"claude-1"}, true, nil)

	resp := doGet(t, ts.URL+"/api/v1/providers/health")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]any
	decodeJSON(t, resp, &body)

	providers, ok := body["providers"].([]any)
	if !ok || len(providers) == 0 {
		t.Fatalf("expected providers array, got %v", body["providers"])
	}

	entry, ok := providers[0].(map[string]any)
	if !ok {
		t.Fatalf("expected provider entry map, got %T", providers[0])
	}

	got, ok := entry["last_success_at"].(string)
	if !ok {
		t.Fatalf("last_success_at missing or not a string: %v", entry["last_success_at"])
	}

	want := successTime.UTC().Format(time.RFC3339)
	if got != want {
		t.Errorf("last_success_at = %q, want %q", got, want)
	}
}

func TestProviderHealth_NeverSucceeded(t *testing.T) {
	// Empty map: provider is known but has no successful sessions.
	st := &healthTestStore{lastSuccess: map[string]time.Time{}}

	ts := buildHealthAPI(t, st, []string{"claude-1"}, true, nil)

	resp := doGet(t, ts.URL+"/api/v1/providers/health")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]any
	decodeJSON(t, resp, &body)

	providers, ok := body["providers"].([]any)
	if !ok || len(providers) == 0 {
		t.Fatalf("expected providers array, got %v", body["providers"])
	}

	entry, ok := providers[0].(map[string]any)
	if !ok {
		t.Fatalf("expected provider entry map")
	}

	got, ok := entry["last_success_at"].(string)
	if !ok {
		t.Fatalf("last_success_at missing or not a string: %v", entry["last_success_at"])
	}
	if got != "never" {
		t.Errorf("last_success_at = %q, want %q", got, "never")
	}
}

func TestProviderHealth_RoutingChains(t *testing.T) {
	st := &healthTestStore{lastSuccess: map[string]time.Time{}}

	routingReg := provider.NewRegistry(zap.NewNop())
	routingReg.SetRouting(map[string][]string{
		"default": {"claude-1"},
		"triage":  {"claude-1"},
	})

	ts := buildHealthAPI(t, st, []string{"claude-1"}, true, routingReg)

	resp := doGet(t, ts.URL+"/api/v1/providers/health")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]any
	decodeJSON(t, resp, &body)

	chains, ok := body["routing_chains"].(map[string]any)
	if !ok || len(chains) == 0 {
		t.Fatalf("routing_chains missing or empty: %v", body["routing_chains"])
	}

	if _, hasDefault := chains["default"]; !hasDefault {
		t.Error("routing_chains should contain 'default' chain")
	}
	if _, hasTriage := chains["triage"]; !hasTriage {
		t.Error("routing_chains should contain 'triage' chain")
	}
}

func TestProviderHealth_NoRegistry(t *testing.T) {
	st := &healthTestStore{lastSuccess: map[string]time.Time{}}

	// Pass nil registry — routing_chains should be absent from response.
	ts := buildHealthAPI(t, st, []string{"claude-1"}, true, nil)

	resp := doGet(t, ts.URL+"/api/v1/providers/health")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]any
	decodeJSON(t, resp, &body)

	if _, hasChains := body["routing_chains"]; hasChains {
		t.Error("routing_chains should be absent when no registry is set")
	}

	if _, ok := body["providers"]; !ok {
		t.Error("providers field should be present")
	}
	if _, ok := body["count"]; !ok {
		t.Error("count field should be present")
	}
}
