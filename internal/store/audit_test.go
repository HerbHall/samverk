package store

import (
	"context"
	"testing"
	"time"

	"samverk.dev/samverk/internal/audit"
)

func TestSaveAndGetAuditResults(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	results := []audit.AuditResult{
		{
			ProviderName: "ollama-local",
			ProviderType: "ollama",
			Model:        "qwen2.5:7b",
			Healthy:      true,
			Latency:      42 * time.Millisecond,
			AuditedAt:    now,
		},
		{
			ProviderName: "claude-api",
			ProviderType: "claude",
			Model:        "claude-sonnet",
			Healthy:      false,
			Latency:      1500 * time.Millisecond,
			Error:        "health check failed",
			AuditedAt:    now,
		},
	}

	if err := s.SaveAuditResults(ctx, results); err != nil {
		t.Fatalf("save audit results: %v", err)
	}

	got, err := s.GetLastAudit(ctx)
	if err != nil {
		t.Fatalf("get last audit: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}

	// Results are ordered by provider_name.
	if got[0].ProviderName != "claude-api" {
		t.Errorf("first result name = %q, want %q", got[0].ProviderName, "claude-api")
	}
	if got[0].Healthy {
		t.Error("claude-api should be unhealthy")
	}
	if got[0].Error != "health check failed" {
		t.Errorf("error = %q, want %q", got[0].Error, "health check failed")
	}

	if got[1].ProviderName != "ollama-local" {
		t.Errorf("second result name = %q, want %q", got[1].ProviderName, "ollama-local")
	}
	if !got[1].Healthy {
		t.Error("ollama-local should be healthy")
	}
	if got[1].Latency != 42*time.Millisecond {
		t.Errorf("latency = %v, want %v", got[1].Latency, 42*time.Millisecond)
	}
}

func TestGetLastAuditEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	got, err := s.GetLastAudit(ctx)
	if err != nil {
		t.Fatalf("get last audit: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 results, got %d", len(got))
	}
}

func TestGetLastAuditReturnsLatest(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)

	// Save old results.
	oldResults := []audit.AuditResult{
		{ProviderName: "old-provider", ProviderType: "ollama", Model: "old-model", Healthy: true, AuditedAt: old},
	}
	if err := s.SaveAuditResults(ctx, oldResults); err != nil {
		t.Fatalf("save old results: %v", err)
	}

	// Save recent results.
	recentResults := []audit.AuditResult{
		{ProviderName: "new-provider", ProviderType: "claude", Model: "new-model", Healthy: false, Error: "down", AuditedAt: recent},
	}
	if err := s.SaveAuditResults(ctx, recentResults); err != nil {
		t.Fatalf("save recent results: %v", err)
	}

	got, err := s.GetLastAudit(ctx)
	if err != nil {
		t.Fatalf("get last audit: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result (latest only), got %d", len(got))
	}
	if got[0].ProviderName != "new-provider" {
		t.Errorf("got %q, want latest provider %q", got[0].ProviderName, "new-provider")
	}
}
