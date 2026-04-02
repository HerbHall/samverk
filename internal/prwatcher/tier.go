package prwatcher

import (
	"strings"

	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/pkg/models"
)

// PRTier represents the merge policy tier for a pull request.
type PRTier int

const (
	// PRTierUnknown is the default when classification cannot determine a tier.
	PRTierUnknown PRTier = iota
	// PRTier1 is for docs, config, chores — auto-merge immediately on CI green.
	PRTier1
	// PRTier2 is for features, fixes, refactoring — auto-merge after delay.
	PRTier2
	// PRTier3 is for architecture, breaking, security — never auto-merge.
	PRTier3
)

// String returns the human-readable tier name.
func (t PRTier) String() string {
	switch t {
	case PRTier1:
		return "tier-1"
	case PRTier2:
		return "tier-2"
	case PRTier3:
		return "tier-3"
	default:
		return "unknown"
	}
}

// Description returns a short explanation of the tier's merge policy.
func (t PRTier) Description() string {
	switch t {
	case PRTier1:
		return "auto-merge on CI green"
	case PRTier2:
		return "auto-merge after delay"
	case PRTier3:
		return "requires human review"
	default:
		return "unclassified"
	}
}

// tier3Labels are issue labels that force Tier 3 classification.
var tier3Labels = []string{
	models.LabelPriorityCritical,
	"complexity:high",
	"type:architecture",
	"type:breaking",
}

// tier3TitleKeywords in the PR title force Tier 3.
var tier3TitleKeywords = []string{"breaking", "security", "deploy"}

// tier1Labels are issue labels that indicate Tier 1.
var tier1Labels = []string{
	models.LabelAgentDocs,
	"type:chore",
	"autorelease: pending",
}

// tier1Prefixes are PR title prefixes that indicate Tier 1.
var tier1Prefixes = []string{"docs:", "docs(", "chore:", "chore(", "ci:", "ci(", "style:", "style("}

// tier2Prefixes are PR title prefixes that indicate Tier 2.
var tier2Prefixes = []string{"feat:", "feat(", "fix:", "fix(", "refactor:", "refactor(", "test:", "test("}

// ClassifyPRTier determines the merge policy tier for a pull request.
// It examines the PR's labels, title, and linked issue labels.
// The highest matching tier wins (Tier 3 > Tier 2 > Tier 1).
// issueLabels are the labels from the linked issue (if available).
func ClassifyPRTier(pr *forge.PullRequest, issueLabels []string) PRTier {
	allLabels := make(map[string]bool, len(pr.Labels)+len(issueLabels))
	for _, l := range pr.Labels {
		allLabels[l] = true
	}
	for _, l := range issueLabels {
		allLabels[l] = true
	}

	lower := strings.ToLower(pr.Title)

	// Check Tier 3 first (highest priority).
	for _, l := range tier3Labels {
		if allLabels[l] {
			return PRTier3
		}
	}
	for _, kw := range tier3TitleKeywords {
		if strings.Contains(lower, kw) {
			return PRTier3
		}
	}

	// Check Tier 1 (lowest priority — most permissive).
	for _, l := range tier1Labels {
		if allLabels[l] {
			return PRTier1
		}
	}
	for _, pfx := range tier1Prefixes {
		if strings.HasPrefix(lower, pfx) {
			return PRTier1
		}
	}

	// Check Tier 2.
	for _, pfx := range tier2Prefixes {
		if strings.HasPrefix(lower, pfx) {
			return PRTier2
		}
	}
	if allLabels[models.LabelAgentCodeGen] || allLabels[models.LabelAgentTest] {
		return PRTier2
	}

	// Default to Tier 2 for unclassified PRs.
	return PRTier2
}
