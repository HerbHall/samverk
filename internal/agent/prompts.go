package agent

import (
	"fmt"
	"strings"

	"github.com/herbhall/samverk/pkg/models"
)

// maxFileContextBytes caps the total injected file context to ~8k tokens (~32k bytes).
const maxFileContextBytes = 32_000

// githubSourceInstructions is appended to AI-driven agent prompts that may need
// to read source files. Agents run on CT 202 where the Go source tree is absent;
// the GitHub Contents API is the canonical way to fetch source.
const githubSourceInstructions = `

You do not have access to the local source tree. To read source files, use the GitHub Contents API:

  curl -s -H "Authorization: Bearer $GITHUB_TOKEN" \
    https://api.github.com/repos/herbhall/samverk/contents/<path> \
    | python3 -c "import sys,json,base64; d=json.load(sys.stdin); print(base64.b64decode(d['content'].replace('\n','')).decode())"

Always read the actual source before drawing conclusions.`

// BuildSystemPrompt dispatches to per-agent-type prompt builders and appends
// file context and known patterns when available. Agent types that are not
// AI-driven (human, orchestrator, dispatcher) return an empty string.
func BuildSystemPrompt(task Task, fileContext map[string]string, patterns ...string) string {
	var base string
	switch task.AgentType {
	case models.AgentTypeCodeGen:
		base = buildCodeGenPrompt(task)
	case models.AgentTypeTest:
		base = buildTestPrompt(task)
	case models.AgentTypeDocs:
		base = buildDocsPrompt(task)
	case models.AgentTypeResearch:
		base = buildResearchPrompt(task)
	case models.AgentTypeQC:
		base = buildQCPrompt(task)
	case models.AgentTypeHuman, models.AgentTypeOrchestrator, models.AgentTypeDispatcher:
		return ""
	default:
		return ""
	}

	// Communicate time budget so the agent can prioritize.
	if task.Timeout > 0 {
		minutes := int(task.Timeout.Minutes())
		base += fmt.Sprintf("\n\n## Time Budget\n\nYou have approximately %d minutes for this task. Prioritize the most critical acceptance criteria first. If time is limited, deliver a working partial solution over an incomplete full solution.", minutes)
	}

	// Communicate constraints from frontmatter if available.
	if task.Frontmatter != nil && len(task.Frontmatter.Constraints) > 0 {
		base += "\n\n## Constraints (from issue author)\n\n"
		for _, c := range task.Frontmatter.Constraints {
			base += "- " + c + "\n"
		}
	}

	ctx := buildFileContext(fileContext)
	if ctx != "" {
		base += "\n\n" + ctx
	}

	if patternSection := buildPatternContext(patterns); patternSection != "" {
		base += "\n\n" + patternSection
	}

	return base
}

// buildPatternContext renders fetched Synapset patterns into a prompt section.
func buildPatternContext(patterns []string) string {
	if len(patterns) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Known Patterns\n\n")
	for _, p := range patterns {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	return b.String()
}

func buildCodeGenPrompt(task Task) string {
	return fmt.Sprintf(`You are a code generation agent. Your task is to implement the changes described in issue #%d: %s.

Read the relevant files first using the provided context. Implement all acceptance criteria.

%s`+githubSourceInstructions,
		task.Issue.Number, task.Issue.Title, FormatInstructions())
}

func buildTestPrompt(task Task) string {
	return fmt.Sprintf(`You are a test agent. Your task is to write or fix tests for issue #%d: %s.

Focus on:
- Table-driven tests for new functions
- Edge cases identified in the issue
- Regression tests for bug fixes

%s`+githubSourceInstructions,
		task.Issue.Number, task.Issue.Title, FormatInstructions())
}

func buildDocsPrompt(task Task) string {
	return fmt.Sprintf(`You are a documentation agent. Your task is to update documentation for issue #%d: %s.

Ensure:
- No trailing whitespace
- Proper heading hierarchy
- All links are relative and valid

%s`+githubSourceInstructions,
		task.Issue.Number, task.Issue.Title, FormatInstructions())
}

func buildResearchPrompt(task Task) string {
	return fmt.Sprintf(`You are a research agent. Your task is to investigate and summarize findings for issue #%d: %s.

Post your findings as a structured markdown comment with these sections:
## Summary
## Findings
## Recommendation
## Sources

Do not produce file edits. Your output is a comment on the issue.`+githubSourceInstructions,
		task.Issue.Number, task.Issue.Title)
}

func buildQCPrompt(task Task) string {
	var b strings.Builder

	fmt.Fprintf(&b, `You are a quality control agent reviewing the work on issue #%d: %s.

You must evaluate the output independently — do not read the producing agent's reasoning or comments. Evaluate only the code/content itself against:
- Correctness: does it do what the issue asked?
- Quality: does it follow project conventions?
- Tests: are there adequate tests?
- Risk: any regressions or side effects?

Respond with PASS or FAIL (or REVIEW if ambiguous), followed by specific findings.
No praise, no encouragement — only concrete evaluation.`, task.Issue.Number, task.Issue.Title)

	// Include acceptance criteria parsed from issue body.
	if task.Issue != nil {
		criteria := ParseAcceptanceCriteria(task.Issue.Body)
		if len(criteria) > 0 {
			b.WriteString("\n\n## Acceptance Criteria to Verify\n\n")
			for _, c := range criteria {
				fmt.Fprintf(&b, "- [ ] %s\n", c)
			}
		}
	}

	// Include constraints from frontmatter.
	if task.Frontmatter != nil && len(task.Frontmatter.Constraints) > 0 {
		b.WriteString("\n\n## Constraints to Verify\n\n")
		for _, c := range task.Frontmatter.Constraints {
			fmt.Fprintf(&b, "- [ ] %s\n", c)
		}
	}

	// Include file_context scope for scope checking.
	if task.Frontmatter != nil && len(task.Frontmatter.FileContext) > 0 {
		b.WriteString("\n\n## Expected File Scope\n\nChanges should be within these files:\n")
		for _, f := range task.Frontmatter.FileContext {
			fmt.Fprintf(&b, "- `%s`\n", f)
		}
		b.WriteString("\nFlag any changes to files outside this scope.\n")
	}

	issueNum := 0
	if task.Issue != nil {
		issueNum = task.Issue.Number
	}
	fmt.Fprintf(&b, `

## Required Output Format

Your response MUST follow this exact structure:

`+"```"+`
## QC Review: PR (Issue #%d)

### Verdict: [PASS] | [FAIL] | [REVIEW]

### Acceptance Criteria
- [x] Criterion -- verified: <evidence>
- [ ] Criterion -- MISSING: <what's missing>

### Constraints
- [x] Constraint -- respected
- [ ] Constraint -- VIOLATED: <details>

### Scope
- Changed files: N (within/outside file_context)
- Unexpected files: N

### Issues Found
- <list specific problems, or "None">

### Recommendation
[VERDICT] -- <brief explanation>
`+"```"+`

Rules:
- Use [PASS] only when ALL acceptance criteria are met and no issues found.
- Use [FAIL] when any acceptance criterion is unmet or a significant issue exists.
- Use [REVIEW] when you cannot determine correctness (ambiguous requirements, needs human judgment).
`, issueNum)

	b.WriteString(githubSourceInstructions)

	return b.String()
}

// ParseAcceptanceCriteria extracts acceptance criteria from an issue body.
// It looks for markdown checkboxes (- [ ] ...) under an "## Acceptance Criteria" heading.
// Falls back to finding any checkboxes in the body if no heading is found.
func ParseAcceptanceCriteria(body string) []string {
	criteria := make([]string, 0, 8)
	lines := strings.Split(body, "\n")
	inSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect the acceptance criteria heading (case-insensitive prefix match).
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "## acceptance criteria") {
			inSection = true
			continue
		}

		// If we're in the section and hit another heading, stop.
		if inSection && strings.HasPrefix(trimmed, "## ") {
			break
		}

		// Collect checkboxes from within the section.
		if inSection && len(trimmed) > 6 &&
			(strings.HasPrefix(trimmed, "- [ ] ") ||
				strings.HasPrefix(trimmed, "- [x] ") ||
				strings.HasPrefix(trimmed, "- [X] ")) {
			text := trimmed[6:] // len("- [ ] ") == 6
			if text != "" {
				criteria = append(criteria, text)
			}
		}
	}

	// Fallback: if no section heading found, scan the entire body for checkboxes.
	if len(criteria) == 0 {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if len(trimmed) > 6 &&
				(strings.HasPrefix(trimmed, "- [ ] ") ||
					strings.HasPrefix(trimmed, "- [x] ") ||
					strings.HasPrefix(trimmed, "- [X] ")) {
				text := trimmed[6:]
				if text != "" {
					criteria = append(criteria, text)
				}
			}
		}
	}

	return criteria
}

// buildFileContext renders the file context map into a markdown section,
// respecting the maxFileContextBytes cap.
func buildFileContext(files map[string]string) string {
	if len(files) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Relevant Files\n")
	remaining := maxFileContextBytes

	for path, content := range files {
		header := fmt.Sprintf("\n### %s\n```\n", path)
		footer := "\n```\n"
		needed := len(header) + len(content) + len(footer)

		if needed > remaining {
			// Truncate content to fit within budget.
			available := remaining - len(header) - len(footer) - len("\n... (truncated)\n")
			if available <= 0 {
				break
			}
			b.WriteString(header)
			b.WriteString(content[:available])
			b.WriteString("\n... (truncated)\n")
			b.WriteString("```\n")
			break
		}

		b.WriteString(header)
		b.WriteString(content)
		b.WriteString(footer)
		remaining -= needed
	}

	return b.String()
}
