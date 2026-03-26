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
func FormatInstructions() string {
	return `## Output Format

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
}

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
