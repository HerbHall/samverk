package dispatcher

import (
	"sort"
	"strings"
	"time"

	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/pkg/models"
)

// priorityWeight returns a numeric weight for the issue's priority label.
// Higher values dispatch first.
func priorityWeight(labels []string) int {
	for _, l := range labels {
		switch l {
		case models.LabelPriorityCritical:
			return 100
		case models.LabelPriorityHigh:
			return 75
		case models.LabelPriorityNormal:
			return 50
		case models.LabelPriorityLow:
			return 25
		}
	}
	return 50 // default to normal
}

// ageBonus returns a bonus score based on issue age, capped at 50.
// Each day adds 5 points, so a 10-day-old issue gets the maximum bonus.
func ageBonus(createdAt, now time.Time) int {
	days := int(now.Sub(createdAt).Hours() / 24)
	bonus := days * 5
	if bonus > 50 {
		bonus = 50
	}
	return bonus
}

// issueScore computes the composite dispatch priority for an issue.
func issueScore(issue *forge.Issue, now time.Time) int {
	return priorityWeight(issue.Labels) + ageBonus(issue.CreatedAt, now)
}

// sortByPriority sorts issues by composite score (highest first).
// Uses stable sort to preserve creation order within equal scores.
func sortByPriority(issues []*forge.Issue, now time.Time) {
	sort.SliceStable(issues, func(i, j int) bool {
		return issueScore(issues[i], now) > issueScore(issues[j], now)
	})
}

// chainPromotionAge is the minimum issue age before chain promotion is considered.
const chainPromotionAge = 3 * 24 * time.Hour

// promotableChain maps source chains to their promotion targets.
// Only free -> plan-based promotions are allowed.
// Never promote TO complex (Opus reserved for critical/architectural work).
// Never promote to API-billed providers.
var promotableChain = map[string]string{
	"local":  "code-gen", // Ollama local -> CLI sonnet/haiku
	"triage": "default",  // Ollama triage -> CLI sonnet
}

// shouldPromoteChain decides whether an issue should be promoted to a
// higher-capacity chain. Returns the promoted chain key and true, or
// the original chain and false.
//
// Promotion criteria:
//   - Issue is older than chainPromotionAge (3 days)
//   - Current chain has a promotion target
//   - Pool has idle workers (capacity available)
func shouldPromoteChain(chain string, issue *forge.Issue, idleWorkers int) (string, bool) {
	if idleWorkers <= 0 {
		return chain, false
	}

	age := time.Since(issue.CreatedAt)
	if age < chainPromotionAge {
		return chain, false
	}

	target, ok := promotableChain[chain]
	if !ok {
		return chain, false
	}

	return target, true
}

// isOllamaChain returns true if the chain name typically routes to Ollama
// (free local GPU providers). Used as a heuristic -- actual chain contents
// are in the provider registry config.
func isOllamaChain(chain string) bool {
	return strings.HasPrefix(chain, "local") || strings.HasPrefix(chain, "triage")
}
