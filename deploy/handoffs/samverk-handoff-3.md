# Samverk Handoff #3 — Apply Agent-Generated Fixes

*Generated: 2026-03-08 | Source: claude.ai session*
*Dispatcher is STOPPED on CT 202 — do NOT restart until all fixes are deployed*

## Context

Agents produced correct code for 4 critical fixes but couldn't push PRs before heartbeat
timeout killed sessions. Code exists in GitHub issue comments as EDIT blocks. Apply these
fixes, verify with `make ci`, commit, and deploy.

Read each issue's comments via `gh issue view <N> --comments` to get the EDIT blocks.
Use the LAST comment's version — it's the cleanest. Do NOT rewrite from scratch.

## Fix 1: Failure counter reset (#233) — CRITICAL, DO FIRST

Failure counter resets to 1 every re-queue cycle. Issues loop forever instead of escalating.

`gh issue view 233 --comments` — Files: dispatcher.go, heartbeat.go, router.go, dispatcher_test.go

Key changes: `Retrying bool` on claimedIssue, releaseTimedOut sets Retrying=true instead of
delete, checkTimeouts skips Retrying entries, route() preserves FailureCount, escalate after 3.

## Fix 2: RemoveLabel 404 suppress (#237)

`gh issue view 237 --comments` — Files: github/github.go, github/github_test.go

Key change: RemoveLabel returns nil on HTTP 404 instead of error.

## Fix 3: Ollama timeout (#239)

`gh issue view 239 --comments` — Files: ollama/ollama.go, ollama/ollama_test.go, cmd/samverk/main.go, deploy/config/providers.yaml

Key changes: Add NewWithTimeout constructor, wire cfg.TimeoutSeconds in ollama factory case,
set timeout_seconds: 120 for ollama-coder in providers.yaml.

## Fix 4: Heartbeat timeout headroom (no issue — config change)

In `internal/dispatcher/config.go`, DefaultConfig():
Change `HeartbeatInterval: 10 * time.Minute` to `HeartbeatInterval: 20 * time.Minute`
This gives 30min effective timeout (20 x 1.5), enough for opus + PR lifecycle.

## Workflow

1. `gh issue view 233 --comments` — apply EDIT blocks from last comment
2. `gh issue view 237 --comments` — apply EDIT blocks from last comment
3. `gh issue view 239 --comments` — apply EDIT blocks from last comment
4. Edit config.go heartbeat interval manually
5. `make ci` — must pass
6. Commit each fix separately:
   - fix(#233): preserve failure counter across heartbeat re-queue cycles
   - fix(#237): suppress 404 on RemoveLabel when label is not present
   - fix(#239): wire timeout_seconds config to Ollama provider
   - fix: increase heartbeat interval to 20min for opus session headroom
7. Push to main, cross-compile, deploy to CT 202
8. `ssh root@192.168.1.162 "systemctl start samverk-dispatch"`
9. Verify: `journalctl -u samverk-dispatch -n 20 --no-pager`

## DO NOT

- Do NOT restart samverk-dispatch until ALL fixes deployed and make ci passes
- Do NOT re-queue any status:needs-human issues yet
- Do NOT modify issues #240-#246 (future work)

## ADDENDUM — Fix 5: Dispatcher routing gate (#247) — CRITICAL, ADD TO BATCH

Filed after initial handoff. Must deploy BEFORE restarting dispatcher or all status:needs-human
issues will immediately re-enter the routing loop.

`gh issue view 247 --comments` for full details.

**In `internal/dispatcher/dispatcher.go` handleOpened()**: Add label gate after fetching issue:

```go
labels := make(map[string]bool, len(issue.Labels))
for _, l := range issue.Labels {
    labels[l] = true
}
if labels["status:needs-human"] || labels["status:blocked"] || labels["status:claimed"] || labels["status:in-progress"] {
    d.logger.Printf("skipping #%d: already has status label", issue.Number)
    return nil
}
```

**In `internal/dispatcher/router.go` route()**: Add human gate before provider selection:

```go
if agentType == models.AgentTypeHuman {
    d.logger.Printf("issue #%d classified as human — not dispatching", issue.Number)
    _ = d.tracker.AddLabel(ctx, issue.Number, "status:needs-human")
    return nil
}
```

Commit as: fix(#247): dispatcher skips issues with terminal status labels and human agent type
