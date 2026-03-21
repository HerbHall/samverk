// Package advisor implements the advisory engine that detects quality patterns
// from failure events and produces actionable recommendations.
package advisor

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/pkg/models"
)

// StoreReader is the minimal store interface the advisor needs.
type StoreReader interface {
	ListFailureEvents(ctx context.Context, since time.Time, limit int) ([]*models.FailureEvent, error)
	GetKPIReport(ctx context.Context) (*models.KPIReport, error)
}

// Advisor runs in the background and periodically refreshes quality recommendations.
type Advisor struct {
	store  StoreReader
	logger *zap.Logger

	mu   sync.RWMutex
	recs []models.QualityRecommendation
}

// New creates a new Advisor. Call Start to begin background detection.
func New(st StoreReader, logger *zap.Logger) *Advisor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Advisor{
		store:  st,
		logger: logger,
		recs:   []models.QualityRecommendation{},
	}
}

// Start runs the advisory engine in a background loop, refreshing every 15 minutes.
// It returns when ctx is cancelled.
func (a *Advisor) Start(ctx context.Context) error {
	// Run once immediately on start.
	a.detect(ctx)

	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			a.detect(ctx)
		}
	}
}

// Recommendations returns a copy of the current advisory recommendations.
func (a *Advisor) Recommendations() []models.QualityRecommendation {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if len(a.recs) == 0 {
		return []models.QualityRecommendation{}
	}
	out := make([]models.QualityRecommendation, len(a.recs))
	copy(out, a.recs)
	return out
}

// detect fetches data and runs all detectors, storing the results.
func (a *Advisor) detect(ctx context.Context) {
	since30d := time.Now().UTC().Add(-30 * 24 * time.Hour)
	events, err := a.store.ListFailureEvents(ctx, since30d, 1000)
	if err != nil {
		a.logger.Warn("advisory: failed to list failure events", zap.Error(err))
		return
	}

	kpi30d, err := a.store.GetKPIReport(ctx)
	if err != nil {
		a.logger.Warn("advisory: failed to get KPI report", zap.Error(err))
		return
	}

	recs := make([]models.QualityRecommendation, 0, len(events))
	recs = append(recs, detectRecurrence(events)...)
	recs = append(recs, detectStagnantWorkarounds(events)...)
	recs = append(recs, detectUnknownRootCause(events)...)
	recs = append(recs, detectPreventionGaps(events)...)
	recs = append(recs, detectKPIRegression(kpi30d)...)

	a.mu.Lock()
	a.recs = recs
	a.mu.Unlock()

	a.logger.Debug("advisory: detection complete", zap.Int("recommendations", len(recs)))
}
