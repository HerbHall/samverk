package loganalyst

import (
	"fmt"
	"strings"

	"github.com/herbhall/samverk/internal/logstore"
)

// buildFallbackSummary produces a structured text summary without AI.
func buildFallbackSummary(entries []logstore.LogEntry, scope ScopeFilter) string {
	stats := computeStats(entries)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Analyzed %d log entries", stats.total))

	switch scope.Type {
	case ScopeSession:
		b.WriteString(fmt.Sprintf(" for session %s.", scope.ID))
	case ScopeIssue:
		b.WriteString(fmt.Sprintf(" for issue #%s.", scope.ID))
	case ScopeTimeRange:
		b.WriteString(fmt.Sprintf(" from %s to %s.",
			scope.Since.Format("2006-01-02 15:04"),
			scope.Until.Format("2006-01-02 15:04"),
		))
	case ScopeFailures:
		b.WriteString(fmt.Sprintf(" (failures since %s).", scope.Since.Format("2006-01-02 15:04")))
	}

	b.WriteString(fmt.Sprintf(" Found %d errors and %d warnings", stats.errors, stats.warnings))

	if len(stats.components) > 0 {
		b.WriteString(fmt.Sprintf(" across components: %s.", strings.Join(stats.components, ", ")))
	} else {
		b.WriteString(".")
	}

	// Include the most recent error message if present.
	for i := range entries {
		if entries[i].Level == "error" {
			b.WriteString(fmt.Sprintf(" Most recent error: %q.", entries[i].Message))
			break
		}
	}

	return b.String()
}
