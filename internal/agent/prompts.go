package agent

import (
	"fmt"
	"strings"

	"samverk.dev/samverk/pkg/models"
)

// maxFileContextBytes caps the total injected file context to ~8k tokens (~32k bytes).
const maxFileContextBytes = 32_000

// sourceInstructionsCLI is appended to prompts for providers with local
// filesystem access (e.g., claude-cli running in a worktree). These agents
// have Read, Write, Edit, Glob, and Grep tools available.
const sourceInstructionsCLI = `

You have full access to the local source tree via your tools (Read, Glob, Grep, etc.).
The files provided below are your starting context. Explore additional files as needed
using your tools -- do not guess at code structure or import paths.`

// sourceInstructionsAPI is appended to prompts for providers without local
// filesystem access (e.g., Ollama). The files injected into the prompt are
// the only source context available.
const sourceInstructionsAPI = `

The source files provided below are your ONLY context. You do not have filesystem access.
Base all implementations strictly on the code shown. Do not invent import paths, API
signatures, or patterns not visible in the provided files.`

// sourceInstructions returns the appropriate file access instructions based
// on whether the provider has local filesystem tool access.
func sourceInstructions(hasToolAccess bool) string {
	if hasToolAccess {
		return sourceInstructionsCLI
	}
	return sourceInstructionsAPI
}

// BuildSystemPrompt dispatches to per-agent-type prompt builders and appends
// file context and known patterns when available. Agent types that are not
// AI-driven (human, orchestrator, dispatcher) return an empty string.
func BuildSystemPrompt(task Task, fileContext map[string]string, patterns ...string) string {
	srcInstructions := sourceInstructions(task.HasToolAccess)

	var base string
	switch task.AgentType {
	case models.AgentTypeCodeGen:
		base = buildCodeGenPrompt(task, srcInstructions)
	case models.AgentTypeTest:
		base = buildTestPrompt(task, srcInstructions)
	case models.AgentTypeDocs:
		base = buildDocsPrompt(task, srcInstructions)
	case models.AgentTypeResearch:
		base = buildResearchPrompt(task, srcInstructions)
	case models.AgentTypeQC:
		base = buildQCPrompt(task, srcInstructions)
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

func buildCodeGenPrompt(task Task, srcInstructions string) string {
	return fmt.Sprintf(`You are a code generation agent. Your task is to implement the changes described in issue #%d: %s.

Read the relevant files first using the provided context. Implement all acceptance criteria.

%s`+srcInstructions,
		task.Issue.Number, task.Issue.Title, FormatInstructionsForProvider(task.HasToolAccess))
}

func buildTestPrompt(task Task, srcInstructions string) string {
	return fmt.Sprintf(`You are a test agent. Your task is to write or fix tests for issue #%d: %s.

Focus on:
- Table-driven tests for new functions
- Edge cases identified in the issue
- Regression tests for bug fixes

%s`+srcInstructions,
		task.Issue.Number, task.Issue.Title, FormatInstructionsForProvider(task.HasToolAccess))
}

func buildDocsPrompt(task Task, srcInstructions string) string {
	return fmt.Sprintf(`You are a documentation agent. Your task is to update documentation for issue #%d: %s.

Ensure:
- No trailing whitespace
- Proper heading hierarchy
- All links are relative and valid

%s`+srcInstructions,
		task.Issue.Number, task.Issue.Title, FormatInstructionsForProvider(task.HasToolAccess))
}

func buildResearchPrompt(task Task, srcInstructions string) string {
	return fmt.Sprintf(`You are a research agent. Your task is to investigate and summarize findings for issue #%d: %s.

Post your findings as a structured markdown comment with these sections:
## Summary
## Findings
## Recommendation
## Sources

Do not produce file edits. Your output is a comment on the issue.`+srcInstructions,
		task.Issue.Number, task.Issue.Title)
}

func buildQCPrompt(task Task, srcInstructions string) string {
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

	// Include doc review findings when present (pre-QC enrichment).
	if task.DocContext != "" {
		b.WriteString(task.DocContext)
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

	b.WriteString(srcInstructions)

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
// respecting the maxFileContextBytes cap. When files are truncated or
// omitted due to the budget, a warning is appended listing what was skipped.
func buildFileContext(files map[string]string) string {
	if len(files) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Relevant Files\n")
	remaining := maxFileContextBytes
	var omitted []string
	truncated := false

	for path, content := range files {
		if content == "" {
			omitted = append(omitted, path+" (content unavailable)")
			continue
		}

		header := fmt.Sprintf("\n### %s\n```\n", path)
		footer := "\n```\n"
		needed := len(header) + len(content) + len(footer)

		if needed > remaining {
			// Truncate content to fit within budget.
			available := remaining - len(header) - len(footer) - len("\n... (truncated)\n")
			if available <= 0 {
				omitted = append(omitted, path+" (budget exceeded)")
				continue
			}
			b.WriteString(header)
			b.WriteString(content[:available])
			b.WriteString("\n... (truncated)\n")
			b.WriteString("```\n")
			truncated = true
			remaining = 0
			continue
		}

		b.WriteString(header)
		b.WriteString(content)
		b.WriteString(footer)
		remaining -= needed
	}

	if truncated || len(omitted) > 0 {
		b.WriteString("\n### File Context Warning\n\n")
		if truncated {
			b.WriteString("Some file contents were truncated to fit within the context budget.\n")
		}
		if len(omitted) > 0 {
			b.WriteString("The following files could not be included:\n")
			for _, o := range omitted {
				fmt.Fprintf(&b, "- %s\n", o)
			}
		}
	}

	return b.String()
}
