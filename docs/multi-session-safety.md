# Multi-Session Work Safety

**Purpose**: Prevent breaking changes when working across multiple projects and AI sessions simultaneously. These are manual guardrails for the current development phase — Samverk will eventually automate most of this.

## The Problem

You work on multiple projects (Samverk, DockPulse, PacketDeck, Subnetree, etc.) across multiple tools (claude.ai, Claude Code, VS Code, terminal) often in the same day. There is currently no mechanism to:

- Detect that two sessions are touching the same repo
- Prevent uncommitted changes from being overwritten or forgotten
- Ensure that changes made in a design session (claude.ai) don't conflict with changes made in an implementation session (Claude Code)
- Alert you when you're editing files that an agent session also has open

## Short-Term Rules (Manual Discipline)

### Before Starting Any Session

1. **Check git status first.** Before opening a project in any tool, run:

   ```powershell
   cd D:\devspace\<project>; git status
   ```

   If there are uncommitted changes, either commit them or stash them before starting new work. Uncommitted changes from a previous session are the #1 source of silent conflicts.

2. **Check CLAUDE.md for handoff notes.** If a previous session left a "Pending Session Handoff" section, handle it before doing anything else.

3. **One writer per repo at a time.** Do not run Claude Code on a repo while also having claude.ai make changes to files in the same repo via Desktop Commander or Filesystem tools. Reading is fine; writing is not.

### During a Session

1. **Commit early, commit often.** Don't let a session accumulate a large batch of uncommitted changes. If a logical unit of work is complete (a doc, a feature, a fix), commit it. This reduces the blast radius if something goes wrong.

2. **Branch for experiments.** If you're trying something that might not work, branch first:

   ```powershell
   git checkout -b experiment/description
   ```

   This protects main from half-finished work.

### When Ending a Session

1. **Commit or document.** Before ending any session that modified files:
   - **Preferred**: Commit the changes with a descriptive message
   - **If not ready to commit**: Add a handoff section to CLAUDE.md describing what changed and why, so the next session can pick it up

2. **Check for orphaned changes.** Quick scan across active projects:

   ```powershell
   foreach ($project in @("Samverk", "DockPulse", "PacketDeck", "Subnetree")) {
       Write-Host "=== $project ===" -ForegroundColor Cyan
       Set-Location "D:\devspace\$project"
       git status --short
   }
   ```

## Active Project Registry

Keep this list current. If a project isn't listed here, it's not being actively developed.

| Project | Location | Primary Tool | Status |
|---------|----------|-------------|--------|
| Samverk | D:\devspace\Samverk | Claude Code | Phase 1 complete, docs expanding |
| DockPulse | D:\devspace\DockPulse | Not started | Scaffolded, needs git remote |
| PacketDeck | D:\devspace\PacketDeck | Not started | Scaffolded, needs git init |
| Subnetree | D:\devspace\Subnetree | Claude Code | Active development |
| RunNotes | D:\devspace\RunNotes | Claude Code | Active development |

## Long-Term Vision (Samverk Feature)

When Samverk is operational, the user should work through the same issue-based workflow as agents:

1. **User checks out an issue** — claims a task just like an agent would
2. **User works on a branch** — isolated from main, same as agent branches
3. **User submits changes** — PR or equivalent, goes through the same QC validation
4. **Samverk validates** — linting, tests, doc consistency, conflict detection
5. **Changes merge** — only after passing the same gates agents pass

This means the user doesn't have special "break glass" access to main by default. Direct file edits bypass every safeguard in the system. The user is the most dangerous actor precisely because they have the most access and the least structure.

This is NOT about restricting the user — it's about giving the user the same safety net that agents get. Agents can't accidentally push to main, forget to run tests, or leave uncommitted changes. The user shouldn't have to rely on memory to avoid those things either.

See [open-questions.md](docs/open-questions.md) § Multi-Session Coordination for the design questions this raises.
