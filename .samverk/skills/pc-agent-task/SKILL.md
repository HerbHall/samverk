# pc-agent-task

Discover, claim, and format a handoff prompt for the next `agent:pc` task.

## Triggers

- "get my next PC task"
- "what's my next agent:pc issue"
- "give me a PC agent task"
- "load my next task for VS Code"

## Workflow

### Step 1: Discover queued issues

Call `list_issues(labels=["agent:pc", "status:queued"])` via Samverk MCP.

If no issues are returned, report "No queued agent:pc issues found." and stop.

### Step 2: Select highest priority

Sort the returned issues by priority, then by age:

1. `priority:critical` (highest)
2. `priority:high`
3. `priority:normal`
4. Within the same priority tier, pick the oldest by `created_at`

Select the single highest-priority issue.

### Step 3: Fetch full issue details

Call `get_issue(issue_number=<N>)` to retrieve the complete issue body.

### Step 4: Read referenced files

Scan the issue body for file path references matching these patterns:

- Paths starting with `scripts/`, `docs/`, `internal/`, `pkg/`, `cmd/`, `.samverk/`
- Paths enclosed in backticks that look like file paths (contain `/` and a file extension)

For each detected path, call `read_file(path=<detected_path>)`. Collect
the contents for inclusion in the handoff prompt.

If a file cannot be read (not found), note it as `(file not found)` in the
referenced files section.

### Step 5: Claim the issue

Retrieve the current label set from the issue. Build a new label list:

- Remove `status:queued`
- Add `status:claimed`
- Preserve all other labels

Call `set_labels(issue_number=<N>, labels=[<new label list>])`.

### Step 6: Determine branch type and slug

**Branch type prefix:** Check the issue labels:

- If `fix` or `bug` label is present, use `fix`
- Otherwise use `feat`

**Slug generation:** Take the issue title, then:

1. Lowercase
2. Strip all characters except alphanumeric and spaces
3. Replace spaces with hyphens
4. Truncate at 40 characters

Branch name: `<type>/<N>-<slug>`

### Step 7: Render handoff prompt

Generate the following prompt and present it to the user for pasting into a
Claude Code session in VS Code:

````text
You are a Samverk PC agent. Implement issue #<N> using the Samverk MCP tools
available in this VS Code session.

## Task
Title: <title>
Branch: <type>/<N>-<slug>
Issue: http://gitea.herbhall.net/samverk/samverk/issues/<N>

## Issue Body
<full issue body>

## Referenced Files
<for each detected file path, show: "### <path>" followed by file contents>

## Workspace Setup
Open a terminal and run:
  git -C D:\bots\samverk.git fetch --all
  git worktree add D:\bots\worker-1 -b <branch> origin/main
  cd D:\bots\worker-1

(If worker-1 is in use, pick the next available slot: worker-2, worker-3)

## Implementation
- Read CLAUDE.md for conventions and build commands.
- Implement all acceptance criteria in the issue body.
- Run `make ci` before committing. Fix all failures.
- Commit: `git commit -m "<type>(#<N>): <summary>"`

## Completion (after make ci passes and changes are committed)

Run in terminal:
  git push origin <branch>

Then via Samverk MCP tools in this session:

1. Create PR:
   create_pr(title="<type>(#<N>): <title>", head="<branch>", base="main",
             body="Closes #<N>\n\n<one-line summary of changes>")
   Note the PR number returned.

2. Update issue labels (replace full set -- preserve all existing labels except
   status:claimed, add status:needs-qc):
   set_labels(issue_number=<N>, labels=[<existing labels minus status:claimed
   plus status:needs-qc>])

3. Post completion comment:
   add_comment(issue_number=<N>,
               body="PR opened: #<PR number> -- <branch>")

## If Blocked
Create AGENT_BLOCKED.md in the project root with:
- What you attempted
- What is blocking you
- What would unblock you

Then via Samverk MCP:
  set_labels(issue_number=<N>, labels=[<existing labels minus status:claimed
  plus status:needs-human>])

Do NOT commit incomplete work.
````

### Step 8: Report

Print a summary line: `Claimed issue #<N>: <title>. Handoff prompt rendered above.`

## Cancel

If the user says "skip", "cancel", or "nevermind", do not claim any issue.
Report "Cancelled -- no issue claimed." and stop.
