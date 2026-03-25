package dispatcher

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"regexp"
	"strconv"
	"time"
)

// Heartbeat represents a parsed heartbeat comment from an agent.
type Heartbeat struct {
	AgentID   string
	Timestamp time.Time
	Progress  int
	Status    string
}

// heartbeatHeaderRe matches the first line: HEARTBEAT [agent-id] [iso-timestamp]
var heartbeatHeaderRe = regexp.MustCompile(
	`HEARTBEAT\s+\[([^\]]+)\]\s+\[([^\]]+)\]`,
)

// heartbeatProgressRe matches "progress: N%"
var heartbeatProgressRe = regexp.MustCompile(`(?m)^progress:\s*(\d+)%`)

// heartbeatStatusRe matches "status: <text>"
var heartbeatStatusRe = regexp.MustCompile(`(?m)^status:\s*(.+)`)

// parseHeartbeat extracts heartbeat data from a comment body.
// Returns nil if the comment is not a heartbeat.
func parseHeartbeat(body string) *Heartbeat {
	headerMatch := heartbeatHeaderRe.FindStringSubmatch(body)
	if headerMatch == nil {
		return nil
	}

	ts, err := time.Parse(time.RFC3339, headerMatch[2])
	if err != nil {
		return nil
	}

	hb := &Heartbeat{
		AgentID:   headerMatch[1],
		Timestamp: ts,
	}

	if progressMatch := heartbeatProgressRe.FindStringSubmatch(body); progressMatch != nil {
		hb.Progress, _ = strconv.Atoi(progressMatch[1])
	}

	if statusMatch := heartbeatStatusRe.FindStringSubmatch(body); statusMatch != nil {
		hb.Status = statusMatch[1]
	}

	return hb
}

// checkTimeouts iterates claimed issues and releases any that have timed out.
func (d *Dispatcher) checkTimeouts(ctx context.Context) error {
	d.mu.Lock()
	// Snapshot the claimed map to avoid holding the lock during tracker calls.
	timedOut := make([]string, 0)
	now := time.Now()
	timeout := time.Duration(float64(d.config.HeartbeatInterval) * d.config.HeartbeatTimeoutMultiplier)

	for key, claimed := range d.claimed {
		if now.Sub(claimed.LastHeartbeat) > timeout {
			timedOut = append(timedOut, key)
		}
	}
	d.mu.Unlock()

	for _, key := range timedOut {
		if err := d.releaseTimedOut(ctx, key); err != nil {
			d.logger.Warn("release timeout", zap.String("key", key), zap.String("error", err.Error()))
		}
	}
	return nil
}

// releaseTimedOut unclaims a single timed-out issue and re-queues it.
// Uses persisted failure counts (SQLite) so the counter survives restarts.
// Falls back to in-memory counting when no store is configured.
// The key is a composite issueKey(owner, repo, number).
func (d *Dispatcher) releaseTimedOut(ctx context.Context, key string) error {
	d.mu.Lock()
	claimed, ok := d.claimed[key]
	if !ok {
		d.mu.Unlock()
		return nil
	}
	// Increment in-memory counter (always works, even without store).
	claimed.FailureCount++
	inMemoryCount := claimed.FailureCount
	agentID := claimed.AgentID
	owner := claimed.Owner
	repo := claimed.Repo
	lastHB := claimed.LastHeartbeat
	delete(d.claimed, key)
	d.issueFailures[key] = inMemoryCount
	d.mu.Unlock()

	tracker := d.trackerFor(owner, repo)
	if tracker == nil {
		return fmt.Errorf("no tracker for %s/%s", owner, repo)
	}

	// Extract issue number from the claimed entry. We need it for API calls.
	// The issue number is embedded in the key but we need to parse it or
	// store it. We can get it from the tracker by looking at the key format.
	// For now, we re-derive it from the issue key.
	issueNumber := parseIssueNumber(key)

	// Record failure event (also increments the persisted counter).
	elapsed := time.Since(lastHB)
	errMsg := fmt.Sprintf("timeout: agent %s missed heartbeat, last seen %s ago", agentID, elapsed.Truncate(time.Second))
	d.recordFailure(ctx, issueNumber, "", agentID, "", errMsg, elapsed)

	// Use persisted count when available (survives restarts); fall back to in-memory.
	failureCount := inMemoryCount
	if d.store != nil {
		if persisted := d.getPersistedFailureCount(ctx, issueNumber); persisted > failureCount {
			failureCount = persisted
			d.mu.Lock()
			d.issueFailures[key] = failureCount
			d.mu.Unlock()
		}
	}

	comment := fmt.Sprintf(
		"RELEASE [dispatcher] [%s] timeout\nAgent %s missed heartbeat. Last seen: %s.\nUnclaiming issue for re-queue. (attempt %d)",
		time.Now().UTC().Format(time.RFC3339), agentID, lastHB.UTC().Format(time.RFC3339), failureCount,
	)
	if _, err := tracker.AddComment(ctx, issueNumber, comment); err != nil {
		d.logger.Warn("add comment", zap.Int("issue", issueNumber), zap.String("context", "timeout-release"), zap.String("error", err.Error()))
	}

	// Remove in-progress or claimed label.
	_ = tracker.RemoveLabel(ctx, issueNumber, "status:in-progress")
	_ = tracker.RemoveLabel(ctx, issueNumber, "status:claimed")

	if err := tracker.AddLabel(ctx, issueNumber, "status:queued"); err != nil {
		d.logger.Warn("add label", zap.Int("issue", issueNumber), zap.String("label", "queued"), zap.String("error", err.Error()))
	}
	if err := tracker.Unassign(ctx, issueNumber, agentID); err != nil {
		d.logger.Warn("unassign", zap.Int("issue", issueNumber), zap.String("agent", agentID), zap.String("error", err.Error()))
	}

	d.logger.Warn("timeout released",
		zap.Int("issue", issueNumber),
		zap.String("agent", agentID),
		zap.Int("failures", failureCount),
		zap.Duration("since_heartbeat", elapsed.Truncate(time.Second)),
	)

	// Escalate after max consecutive failures.
	if failureCount >= d.config.MaxConsecutiveFailures {
		// Remove status:queued BEFORE adding needs-human to prevent a race
		// where handleLabeled fires on the queued event before needs-human
		// is applied, causing the issue to be re-routed.
		_ = tracker.RemoveLabel(ctx, issueNumber, "status:queued")

		escalateComment := fmt.Sprintf(
			"ESCALATE [dispatcher] [%s]\ntrigger: %d_consecutive_failures\nseverity: high\nissue: #%d\ndetails: Agent %s failed %d times on this issue.\naction_needed: Review issue complexity and acceptance criteria.",
			time.Now().UTC().Format(time.RFC3339),
			failureCount, issueNumber, agentID, failureCount,
		)
		if err := tracker.AddLabel(ctx, issueNumber, "status:needs-human"); err != nil {
			d.logger.Error("add label", zap.Int("issue", issueNumber), zap.String("label", "needs-human"), zap.String("error", err.Error()))
		}
		if _, err := tracker.AddComment(ctx, issueNumber, escalateComment); err != nil {
			d.logger.Error("add comment", zap.Int("issue", issueNumber), zap.String("context", "escalate"), zap.String("error", err.Error()))
		}
	}

	return nil
}

// parseIssueNumber extracts the issue number from a composite key like "owner/repo#42".
func parseIssueNumber(key string) int {
	idx := len(key) - 1
	for idx >= 0 && key[idx] >= '0' && key[idx] <= '9' {
		idx--
	}
	if idx < 0 || key[idx] != '#' {
		return 0
	}
	n, _ := strconv.Atoi(key[idx+1:])
	return n
}
