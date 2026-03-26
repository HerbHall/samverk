# Agent Output Format Specification

Single source of truth for agent response format across the pipeline.

## Code-Producing Agents (code-gen, test, docs)

### EDIT Block Format

```text
EDIT <filepath>
<complete file contents>
END
```

Rules:

- One EDIT block per file
- `EDIT` and `END` must be on their own lines with no other text
- Include COMPLETE file contents (not diffs)
- Do NOT use markdown code fences for file contents
- Do NOT modify `CLAUDE.md`, `.claude/`, or `.github/` files

### PR Title

After all EDIT blocks, include:

```text
PR_TITLE: <conventional commit type>: <brief description>
```

### Example

```text
EDIT internal/api/handler.go
package api

func handleRequest(w http.ResponseWriter, r *http.Request) {
    // implementation
}
END

EDIT internal/api/handler_test.go
package api

func TestHandleRequest(t *testing.T) {
    // tests
}
END

PR_TITLE: feat(api): add request handler with validation
```

## Research Agents

Research agents produce markdown comments (no EDIT blocks):

```text
## Summary
<brief overview>

## Findings
<detailed analysis>

## Recommendation
<go/no-go with reasoning>

## Sources
<references>
```

## QC Agents

QC agents produce verdicts:

```text
PASS or FAIL

## Findings
<specific items checked>
```

## Where Format Is Enforced

| Component | File | What it checks |
|-----------|------|---------------|
| Agent prompt | `internal/agent/format.go` | `FormatInstructions()` -- canonical instructions |
| Edit parser | `internal/agent/editparser.go` | `ParseEditBlocks()` -- extracts EDIT/END blocks |
| Validator | `internal/agent/validator.go` | `ValidateEditBlocks()` -- checks blocks exist |
| Quality gate | `internal/dispatcher/quality.go` | `agent.HasCodeOutput()` -- detects code output |

All components reference constants from `internal/agent/format.go`:

- `EditBlockPrefix` = `"EDIT "`
- `EditBlockEnd` = `"END"`
- `PRTitlePrefix` = `"PR_TITLE:"`
- `FencedCodeMarker` = `` ``` ``

## CLI Workspace Mode

When agents run with workspace isolation (worktree), they use filesystem
tools (Edit, Write, Bash) instead of EDIT blocks. The EDIT block format
only applies to the API flow where agents cannot modify files directly.

In workspace mode:

- Agent modifies files directly via CLI tools
- `CommitAndPush` handles staging, committing, and pushing
- No EDIT block parsing needed
- Quality gate still checks for code output (fenced blocks in response)

## Adding a New Format

To add a new output format:

1. Add constants to `internal/agent/format.go`
2. Update `FormatInstructions()` if agents need to produce it
3. Update `HasCodeOutput()` if the quality gate should recognize it
4. Update `ParseEditBlocks()` if the parser should extract it
5. Update this document
