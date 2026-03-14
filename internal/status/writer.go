// Package status generates status.md content from live project state.
package status

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/herbhall/samverk/internal/forge"
)

// Writer generates status.md content by querying the forge tracker.
type Writer struct {
	tracker   forge.IssueTracker
	healthURL string // optional, e.g. "http://localhost:8080/healthz"
}

// New creates a Writer with the given forge tracker.
func New(tracker forge.IssueTracker, healthURL string) *Writer {
	return &Writer{tracker: tracker, healthURL: healthURL}
}

// StatusData holds the data used to render the status template.
type StatusData struct {
	UpdatedAt    string         // RFC3339
	ServerHealth string         // "healthy", "unreachable", or "unknown"
	InProgress   []*forge.Issue // status:in-progress or status:claimed
	Queued       []*forge.Issue // no recognized status label
	Blocked      []*forge.Issue // status:blocked
	NeedsHuman   []*forge.Issue // status:needs-human or status:human-pending
	NeedsQC      []*forge.Issue // status:needs-qc
	TotalOpen    int
}

// Generate produces the status.md content as a string.
func (w *Writer) Generate(ctx context.Context) (content string, err error) {
	// 1. Fetch all open issues (paginate).
	allOpen := make([]*forge.Issue, 0)
	page := 1
	for {
		batch, listErr := w.tracker.ListIssues(ctx, &forge.ListOptions{
			State:   forge.StateOpen,
			Page:    page,
			PerPage: 100,
		})
		if listErr != nil {
			return "", fmt.Errorf("list open issues: %w", listErr)
		}
		allOpen = append(allOpen, batch...)
		if len(batch) < 100 {
			break
		}
		page++
	}

	// 2. Classify by status label.
	data := StatusData{
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		TotalOpen: len(allOpen),
	}
	for _, issue := range allOpen {
		labels := make(map[string]bool, len(issue.Labels))
		for _, l := range issue.Labels {
			labels[l] = true
		}
		switch {
		case labels["status:in-progress"] || labels["status:claimed"]:
			data.InProgress = append(data.InProgress, issue)
		case labels["status:blocked"]:
			data.Blocked = append(data.Blocked, issue)
		case labels["status:needs-human"] || labels["status:human-pending"]:
			data.NeedsHuman = append(data.NeedsHuman, issue)
		case labels["status:needs-qc"]:
			data.NeedsQC = append(data.NeedsQC, issue)
		default:
			data.Queued = append(data.Queued, issue)
		}
	}

	// 3. Health check (optional, don't fail if unreachable).
	data.ServerHealth = "unknown"
	if w.healthURL != "" {
		data.ServerHealth = checkHealth(w.healthURL)
	}

	// 4. Render template.
	var buf strings.Builder
	if tmplErr := statusTemplate.Execute(&buf, data); tmplErr != nil {
		return "", fmt.Errorf("render template: %w", tmplErr)
	}
	return buf.String(), nil
}

// checkHealth performs an HTTP GET and returns a health status string.
func checkHealth(url string) string {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url) //nolint:gosec // G107: URL is from --health-url flag, not user input
	if err != nil {
		return "unreachable"
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return "healthy"
	}
	return fmt.Sprintf("unhealthy (%d)", resp.StatusCode)
}

// formatLabels returns a comma-separated label string for display.
func formatLabels(labels []string) string {
	return strings.Join(labels, ", ")
}

var statusTemplate = template.Must(template.New("status").Funcs(template.FuncMap{
	"formatLabels": formatLabels,
}).Parse(`---
phase: execution
updated: {{ .UpdatedAt }}
updated_by: samverk-cli
---

# Samverk -- Current State

## Server Health

{{ .ServerHealth }}

## Summary

Total open issues: {{ .TotalOpen }}

## In Progress ({{ len .InProgress }})
{{ if .InProgress }}
{{ range .InProgress }}- #{{ .Number }}: {{ .Title }} ({{ formatLabels .Labels }})
{{ end }}{{ else }}
No issues in progress.
{{ end }}
## Needs QC ({{ len .NeedsQC }})
{{ if .NeedsQC }}
{{ range .NeedsQC }}- #{{ .Number }}: {{ .Title }} ({{ formatLabels .Labels }})
{{ end }}{{ else }}
No issues awaiting QC.
{{ end }}
## Blocked ({{ len .Blocked }})
{{ if .Blocked }}
{{ range .Blocked }}- #{{ .Number }}: {{ .Title }} ({{ formatLabels .Labels }})
{{ end }}{{ else }}
No blocked issues.
{{ end }}
## Needs Human ({{ len .NeedsHuman }})
{{ if .NeedsHuman }}
{{ range .NeedsHuman }}- #{{ .Number }}: {{ .Title }} ({{ formatLabels .Labels }})
{{ end }}{{ else }}
No issues requiring human action.
{{ end }}
## Queued ({{ len .Queued }})
{{ if .Queued }}
{{ range .Queued }}- #{{ .Number }}: {{ .Title }} ({{ formatLabels .Labels }})
{{ end }}{{ else }}
No queued issues.
{{ end }}
## Start Here (Cold Start Protocol)

1. Read this file
2. Call samverk get_digest --since 168h if MCP is configured
3. Read open issues if relevant to the task
4. Proceed -- do not ask the user to explain project state
`))
