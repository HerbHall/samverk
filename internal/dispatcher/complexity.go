package dispatcher

import (
	"context"
	"strings"
	"time"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/pkg/models"
	"go.uber.org/zap"
)

// Default timeout bounds.
const (
	DefaultTimeout = 15 * time.Minute
	MinTimeout     = 5 * time.Minute
	MaxTimeout     = 60 * time.Minute
)

// complexitySignals captures the raw signals extracted from an issue.
type complexitySignals struct {
	WordCount      int
	CheckboxCount  int
	FileRefCount   int
	HasCodeBlocks  bool
	AgentType      models.AgentType
	ProviderKey    string
	Labels         map[string]bool
}

// extractSignals gathers complexity-relevant signals from an issue.
func extractSignals(issue *forge.Issue, agentType models.AgentType, providerKey string) complexitySignals {
	labels := make(map[string]bool, len(issue.Labels))
	for _, l := range issue.Labels {
		labels[l] = true
	}

	body := issue.Body
	return complexitySignals{
		WordCount:     len(strings.Fields(body)),
		CheckboxCount: countCheckboxes(body),
		FileRefCount:  countFileRefs(body),
		HasCodeBlocks: strings.Contains(body, "```"),
		AgentType:     agentType,
		ProviderKey:   providerKey,
		Labels:        labels,
	}
}

// EstimateTimeout returns a per-issue timeout based on complexity signals.
// If the frontmatter specifies timeout_minutes, that value is used directly
// (clamped to [MinTimeout, MaxTimeout]). Otherwise, heuristic signals
// determine the timeout.
func EstimateTimeout(issue *forge.Issue, fm *models.IssueFrontmatter, agentType models.AgentType, providerKey string) time.Duration {
	// Frontmatter override takes priority.
	if fm != nil && fm.TimeoutMinutes > 0 {
		d := time.Duration(fm.TimeoutMinutes) * time.Minute
		return clampTimeout(d)
	}

	sig := extractSignals(issue, agentType, providerKey)
	return estimateFromSignals(sig)
}

// estimateFromSignals computes a timeout from heuristic signals.
func estimateFromSignals(sig complexitySignals) time.Duration {
	minutes := DefaultTimeout.Minutes()

	// Body length factor: +1 min per 100 words above 200.
	if sig.WordCount > 200 {
		minutes += float64(sig.WordCount-200) / 100.0
	}

	// Checkbox factor: +2 min per checkbox (each is a discrete subtask).
	minutes += float64(sig.CheckboxCount) * 2.0

	// File reference factor: +1 min per referenced file.
	minutes += float64(sig.FileRefCount) * 1.0

	// Code blocks suggest implementation detail.
	if sig.HasCodeBlocks {
		minutes += 3.0
	}

	// Agent type adjustments.
	switch sig.AgentType {
	case models.AgentTypeResearch:
		minutes *= 1.5 // research tasks tend to run longer
	case models.AgentTypeDocs:
		minutes *= 0.7 // docs are typically faster
	case models.AgentTypeQC:
		minutes *= 0.5 // QC is validation, usually quick
	case models.AgentTypeOrchestrator, models.AgentTypeDispatcher,
		models.AgentTypeCodeGen, models.AgentTypeTest,
		models.AgentTypeHuman, models.AgentTypeInfra, models.AgentTypePC:
		// default multiplier (1.0) for all other agent types
	}

	// Provider key adjustments.
	switch sig.ProviderKey {
	case "complex":
		minutes *= 1.3 // complex routing implies harder work
	case "local":
		minutes *= 0.8 // local models are faster but less capable
	case "triage":
		minutes *= 0.6 // triage is quick classification
	}

	// Label overrides.
	if sig.Labels["complexity:high"] {
		minutes = max(minutes, 30.0)
	}
	if sig.Labels[models.LabelPriorityCritical] {
		minutes *= 1.2 // critical tasks get more headroom
	}

	return clampTimeout(time.Duration(minutes * float64(time.Minute)))
}

// clampTimeout constrains a duration to [MinTimeout, MaxTimeout].
func clampTimeout(d time.Duration) time.Duration {
	if d < MinTimeout {
		return MinTimeout
	}
	if d > MaxTimeout {
		return MaxTimeout
	}
	return d
}

// countCheckboxes counts markdown task list items (- [ ] and - [x]).
func countCheckboxes(body string) int {
	count := 0
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ]") || strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]") {
			count++
		}
	}
	return count
}

// minCalibrationSamples is the minimum number of completed sessions required
// before historical data replaces heuristic timeout estimation.
const minCalibrationSamples = 20

// calibrationSafetyMargin is the multiplier applied to p90 duration to provide
// headroom for variance in task completion time.
const calibrationSafetyMargin = 1.2

// CalibratedTimeout returns a timeout based on historical session data when
// enough samples exist for the (agentType, provider) pair. It uses p90
// duration with a 20% safety margin, clamped to [MinTimeout, MaxTimeout].
// Falls back to heuristic estimation when insufficient data is available.
func CalibratedTimeout(
	ctx context.Context,
	st store.Store,
	logger *zap.Logger,
	issue *forge.Issue,
	fm *models.IssueFrontmatter,
	agentType models.AgentType,
	providerKey string,
) time.Duration {
	// Frontmatter override always takes priority.
	if fm != nil && fm.TimeoutMinutes > 0 {
		return clampTimeout(time.Duration(fm.TimeoutMinutes) * time.Minute)
	}

	profile, err := st.GetTaskProfile(ctx, string(agentType), providerKey)
	if err != nil {
		logger.Warn("calibration: failed to get task profile, falling back to heuristic",
			zap.String("agent_type", string(agentType)),
			zap.String("provider", providerKey),
			zap.Error(err),
		)
		return EstimateTimeout(issue, fm, agentType, providerKey)
	}

	if profile != nil && profile.SampleCount >= minCalibrationSamples {
		calibrated := time.Duration(float64(profile.P90Duration) * calibrationSafetyMargin)
		heuristic := EstimateTimeout(issue, fm, agentType, providerKey)
		result := clampTimeout(calibrated)

		logger.Info("timeout calibrated from historical data",
			zap.String("agent_type", string(agentType)),
			zap.String("provider", providerKey),
			zap.Int("samples", profile.SampleCount),
			zap.Duration("p90", profile.P90Duration),
			zap.Duration("calibrated", result),
			zap.Duration("heuristic", heuristic),
		)
		return result
	}

	return EstimateTimeout(issue, fm, agentType, providerKey)
}

// countFileRefs counts file path references matching common project directories.
func countFileRefs(body string) int {
	prefixes := []string{"internal/", "cmd/", "pkg/", "docs/", "web/", "scripts/"}
	seen := make(map[string]bool)
	for _, word := range strings.Fields(body) {
		// Strip markdown formatting.
		clean := strings.Trim(word, "`*_[](){}\"',;:")
		for _, p := range prefixes {
			if strings.HasPrefix(clean, p) && !seen[clean] {
				seen[clean] = true
				break
			}
		}
	}
	return len(seen)
}
