// Package audit provides health-check and freshness auditing for AI providers.
package audit

import (
	"context"
	"time"

	"github.com/herbhall/samverk/internal/provider"
)

// AuditResult holds the outcome of a single provider health check.
type AuditResult struct {
	ProviderName string        `json:"provider_name"`
	ProviderType string        `json:"provider_type"`
	Model        string        `json:"model"`
	Healthy      bool          `json:"healthy"`
	Latency      time.Duration `json:"latency"`
	Error        string        `json:"error,omitempty"`
	AuditedAt    time.Time     `json:"audited_at"`
}

// ProviderAudit runs health checks against a set of providers.
type ProviderAudit struct {
	infos []provider.ProviderInfo
	provs []provider.Provider
}

// New creates a ProviderAudit from a list of ProviderInfo and their corresponding
// Provider instances. The infos slice provides metadata (name, type, model) while
// the provs slice provides the actual health-check targets. Both slices must have
// the same length and be in the same order.
func New(infos []provider.ProviderInfo, provs []provider.Provider) *ProviderAudit {
	return &ProviderAudit{
		infos: infos,
		provs: provs,
	}
}

// Run executes health checks on all providers and returns the results.
func (pa *ProviderAudit) Run(ctx context.Context) ([]AuditResult, error) {
	now := time.Now().UTC()
	results := make([]AuditResult, 0, len(pa.provs))

	for i, p := range pa.provs {
		info := pa.infos[i]
		r := AuditResult{
			ProviderName: info.Name,
			ProviderType: info.Type,
			Model:        info.Model,
			AuditedAt:    now,
		}

		start := time.Now()
		r.Healthy = p.Healthy(ctx)
		r.Latency = time.Since(start)

		if !r.Healthy {
			r.Error = "health check failed"
		}

		results = append(results, r)
	}

	return results, nil
}
