// Package loganalyst queries structured logs and produces AI-powered or
// fallback natural-language summaries of agent sessions, issues, failures,
// and arbitrary time ranges.
package loganalyst

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/logstore"
)

// ScopeType identifies what dimension of logs to summarize.
type ScopeType string

const (
	// ScopeSession summarizes logs for a single agent session.
	ScopeSession ScopeType = "session"
	// ScopeIssue summarizes logs related to a specific issue number.
	ScopeIssue ScopeType = "issue"
	// ScopeTimeRange summarizes logs within an explicit time window.
	ScopeTimeRange ScopeType = "timerange"
	// ScopeFailures summarizes only error-level logs since a given time.
	ScopeFailures ScopeType = "failures"
)

// ScopeFilter selects which logs to analyze.
type ScopeFilter struct {
	Type  ScopeType `json:"type"`
	ID    string    `json:"id"`    // session ID or issue number as string
	Since time.Time `json:"since"` // for timerange/failures
	Until time.Time `json:"until"` // for timerange
}

// Summary is the result of a log analysis.
type Summary struct {
	Scope        ScopeFilter `json:"scope"`
	Text         string      `json:"text"`
	TotalEntries int         `json:"total_entries"`
	ErrorCount   int         `json:"error_count"`
	WarnCount    int         `json:"warn_count"`
	Components   []string    `json:"components"`
	AIGenerated  bool        `json:"ai_generated"`
	GeneratedAt  time.Time   `json:"generated_at"`
}

// LogQuerier abstracts the logstore for testing.
type LogQuerier interface {
	Query(ctx context.Context, f logstore.QueryFilter) ([]logstore.LogEntry, error)
}

// OllamaClient abstracts the Ollama generate API for testing.
type OllamaClient interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// Analyst coordinates log querying, AI summarization, and caching.
type Analyst struct {
	logs   LogQuerier
	ollama OllamaClient // may be nil; fallback to structured summary
	cache  sync.Map     // key: string -> *cacheEntry
	logger *zap.Logger
}

// New creates an Analyst. If ollama is nil the analyst produces
// structured fallback summaries instead of AI-generated text.
func New(logs LogQuerier, ollama OllamaClient, logger *zap.Logger) *Analyst {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Analyst{
		logs:   logs,
		ollama: ollama,
		logger: logger,
	}
}

// Summarize returns a natural-language summary for the given scope.
// Results are cached for 5 minutes.
func (a *Analyst) Summarize(ctx context.Context, scope ScopeFilter) (summary *Summary, err error) {
	key := cacheKey(scope)

	if cached, ok := a.getCached(key); ok {
		return cached, nil
	}

	filter, buildErr := buildFilter(scope)
	if buildErr != nil {
		return nil, buildErr
	}

	entries, err := a.logs.Query(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("query logs: %w", err)
	}

	stats := computeStats(entries)

	var text string
	var aiGenerated bool

	if a.ollama != nil {
		prompt := buildPrompt(entries, scope)
		aiText, genErr := a.ollama.Generate(ctx, prompt)
		if genErr != nil {
			a.logger.Warn("ollama generate failed, using fallback",
				zap.Error(genErr),
				zap.String("scope_type", string(scope.Type)),
			)
			text = buildFallbackSummary(entries, scope)
		} else {
			text = strings.TrimSpace(aiText)
			aiGenerated = true
		}
	} else {
		text = buildFallbackSummary(entries, scope)
	}

	summary = &Summary{
		Scope:        scope,
		Text:         text,
		TotalEntries: stats.total,
		ErrorCount:   stats.errors,
		WarnCount:    stats.warnings,
		Components:   stats.components,
		AIGenerated:  aiGenerated,
		GeneratedAt:  time.Now().UTC(),
	}

	a.setCache(key, summary)
	return summary, nil
}

// buildFilter converts a ScopeFilter into a logstore.QueryFilter.
func buildFilter(scope ScopeFilter) (f logstore.QueryFilter, err error) {
	f.Limit = 200

	switch scope.Type {
	case ScopeSession:
		if scope.ID == "" {
			return f, fmt.Errorf("session scope requires an id")
		}
		f.SessionID = scope.ID

	case ScopeIssue:
		if scope.ID == "" {
			return f, fmt.Errorf("issue scope requires an id")
		}
		var n int
		n, err = strconv.Atoi(scope.ID)
		if err != nil {
			return f, fmt.Errorf("invalid issue number %q: %w", scope.ID, err)
		}
		f.Issue = n

	case ScopeTimeRange:
		if scope.Since.IsZero() {
			return f, fmt.Errorf("timerange scope requires since")
		}
		f.Since = scope.Since
		if !scope.Until.IsZero() {
			f.Until = scope.Until
		}

	case ScopeFailures:
		if scope.Since.IsZero() {
			return f, fmt.Errorf("failures scope requires since")
		}
		f.Level = "error"
		f.Since = scope.Since

	default:
		return f, fmt.Errorf("unknown scope type %q", scope.Type)
	}

	return f, nil
}

// logStats holds aggregate counts from a set of log entries.
type logStats struct {
	total      int
	errors     int
	warnings   int
	components []string
}

// computeStats counts errors, warnings, and unique components.
func computeStats(entries []logstore.LogEntry) logStats {
	s := logStats{total: len(entries)}
	seen := make(map[string]struct{})

	for i := range entries {
		switch entries[i].Level {
		case "error":
			s.errors++
		case "warn":
			s.warnings++
		}
		c := entries[i].Component
		if c != "" {
			if _, ok := seen[c]; !ok {
				seen[c] = struct{}{}
				s.components = append(s.components, c)
			}
		}
	}
	return s
}
