package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/internal/hostmetrics"
	"samverk.dev/samverk/pkg/models"
)

type capacityApplyRequest struct {
	Resource string `json:"resource"`
}

type capacityApplyResponse struct {
	IssueNumber int    `json:"issue_number"`
	IssueURL    string `json:"issue_url"`
}

// handleCapacityApply creates a needs-human draft issue for a Proxmox reconfiguration action.
func (a *API) handleCapacityApply(w http.ResponseWriter, r *http.Request) {
	if a.hostMetrics == nil {
		writeError(w, http.StatusServiceUnavailable, "host metrics not configured")
		return
	}
	if a.tracker == nil {
		writeError(w, http.StatusServiceUnavailable, "issue tracker not configured")
		return
	}

	var req capacityApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Resource == "" {
		writeError(w, http.StatusBadRequest, "resource is required")
		return
	}

	// Run the recommendation engine against the last 24h of history.
	analysisWindow := a.hostMetrics.History(time.Now().Add(-24 * time.Hour))
	recs := hostmetrics.AnalyzeHistory(analysisWindow, a.infra)

	// Find the recommendation for the requested resource.
	var rec *hostmetrics.Recommendation
	for i := range recs {
		if recs[i].Resource == req.Resource {
			rec = &recs[i]
			break
		}
	}
	if rec == nil {
		writeError(w, http.StatusNotFound, "no recommendation found for resource: "+req.Resource)
		return
	}
	if rec.ActionType != hostmetrics.ActionProxmoxConfig || rec.Action == nil {
		writeError(w, http.StatusBadRequest, "recommendation for "+req.Resource+" has no applicable Proxmox action")
		return
	}

	body := buildCapacityIssueBody(rec)
	title := fmt.Sprintf("infra: %s", rec.Title)

	issue, err := a.tracker.CreateIssue(r.Context(), &forge.CreateIssueRequest{
		Title:  title,
		Body:   body,
		Labels: []string{"agent:infra", models.LabelStatusNeedsHuman},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create issue: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, capacityApplyResponse{
		IssueNumber: issue.Number,
		IssueURL:    issueURL(issue),
	})
}

// issueURL returns a best-effort URL for the issue. The forge interface does not
// expose a URL field, so we cannot construct a real link here. The issue number
// is the authoritative identifier; callers should resolve the URL via the forge.
func issueURL(issue *forge.Issue) string {
	return fmt.Sprintf("#%d", issue.Number)
}

func buildCapacityIssueBody(rec *hostmetrics.Recommendation) string {
	act := rec.Action
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("schema-version: \"1.0\"\n")
	sb.WriteString("type: task\n")
	sb.WriteString("agent-type: infra\n")
	sb.WriteString("priority: high\n")
	sb.WriteString("status: needs-human\n")
	sb.WriteString("---\n\n")

	sb.WriteString("## Capacity Recommendation\n\n")
	fmt.Fprintf(&sb, "**Resource:** %s  \n", rec.Resource)
	fmt.Fprintf(&sb, "**Severity:** %s  \n", string(rec.Level))
	fmt.Fprintf(&sb, "**Finding:** %s\n\n", rec.Detail)

	sb.WriteString("## Evidence (24h window)\n\n")
	fmt.Fprintf(&sb, "- Average utilisation: %.1f%%\n", rec.Stats.Avg)
	fmt.Fprintf(&sb, "- 95th percentile: %.1f%%\n", rec.Stats.P95)
	fmt.Fprintf(&sb, "- Peak: %.1f%%\n", rec.Stats.Peak)
	fmt.Fprintf(&sb, "- Time above warn threshold: %.0f%%\n", rec.Stats.TimeAboveWarn*100)
	fmt.Fprintf(&sb, "- Time above critical threshold: %.0f%%\n", rec.Stats.TimeAboveCrit*100)
	if rec.Stats.DaysToCrit > 0 {
		fmt.Fprintf(&sb, "- Projected days to critical: %.0f days\n", rec.Stats.DaysToCrit)
	}
	fmt.Fprintf(&sb, "- Sample count: %d snapshots\n\n", rec.Stats.SampleCount)

	sb.WriteString("## Recommended Action\n\n")
	fmt.Fprintf(&sb, "**Description:** %s  \n", act.Description)
	fmt.Fprintf(&sb, "**Cost:** %s  \n", act.CostNote)
	fmt.Fprintf(&sb, "**SSH Target:** `%s`  \n", act.SSHTarget)
	fmt.Fprintf(&sb, "**Current:** %s → **Recommended:** %s\n\n", act.CurrentValue, act.RecommendedValue)

	sb.WriteString("## Command to Execute\n\n")
	fmt.Fprintf(&sb, "```bash\nssh %s '%s'\n```\n\n", act.SSHTarget, act.Command)

	sb.WriteString("## Verification Steps\n\n")
	sb.WriteString("1. Confirm the container is idle or can tolerate a brief restart\n")
	sb.WriteString("2. Execute the command above via SSH\n")
	fmt.Fprintf(&sb, "3. Verify: `ssh %s 'pct config %s'`\n", act.SSHTarget, getContainerID(act.Command))
	sb.WriteString("4. Monitor resource utilisation over the next 24h\n\n")
	sb.WriteString("> **Review gate:** Relabel this issue to `status:queued` once satisfied with the command to dispatch the infra agent.\n")

	return sb.String()
}

// getContainerID extracts the container ID from a pct command string.
func getContainerID(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) >= 3 {
		return parts[2]
	}
	return "202"
}
