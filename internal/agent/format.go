package agent

import "strings"

// Output format constants — single source of truth for agent response format.
// All components (prompts, parser, validator, quality gate) must reference these.
//
// The canonical format for code-gen/test/docs agents:
//
//	EDIT <filepath>
//	<complete file contents>
//	END
//
//	PR_TITLE: <conventional commit summary>
//
// Research agents produce markdown comments (no EDIT blocks).
// QC agents produce PASS/FAIL verdicts (no EDIT blocks).

const (
	// EditBlockPrefix is the line prefix that opens an EDIT block.
	// The parser accepts "EDIT <filepath>" on its own line.
	EditBlockPrefix = "EDIT "

	// EditBlockEnd is the exact line that closes an EDIT block.
	EditBlockEnd = "END"

	// PRTitlePrefix is the line prefix for the PR title extraction.
	PRTitlePrefix = "PR_TITLE:"

	// FencedCodeMarker is the markdown fenced code block delimiter.
	// Agents may use this in CLI workspace mode where EDIT blocks aren't needed.
	FencedCodeMarker = "```"
)

// FormatInstructions returns the canonical output format instructions for
// code-producing agents (code-gen, test, docs). This is the single source
// of truth that agent prompts must include.
//
// For API agents (no tool access): returns EDIT block format instructions.
// For CLI agents (tool access): returns tool-based workflow instructions.
func FormatInstructions() string {
	return FormatInstructionsForProvider(false)
}

// FormatInstructionsForProvider returns output format instructions tailored
// to the provider's capabilities.
func FormatInstructionsForProvider(hasToolAccess bool) string {
	if hasToolAccess {
		return formatInstructionsCLI
	}
	return formatInstructionsAPI
}

// formatInstructionsCLI is for CLI agents (claude-cli) that have filesystem
// tools (Read, Write, Edit, Glob, Grep). They implement changes directly
// in the worktree -- no EDIT blocks needed.
var formatInstructionsCLI = `## Output Format

You have full filesystem access via your tools (Read, Write, Edit, Glob, Grep, Bash).
Implement the changes directly in the source tree:

1. Read the relevant files to understand the existing code
2. Use Write or Edit to create or modify files
3. Run build and test commands via Bash to verify your changes compile and pass
4. Do NOT modify CLAUDE.md, .claude/, or .github/ files

After making all changes, include a PR title line in your response:

PR_TITLE: <conventional commit type>: <brief description>`

// formatInstructionsAPI is for API agents (Ollama) that cannot access the
// filesystem. They must return structured EDIT blocks in their response.
var formatInstructionsAPI = `## Output Format

Your response MUST use EDIT blocks for every file you create or modify.
Each block contains the COMPLETE file contents (not a diff).

Format:

EDIT <filepath>
<complete new file contents>
END

Rules:
- One EDIT block per file
- EDIT and END must be on their own lines with no other text
- Include the COMPLETE file contents, not just changes
- Do NOT use markdown code fences (` + "```" + `) for file contents
- Do NOT modify CLAUDE.md, .claude/, or .github/ files

After all EDIT blocks, include a PR title:

PR_TITLE: <conventional commit type>: <brief description>`

// HasEditBlocks returns true if the output contains at least one EDIT block
// marker. Used by the quality gate and validator to detect code output.
// Checks for both "EDIT " at line start and after newlines.
func HasEditBlocks(output string) bool {
	// Check start of output (no preceding newline).
	if len(output) >= len(EditBlockPrefix) && output[:len(EditBlockPrefix)] == EditBlockPrefix {
		return true
	}
	// Check after any newline in the output.
	return strings.Contains(output, "\n"+EditBlockPrefix)
}

// HasCodeOutput returns true if the output contains either EDIT blocks or
// fenced code blocks. This is the canonical check for "did the agent
// produce code?" — used by the quality gate.
func HasCodeOutput(output string) bool {
	return HasEditBlocks(output) || strings.Contains(output, FencedCodeMarker)
}
