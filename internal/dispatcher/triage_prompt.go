package dispatcher

import (
	"fmt"
	"strings"
)

// TriagePromptInput contains the data passed to the triage evaluation prompt.
type TriagePromptInput struct {
	IssueNumber      int
	IssueTitle       string
	IssueBody        string
	Comments         []string
	FailureHistory   []string
	Labels           []string
	HardBlockRules   []string
	AutoApproveTypes []string
}

// BuildTriagePrompt constructs the evaluation prompt for the triage agent.
// This lives in a separate file so the prompt can be refined without touching
// the orchestration code.
func BuildTriagePrompt(input TriagePromptInput) string {
	var sb strings.Builder

	sb.WriteString(`You are an autonomous triage agent evaluating whether a "needs-human" issue can be safely automated.

## Issue Details

`)
	fmt.Fprintf(&sb, "**Issue #%d**: %s\n\n", input.IssueNumber, input.IssueTitle)
	sb.WriteString("### Body\n\n")
	sb.WriteString(input.IssueBody)
	sb.WriteString("\n\n")

	if len(input.Labels) > 0 {
		sb.WriteString("### Labels\n\n")
		for _, l := range input.Labels {
			fmt.Fprintf(&sb, "- %s\n", l)
		}
		sb.WriteString("\n")
	}

	if len(input.Comments) > 0 {
		sb.WriteString("### Comments\n\n")
		for i, c := range input.Comments {
			fmt.Fprintf(&sb, "**Comment %d:**\n%s\n\n", i+1, c)
		}
	}

	if len(input.FailureHistory) > 0 {
		sb.WriteString("### Failure History\n\n")
		for _, f := range input.FailureHistory {
			fmt.Fprintf(&sb, "- %s\n", f)
		}
		sb.WriteString("\n")
	}

	sb.WriteString(`## Evaluation Criteria

Evaluate the issue against these autonomy criteria:

| Criterion | Auto-approve if... |
|-----------|-------------------|
| Recoverable | Changes are on a feature branch, revertible via PR close |
| Non-destructive | No database migrations, no infra changes, no secret handling |
| Spec-aligned | Work matches an existing ADR, design doc, or approved plan |
| Decomposable | A complex issue can be split into smaller atomic pieces |
| Pre-approved scope | Issue type is one of: `)

	sb.WriteString(strings.Join(input.AutoApproveTypes, ", "))
	sb.WriteString(` within existing architecture |

## Hard Block Rules (NEVER auto-approve)

These categories MUST stay needs-human regardless of other criteria:

`)
	for _, rule := range input.HardBlockRules {
		fmt.Fprintf(&sb, "- %s\n", rule)
	}

	sb.WriteString(`
- Issues explicitly marked human:review by a human

## Required Output

Respond with ONLY valid JSON (no markdown fencing, no extra text):

{
  "verdict": "auto_unblock" | "decompose" | "confirmed_human",
  "reasoning": "detailed explanation of the decision",
  "criteria_results": [
    {"criterion": "recoverable", "passed": true/false, "detail": "..."},
    {"criterion": "non_destructive", "passed": true/false, "detail": "..."},
    {"criterion": "spec_aligned", "passed": true/false, "detail": "..."},
    {"criterion": "decomposable", "passed": true/false, "detail": "..."},
    {"criterion": "pre_approved_scope", "passed": true/false, "detail": "..."}
  ],
  "docs_consulted": ["list of docs/ADRs referenced"],
  "sub_issues": [
    {"title": "...", "body": "...", "agent_type": "code-gen"}
  ]
}

Rules:
- If ANY hard block rule matches, verdict MUST be "confirmed_human"
- For "auto_unblock": all criteria must pass, no hard blocks
- For "decompose": issue is too complex as-is but parts are automatable
- sub_issues array should only be populated for "decompose" verdict
- Be conservative: when in doubt, use "confirmed_human"
`)

	return sb.String()
}
