package provider

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DefaultHealthCheckInterval is the default interval between health probe cycles.
const DefaultHealthCheckInterval = 60 * time.Second

// ProviderHealth describes the health status of a single provider.
type ProviderHealth struct {
	Name        string    `json:"name"`
	Healthy     bool      `json:"healthy"`
	LastChecked time.Time `json:"last_checked"`
	LastHealthy time.Time `json:"last_healthy"`
	Error       string    `json:"error,omitempty"`
	// Ollama-specific fields (zero values for non-Ollama providers).
	ModelLoaded bool  `json:"model_loaded,omitempty"`
	VRAMFree    int64 `json:"vram_free_bytes,omitempty"`
	VRAMTotal   int64 `json:"vram_total_bytes,omitempty"`
}

// HealthDetailer is an optional interface that providers may implement
// to expose richer health information (e.g., model load status, VRAM usage).
type HealthDetailer interface {
	HealthDetail(ctx context.Context) (*HealthDetail, error)
}

// HealthDetail contains extended health information from providers that
// support it (currently Ollama).
type HealthDetail struct {
	ModelLoaded bool
	VRAMFree    int64
	VRAMTotal   int64
}

// HealthMonitor runs periodic health checks against all providers in
// a Registry and caches the results for fast, lock-free reads by the
// dispatcher and API layer.
type HealthMonitor struct {
	registry *Registry
	interval time.Duration
	logger   *zap.Logger

	mu     sync.RWMutex
	health map[string]*ProviderHealth // keyed by provider name

	wolTargets  map[string]WoLConfig // provider name -> WoL config
	wolCooldown *wolCooldownTracker
}

// NewHealthMonitor creates a HealthMonitor that probes all providers in
// the registry at the given interval. Use Start to begin the probe loop.
func NewHealthMonitor(registry *Registry, interval time.Duration, logger *zap.Logger) *HealthMonitor {
	if interval <= 0 {
		interval = DefaultHealthCheckInterval
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &HealthMonitor{
		registry:    registry,
		interval:    interval,
		logger:      logger,
		health:      make(map[string]*ProviderHealth),
		wolTargets:  make(map[string]WoLConfig),
		wolCooldown: newWoLCooldownTracker(),
	}
}

// SetWoLTargets configures Wake-on-LAN targets for providers. When a
// provider with a configured WoL target is unhealthy, the monitor sends
// a magic packet (subject to a 5-minute cooldown per target).
func (hm *HealthMonitor) SetWoLTargets(targets map[string]WoLConfig) {
	hm.wolTargets = targets
}

// Start runs the health check loop in a goroutine. It performs an initial
// probe immediately and then repeats at the configured interval. The loop
// exits when ctx is cancelled.
func (hm *HealthMonitor) Start(ctx context.Context) {
	// Initial probe before starting the ticker so that callers can rely
	// on health data being available shortly after Start returns.
	hm.probeAll(ctx)

	go func() {
		ticker := time.NewTicker(hm.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				hm.probeAll(ctx)
			}
		}
	}()
}

// GetHealth returns the cached health status for a named provider.
// Returns a zero-value ProviderHealth (Healthy=false) if the provider
// has not been probed yet.
func (hm *HealthMonitor) GetHealth(name string) ProviderHealth {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	if h, ok := hm.health[name]; ok {
		return *h
	}
	return ProviderHealth{Name: name}
}

// AllHealth returns cached health for all probed providers.
func (hm *HealthMonitor) AllHealth() []ProviderHealth {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	out := make([]ProviderHealth, 0, len(hm.health))
	for _, h := range hm.health {
		out = append(out, *h)
	}
	return out
}

// ChainHealthy returns true if at least one provider in the named
// routing chain is healthy according to cached probe results.
func (hm *HealthMonitor) ChainHealthy(chain []string) bool {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	for _, name := range chain {
		if h, ok := hm.health[name]; ok && h.Healthy {
			return true
		}
	}
	return false
}

// probeAll checks every registered provider and updates the cache.
func (hm *HealthMonitor) probeAll(ctx context.Context) {
	providers := hm.registry.List(ctx)

	hm.mu.Lock()
	defer hm.mu.Unlock()

	now := time.Now()
	for i := range providers {
		info := &providers[i]
		name := info.Name

		existing, ok := hm.health[name]
		if !ok {
			existing = &ProviderHealth{Name: name}
			hm.health[name] = existing
		}

		existing.LastChecked = now
		existing.Healthy = info.Healthy
		existing.Error = ""

		if info.Healthy {
			existing.LastHealthy = now
		} else {
			// Send WoL packet if the provider is unhealthy and has a
			// configured target, subject to cooldown.
			hm.tryWakeOnLAN(name)
		}

		// Attempt extended health detail for providers that support it.
		p := hm.registry.ProviderByName(name)
		if p == nil {
			continue
		}
		if hd, ok := p.(HealthDetailer); ok {
			detail, err := hd.HealthDetail(ctx)
			if err != nil {
				existing.Error = err.Error()
				hm.logger.Debug("health detail probe failed",
					zap.String("provider", name),
					zap.Error(err),
				)
				continue
			}
			if detail != nil {
				existing.ModelLoaded = detail.ModelLoaded
				existing.VRAMFree = detail.VRAMFree
				existing.VRAMTotal = detail.VRAMTotal
			}
		}
	}

	hm.logger.Debug("health probes completed",
		zap.Int("providers", len(providers)),
		zap.Time("at", now),
	)
}

// tryWakeOnLAN sends a WoL magic packet for the named provider if it has
// a configured WoL target and the cooldown has expired.
func (hm *HealthMonitor) tryWakeOnLAN(name string) {
	wol, ok := hm.wolTargets[name]
	if !ok || wol.MAC == "" {
		return
	}

	if !hm.wolCooldown.shouldSend(wol.MAC) {
		hm.logger.Debug("WoL cooldown active, skipping",
			zap.String("provider", name),
			zap.String("mac", wol.MAC),
		)
		return
	}

	hm.logger.Info("sending Wake-on-LAN packet for unhealthy provider",
		zap.String("provider", name),
		zap.String("mac", wol.MAC),
	)
	if err := WakeOnLAN(wol.MAC); err != nil {
		hm.logger.Warn("Wake-on-LAN failed",
			zap.String("provider", name),
			zap.String("mac", wol.MAC),
			zap.Error(err),
		)
	}
}
