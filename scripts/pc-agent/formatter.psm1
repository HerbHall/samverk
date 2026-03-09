#Requires -Version 7.0
<#
.SYNOPSIS
    PC Agent prompt formatter — converts a Samverk issue into a CC task prompt.

.DESCRIPTION
    Produces a structured prompt that Claude Code can follow to implement a
    Samverk issue in an isolated worktree.
#>

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'

function Format-IssuePrompt {
    <#
    .SYNOPSIS
        Formats a Samverk issue as a Claude Code task prompt.
    .DESCRIPTION
        The prompt instructs CC to:
        - Read CLAUDE.md for project conventions (it is auto-loaded from the worktree)
        - Run `make ci` before committing (build + test + lint)
        - Commit with a conventional commit message referencing the issue number
        - Push the branch to origin when done
        - Write AGENT_BLOCKED.md if it cannot proceed
    .PARAMETER Issue
        PSCustomObject with Number, Title, Body, AgentType fields.
    .PARAMETER Branch
        The worktree branch name (e.g. fix/42-add-feature).
    .OUTPUTS
        String — the complete prompt to pass to `claude --print`.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][PSCustomObject]$Issue,
        [Parameter(Mandatory)][string]$Branch
    )

    return @"
You are a Samverk autonomous agent implementing GitHub issue #$($Issue.Number).

## Task

Title: $($Issue.Title)
Agent type: $($Issue.AgentType)
Branch: $Branch

## Issue Body

$($Issue.Body)

## Instructions

You are working in an isolated git worktree on branch `$Branch`. This is NOT
the developer's working copy — you are fully isolated.

1. Read `CLAUDE.md` in the current directory for project conventions, build
   commands, and coding standards. It is loaded automatically from this directory.

2. Implement the acceptance criteria described in the issue body above.

3. Before committing, run `make ci` to verify the build passes (build + test + lint).
   Fix any failures before proceeding.

4. Commit your changes using a conventional commit message:
   `git commit -m "feat(#$($Issue.Number)): <concise summary>"`
   Include `Co-Authored-By: Claude <noreply@anthropic.com>` in the commit body.

5. Push the branch to origin when done:
   `git push origin $Branch`

6. Do NOT open a pull request — the post-task handler does that.

7. If you cannot complete the task (missing information, ambiguous requirements,
   dependency issues, or a blocking error you cannot resolve), create a file
   named `AGENT_BLOCKED.md` in the project root with:
   - What you tried
   - What is blocking you
   - What information would unblock you
   Then exit. Do NOT commit incomplete work.

## Important Constraints

- Only work in the current directory (the worktree). Never access `D:\DevSpace\`.
- Do not modify files outside the project (no system files, no other repos).
- Do not install new system packages or tools.
- If a test is flaky, note it in your commit message — do not disable the test.
"@
}

Export-ModuleMember -Function @('Format-IssuePrompt')
