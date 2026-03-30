// Package dispatcher provides documentation review enrichment for the QC
// pipeline. When an agent completes work on an issue that touched markdown
// files, this module checks for documentation issues (staleness, structural
// gaps, cross-document contradictions) and produces findings that are injected
// into the QC agent's context.
package dispatcher

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/herbhall/samverk/pkg/models"
	"go.uber.org/zap"
)

// DocFinding represents a single documentation issue found during enrichment.
type DocFinding struct {
	Severity string // "critical", "high", "medium", "low"
	Category string // "stale", "inconsistent", "structure", "reference"
	File     string
	Detail   string
	Tier     int // 1=auto-fix, 2=agent-decides, 3=human-escalate
}

// DocFindings is the result of pre-QC doc enrichment.
type DocFindings struct {
	Files    []string
	Findings []DocFinding
}

// maxDocSection caps the injected documentation section to avoid bloating
// the QC prompt. Measured in bytes of rendered markdown.
const maxDocSection = 1500

// contradictionThreshold is the minimum similarity score for flagging
// potential cross-document contradictions via Synapset.
const contradictionThreshold = 0.85

// staleThresholdDays is the number of days after which a document is
// considered stale if it has not been re-indexed.
const staleThresholdDays = 30

// checkDocFindings runs doc-specific checks on .md files from the issue.
// Returns nil if no .md files were touched or if Synapset is unavailable.
func (d *Dispatcher) checkDocFindings(ctx context.Context, owner, repo string,
	fm *models.IssueFrontmatter) *DocFindings {

	mdFiles := mdFilesFromFrontmatter(fm)
	if len(mdFiles) == 0 {
		return nil
	}

	if d.synapset == nil {
		d.logger.Debug("doc enrichment skipped: no synapset client")
		return nil
	}

	findings := &DocFindings{Files: mdFiles}

	for _, f := range mdFiles {
		d.checkStaleness(ctx, f, findings)
		d.checkContradictions(ctx, f, findings)
	}

	if len(findings.Findings) == 0 {
		return nil
	}
	return findings
}

// mdFilesFromFrontmatter extracts .md file paths from the issue frontmatter's
// FileContext field.
func mdFilesFromFrontmatter(fm *models.IssueFrontmatter) []string {
	if fm == nil {
		return nil
	}
	var mdFiles []string
	for _, f := range fm.FileContext {
		if strings.EqualFold(filepath.Ext(f), ".md") {
			mdFiles = append(mdFiles, f)
		}
	}
	return mdFiles
}

// checkStaleness queries Synapset for the file's last-indexed timestamp and
// flags it as stale if older than staleThresholdDays.
func (d *Dispatcher) checkStaleness(ctx context.Context, file string, findings *DocFindings) {
	basename := filepath.Base(file)
	memories, err := d.synapset.QueryMemory(ctx, "docs", map[string]interface{}{
		"tags":   basename,
		"limit":  1,
		"format": "json",
	})
	if err != nil {
		d.logger.Warn("doc staleness check failed", zap.String("file", file), zap.Error(err))
		return
	}
	if len(memories) == 0 {
		findings.Findings = append(findings.Findings, DocFinding{
			Severity: "medium",
			Category: "stale",
			File:     file,
			Detail:   "file not found in Synapset docs pool (never indexed)",
			Tier:     2,
		})
		return
	}
	// Synapset Memory doesn't expose created_at in the standard response,
	// but we can check for staleness by querying with a date filter.
	cutoff := time.Now().AddDate(0, 0, -staleThresholdDays).Format("2006-01-02")
	recentMemories, err := d.synapset.QueryMemory(ctx, "docs", map[string]interface{}{
		"tags":          basename,
		"created_after": cutoff,
		"limit":         1,
		"format":        "json",
	})
	if err != nil {
		d.logger.Warn("doc staleness date check failed", zap.String("file", file), zap.Error(err))
		return
	}
	if len(recentMemories) == 0 {
		findings.Findings = append(findings.Findings, DocFinding{
			Severity: "medium",
			Category: "stale",
			File:     file,
			Detail:   fmt.Sprintf("last indexed more than %d days ago", staleThresholdDays),
			Tier:     2,
		})
	}
}

// checkContradictions queries Synapset for semantically similar content that
// may contradict the file's sections. Only high-similarity matches (>0.85) are
// flagged, and self-matches (same file) are filtered out.
func (d *Dispatcher) checkContradictions(ctx context.Context, file string, findings *DocFindings) {
	basename := filepath.Base(file)

	// Search for the file's content in the docs pool.
	memories, err := d.synapset.SearchMemory(ctx, "docs", basename, 3)
	if err != nil || len(memories) == 0 {
		return
	}

	// Use the first section's content as the query for contradiction detection.
	query := memories[0].Content
	if len(query) > 500 {
		query = query[:500]
	}

	candidates, err := d.synapset.FindContradictions(ctx, "docs", query, contradictionThreshold, 5)
	if err != nil {
		d.logger.Warn("doc contradiction check failed", zap.String("file", file), zap.Error(err))
		return
	}

	for _, c := range candidates {
		// Skip self-matches.
		for _, tag := range c.Tags {
			if tag == basename {
				continue
			}
		}
		severity := contradictionSeverity(c.Similarity)
		findings.Findings = append(findings.Findings, DocFinding{
			Severity: severity,
			Category: "inconsistent",
			File:     file,
			Detail:   fmt.Sprintf("potential conflict with %s (similarity: %.2f): %s", c.Source, c.Similarity, truncate(c.Content, 100)),
			Tier:     3,
		})
	}
}

// contradictionSeverity maps similarity scores to severity levels per DES-001.
func contradictionSeverity(similarity float64) string {
	switch {
	case similarity > 0.95:
		return "critical"
	case similarity > 0.90:
		return "high"
	default:
		return "medium"
	}
}

// RenderDocSection formats DocFindings as a markdown section for QC prompt
// injection. Returns empty string if there are no findings.
func (df *DocFindings) RenderDocSection() string {
	if df == nil || len(df.Findings) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n## Documentation Review Findings\n\n")
	b.WriteString("The following documentation issues were detected in files touched by this issue.\n")
	b.WriteString("Factor these into your QC verdict.\n\n")

	for _, f := range df.Findings {
		fmt.Fprintf(&b, "- **[%s]** `%s` (%s): %s\n", strings.ToUpper(f.Severity), f.File, f.Category, f.Detail)
	}

	section := b.String()
	if len(section) > maxDocSection {
		section = section[:maxDocSection] + "\n... (truncated)\n"
	}
	return section
}

// docLabelFor returns the label constant for a doc finding category.
func docLabelFor(category string) string {
	switch category {
	case "stale":
		return models.LabelDocStale
	case "inconsistent":
		return models.LabelDocInconsistent
	case "structure":
		return models.LabelDocStructure
	case "decision-review":
		return models.LabelDocDecisionReview
	default:
		return ""
	}
}

// truncate is defined in triage.go -- reused here for doc review.
