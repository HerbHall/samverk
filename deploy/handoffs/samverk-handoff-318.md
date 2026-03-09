# Handoff: Fix #318 — Completed Tasks Time Out and Re-Queue

**Date:** 2026-03-08
**Type:** Tier 1 — Execute immediately, fix is fully specified
**Repo:** `D:\DevSpace\Samverk`
**Branch:** Create `fix/issue-318-completion-callback` from `main`
**Priority:** CRITICAL — actively burning tokens on every dispatched task

---

## Context

Completed agent tasks are not removed from the dispatcher's `claimed` map. The heartbeat timeout sweep treats them as failures 30 minutes later, re-queues them with `status:queued`, and they burn tokens again. Evidence in dispatcher logs: issues #268 and #304 both completed successfully but were released as timeouts.

Issue: [#318](https://github.com/HerbHall/samverk/issues/318)

---

## Root Cause

`internal/agent/pool.go` `processTask()` calls `runner.Run()`, logs "task completed", but never notifies the dispatcher. The dispatcher's `claimed` map only gets cleaned by `handleClosed` (external close event) or `releaseTimedOut` (the bug path). There is no completion callback from pool → dispatcher.

---

## Changes Required (3 files + tests)

### 1. `internal/agent/pool.go`

**Add TaskResult type and callback:**

```go
// TaskResult reports the outcome of a pool task back to the dispatcher.
type TaskResult struct {
    IssueNumber int
    SessionID   string
    AgentType   models.AgentType
    Success     bool
    Error       string
}
```

Add to the `Pool` struct:

```go
onComplete func(TaskResult) // callback to notify dispatcher of task completion
```

Add setter method:

```go
// SetOnComplete registers a callback invoked after each task finishes.
func (p *Pool) SetOnComplete(fn func(TaskResult)) {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.onComplete = fn
}
```

**Modify `processTask` to call the callback:**

After the existing runner execution and logging, add the callback invocation. The callback must fire on BOTH success and failure paths. Current code:

```go
func (p *Pool) processTask(task Task) {
    // ... provider resolution ...
    
    runner := NewRunner(prov, model, p.tracker, p.store, p.costs)
    if err = runner.Run(ctx, task); err != nil {
        logger.Error("runner failed", slog.String("error", err.Error()))
        return  // <-- BUG: no callback on failure either
    }
    
    logger.Info("task completed")
}
```

Replace with:

```go
func (p *Pool) processTask(task Task) {
    // ... existing provider resolution (unchanged) ...
    
    runner := NewRunner(prov, model, p.tracker, p.store, p.costs)
    runErr := runner.Run(ctx, task)
    
    // Notify dispatcher of completion (success or failure).
    result := TaskResult{
        IssueNumber: task.Issue.Number,
        SessionID:   task.SessionID,
        AgentType:   task.AgentType,
        Success:     runErr == nil,
    }
    if runErr != nil {
        result.Error = runErr.Error()
        logger.Error("runner failed", slog.String("error", runErr.Error()))
    } else {
        logger.Info("task completed")
    }
    
    p.mu.Lock()
    cb := p.onComplete
    p.mu.Unlock()
    if cb != nil {
        cb(result)
    }
}
```

**IMPORTANT:** Also handle the early-return path where no provider is found. That path already calls `p.failSession` — add the callback there too:

```go
if err != nil {
    logger.Error("no healthy provider", slog.String("error", err.Error()))
    p.failSession(ctx, task.SessionID, fmt.Sprintf("no healthy provider: %v", err))
    // Notify dispatcher even on provider failure.
    p.mu.Lock()
    cb := p.onComplete
    p.mu.Unlock()
    if cb != nil {
        cb(TaskResult{
            IssueNumber: task.Issue.Number,
            SessionID:   task.SessionID,
            AgentType:   task.AgentType,
            Success:     false,
            Error:       err.Error(),
        })
    }
    return
}
```

### 2. `internal/dispatcher/dispatcher.go`

**Add the completion handler method:**

```go
// handleTaskComplete is called by the agent pool when a task finishes.
// It removes the issue from the claimed map and updates labels.
func (d *Dispatcher) handleTaskComplete(result agent.TaskResult) {
    d.mu.Lock()
    delete(d.claimed, result.IssueNumber)
    if result.Success {
        delete(d.issueFailures, result.IssueNumber)
    }
    d.mu.Unlock()

    ctx := context.Background()

    _ = d.tracker.RemoveLabel(ctx, result.IssueNumber, "status:claimed")
    _ = d.tracker.RemoveLabel(ctx, result.IssueNumber, "status:in-progress")

    if result.Success {
        if err := d.tracker.AddLabel(ctx, result.IssueNumber, "status:needs-qc"); err != nil {
            d.logger.Printf("add needs-qc to #%d: %v", result.IssueNumber, err)
        }
        d.logger.Printf("task #%d completed successfully, moved to needs-qc", result.IssueNumber)
    } else {
        if err := d.tracker.AddLabel(ctx, result.IssueNumber, "status:queued"); err != nil {
            d.logger.Printf("add queued to #%d: %v", result.IssueNumber, err)
        }
        d.logger.Printf("task #%d failed (%s), re-queued", result.IssueNumber, result.Error)
    }
}
```

**Wire it up in `Run` method:**

In the `Run` method, after the pool is confirmed non-nil (the pool is set before `Run` is called — find where the dispatcher is constructed in `cmd/samverk/` and wire it there). The safest place is at the start of `Run`, before the watch loop:

```go
func (d *Dispatcher) Run(ctx context.Context) error {
    ctx, cancel := context.WithCancel(ctx)
    d.stop = cancel

    // Register completion callback with agent pool.
    if d.pool != nil {
        d.pool.SetOnComplete(d.handleTaskComplete)
    }

    // ... rest of existing Run method unchanged ...
```

### 3. Tests

**`internal/agent/pool_test.go`** — Add test:

```go
func TestPool_OnCompleteCallback(t *testing.T) {
    // Test that the callback fires on successful completion
    // Test that the callback fires on runner failure
    // Test that the callback fires on provider-not-found failure
    // Test that result.Success is true on success, false on failure
}
```

**`internal/dispatcher/dispatcher_test.go`** — Add test:

```go
func TestHandleTaskComplete_Success(t *testing.T) {
    // Setup: issue in claimed map
    // Call handleTaskComplete with Success=true
    // Assert: issue removed from claimed map
    // Assert: issueFailures cleared
    // Assert: status:needs-qc label added
    // Assert: status:claimed label removed
}

func TestHandleTaskComplete_Failure(t *testing.T) {
    // Setup: issue in claimed map
    // Call handleTaskComplete with Success=false
    // Assert: issue removed from claimed map
    // Assert: issueFailures NOT cleared (preserved for escalation)
    // Assert: status:queued label added
    // Assert: status:claimed label removed
}

func TestNoDoubleDispatch_AfterCompletion(t *testing.T) {
    // Setup: route an issue, complete it via callback
    // Assert: checkTimeouts does NOT release it (not in claimed map)
}
```

---

## Git Workflow (MANDATORY)

1. Create a feature branch from main:

   ```bash
   git checkout main && git pull origin main
   git checkout -b fix/issue-318-completion-callback
   ```

2. Make all changes on the feature branch (never on main).

3. Run tests:

   ```bash
   go test ./internal/agent/... ./internal/dispatcher/...
   ```

4. Run full CI:

   ```bash
   make ci
   ```

5. Commit:

   ```bash
   git add internal/agent/pool.go internal/dispatcher/dispatcher.go internal/agent/pool_test.go internal/dispatcher/dispatcher_test.go
   git commit -m "fix(#318): add completion callback from agent pool to dispatcher

   Completed tasks were not removed from the dispatcher's claimed map,
   causing the heartbeat timeout to re-queue them and burn tokens again.

   - Add TaskResult type and OnComplete callback to agent.Pool
   - Add handleTaskComplete method to Dispatcher
   - Wire callback in Dispatcher.Run
   - Callback fires on success, runner failure, and provider failure

   Closes #318

   Co-Authored-By: Claude <noreply@anthropic.com>"
   ```

6. Push and open PR:

   ```bash
   git push origin HEAD
   gh pr create --title "fix(#318): add completion callback from agent pool to dispatcher" --body "Closes #318"
   ```

**CRITICAL: Never commit directly to main.**

---

## Validation

After merging:

1. Redeploy to CT 202: `make redeploy`
2. Watch dispatcher logs: `ssh root@192.168.1.162 journalctl -u samverk-dispatch -f`
3. Verify: tasks that complete should show "moved to needs-qc" in logs, NOT "released timed out"
4. Verify: no issues get `failures=1` after successful completion

---

## Commit Message

```text
fix(#318): add completion callback from agent pool to dispatcher
```
