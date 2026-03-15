package loganalyst

import (
	"fmt"
	"strings"

	"github.com/herbhall/samverk/internal/logstore"
)

// maxPromptEntries is the maximum number of log entries included in the
// prompt to avoid excessive token usage.
const maxPromptEntries = 50

// buildPrompt constructs a structured prompt for the Ollama model.
func buildPrompt(entries []logstore.LogEntry, scope ScopeFilter) string {
	stats := computeStats(entries)

	var b strings.Builder
	b.WriteString("You are analyzing logs from Samverk, an automated development agent system.\n")
	b.WriteString("Summarize the following log data in 2-3 sentences, focusing on: what happened, whether it succeeded, and any failures or patterns.\n\n")

	b.WriteString(fmt.Sprintf("Scope: %s %s\n", scope.Type, scope.ID))
	b.WriteString(fmt.Sprintf("Total entries: %d\n", stats.total))
	b.WriteString(fmt.Sprintf("Errors: %d\n", stats.errors))
	b.WriteString(fmt.Sprintf("Warnings: %d\n", stats.warnings))
	if len(stats.components) > 0 {
		b.WriteString(fmt.Sprintf("Components: %s\n", strings.Join(stats.components, ", ")))
	}

	b.WriteString("\nSample log entries (most recent first):\n")

	limit := len(entries)
	if limit > maxPromptEntries {
		limit = maxPromptEntries
	}
	for i := 0; i < limit; i++ {
		e := &entries[i]
		ts := e.Timestamp.Format("15:04:05")
		b.WriteString(fmt.Sprintf("[%s] [%s] [%s] %s\n", ts, e.Level, e.Component, e.Message))
	}

	return b.String()
}
