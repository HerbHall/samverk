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
	fmt.Fprintf(&b, "Analyzed %d log entries", stats.total)

	switch scope.Type {
	case ScopeSession:
		fmt.Fprintf(&b, " for session %s.", scope.ID)
	case ScopeIssue:
		fmt.Fprintf(&b, " for issue #%s.", scope.ID)
	case ScopeTimeRange:
		fmt.Fprintf(&b, " from %s to %s.",
			scope.Since.Format("2006-01-02 15:04"),
			scope.Until.Format("2006-01-02 15:04"),
		)
	case ScopeFailures:
		fmt.Fprintf(&b, " (failures since %s).", scope.Since.Format("2006-01-02 15:04"))
	}

	fmt.Fprintf(&b, " Found %d errors and %d warnings", stats.errors, stats.warnings)

	if len(stats.components) > 0 {
		fmt.Fprintf(&b, " across components: %s.", strings.Join(stats.components, ", "))
	} else {
		b.WriteString(".")
	}

	// Include the most recent error message if present.
	for i := range entries {
		if entries[i].Level == "error" {
			fmt.Fprintf(&b, " Most recent error: %q.", entries[i].Message)
			break
		}
	}

	return b.String()
}
