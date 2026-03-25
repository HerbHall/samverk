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

Read the relevant files first using the provided context. Produce your response as a structured edit specification:

EDIT <filepath>
<complete new file contents>
END

Open one edit block per file. Do not explain your changes in prose — the edit blocks are your entire output. If you need to create a new file, use the same format with the new path.

When done, add a single line: PR_TITLE: <one-line summary of the change>

IMPORTANT: Do NOT modify CLAUDE.md, .claude/, or .github/ files. Your task is to implement the issue, not modify project configuration.`+githubSourceInstructions,
		task.Issue.Number, task.Issue.Title)
}

func buildTestPrompt(task Task) string {
	return fmt.Sprintf(`You are a test agent. Your task is to write or fix tests for issue #%d: %s.

Produce test file edits in the same EDIT/END format as code-gen. Focus on:
- Table-driven tests for new functions
- Edge cases identified in the issue
- Regression tests for bug fixes

Add a line: PR_TITLE: test: <what is being tested>`+githubSourceInstructions,
		task.Issue.Number, task.Issue.Title)
}

func buildDocsPrompt(task Task) string {
	return fmt.Sprintf(`You are a documentation agent. Your task is to update documentation for issue #%d: %s.

Produce markdown file edits in the EDIT/END format. Ensure:
- No trailing whitespace
- Proper heading hierarchy
- All links are relative and valid

Add a line: PR_TITLE: docs: <what was documented>`+githubSourceInstructions,
		task.Issue.Number, task.Issue.Title)
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
	return fmt.Sprintf(`You are a quality control agent reviewing the work on issue #%d: %s.

You must evaluate the output independently — do not read the producing agent's reasoning or comments. Evaluate only the code/content itself against:
- Correctness: does it do what the issue asked?
- Quality: does it follow project conventions?
- Tests: are there adequate tests?
- Risk: any regressions or side effects?

Respond with: PASS or FAIL, followed by specific findings. No praise, no encouragement — only concrete evaluation.`,
		task.Issue.Number, task.Issue.Title)
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
