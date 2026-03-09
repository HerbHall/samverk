package dispatcher

import (
	"context"
	"fmt"
	"log/slog"
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
	timedOut := make([]int, 0)
	now := time.Now()
	timeout := time.Duration(float64(d.config.HeartbeatInterval) * d.config.HeartbeatTimeoutMultiplier)

	for num, claimed := range d.claimed {
		if now.Sub(claimed.LastHeartbeat) > timeout {
			timedOut = append(timedOut, num)
		}
	}
	d.mu.Unlock()

	for _, num := range timedOut {
		if err := d.releaseTimedOut(ctx, num); err != nil {
			d.logger.Warn("release timeout", slog.Int("issue", num), slog.String("error", err.Error()))
		}
	}
	return nil
}

// releaseTimedOut unclaims a single timed-out issue and re-queues it.
func (d *Dispatcher) releaseTimedOut(ctx context.Context, issueNumber int) error {
	d.mu.Lock()
	claimed, ok := d.claimed[issueNumber]
	if !ok {
		d.mu.Unlock()
		return nil
	}
	claimed.FailureCount++
	failureCount := claimed.FailureCount
	agentID := claimed.AgentID
	lastHB := claimed.LastHeartbeat
	delete(d.claimed, issueNumber)
	// Persist the incremented failure count so the next dispatch cycle picks it up.
	d.issueFailures[issueNumber] = failureCount
	d.mu.Unlock()

	comment := fmt.Sprintf(
		"RELEASE [dispatcher] [%s] timeout\nAgent %s missed heartbeat. Last seen: %s.\nUnclaiming issue for re-queue.",
		time.Now().UTC().Format(time.RFC3339), agentID, lastHB.UTC().Format(time.RFC3339),
	)
	if _, err := d.tracker.AddComment(ctx, issueNumber, comment); err != nil {
		d.logger.Warn("add comment", slog.Int("issue", issueNumber), slog.String("context", "timeout-release"), slog.String("error", err.Error()))
	}

	// Remove in-progress or claimed label.
	_ = d.tracker.RemoveLabel(ctx, issueNumber, "status:in-progress")
	_ = d.tracker.RemoveLabel(ctx, issueNumber, "status:claimed")

	if err := d.tracker.AddLabel(ctx, issueNumber, "status:queued"); err != nil {
		d.logger.Warn("add label", slog.Int("issue", issueNumber), slog.String("label", "queued"), slog.String("error", err.Error()))
	}
	if err := d.tracker.Unassign(ctx, issueNumber, agentID); err != nil {
		d.logger.Warn("unassign", slog.Int("issue", issueNumber), slog.String("agent", agentID), slog.String("error", err.Error()))
	}

	elapsed := time.Since(lastHB)
	d.logger.Warn("timeout released",
		slog.Int("issue", issueNumber),
		slog.String("agent", agentID),
		slog.Int("failures", failureCount),
		slog.Duration("since_heartbeat", elapsed.Truncate(time.Second)),
	)

	// Escalate after max consecutive failures.
	if failureCount >= d.config.MaxConsecutiveFailures {
		escalateComment := fmt.Sprintf(
			"ESCALATE [dispatcher] [%s]\ntrigger: %d_consecutive_failures\nseverity: high\nissue: #%d\ndetails: Agent %s failed %d times on this issue.\naction_needed: Review issue complexity and acceptance criteria.",
			time.Now().UTC().Format(time.RFC3339),
			failureCount, issueNumber, agentID, failureCount,
		)
		if err := d.tracker.AddLabel(ctx, issueNumber, "status:needs-human"); err != nil {
			d.logger.Error("add label", slog.Int("issue", issueNumber), slog.String("label", "needs-human"), slog.String("error", err.Error()))
		}
		if _, err := d.tracker.AddComment(ctx, issueNumber, escalateComment); err != nil {
			d.logger.Error("add comment", slog.Int("issue", issueNumber), slog.String("context", "escalate"), slog.String("error", err.Error()))
		}
	}

	return nil
}
