package loganalyst

import (
	"fmt"
	"time"
)

const cacheTTL = 5 * time.Minute

// cacheEntry wraps a summary with its creation timestamp.
type cacheEntry struct {
	summary   *Summary
	createdAt time.Time
}

// cacheKey builds a deterministic key for the given scope.
func cacheKey(scope ScopeFilter) string {
	switch scope.Type {
	case ScopeSession:
		return fmt.Sprintf("session:%s", scope.ID)
	case ScopeIssue:
		return fmt.Sprintf("issue:%s", scope.ID)
	case ScopeTimeRange:
		return fmt.Sprintf("timerange:%s", scope.Since.UTC().Format(time.RFC3339))
	case ScopeFailures:
		return fmt.Sprintf("failures:%s", scope.Since.UTC().Format(time.RFC3339))
	default:
		return fmt.Sprintf("unknown:%s", scope.ID)
	}
}

// getCached returns a cached summary if it exists and hasn't expired.
func (a *Analyst) getCached(key string) (*Summary, bool) {
	raw, ok := a.cache.Load(key)
	if !ok {
		return nil, false
	}
	entry, ok := raw.(*cacheEntry)
	if !ok {
		return nil, false
	}
	if time.Since(entry.createdAt) > cacheTTL {
		a.cache.Delete(key)
		return nil, false
	}
	return entry.summary, true
}

// setCache stores a summary in the cache.
func (a *Analyst) setCache(key string, s *Summary) {
	a.cache.Store(key, &cacheEntry{
		summary:   s,
		createdAt: time.Now(),
	})
}
