package mcp

import (
	"context"
	"time"

	"samverk.dev/samverk/internal/forge"
)

// ClaimResult is returned when an issue is successfully claimed.
type ClaimResult struct {
	IssueNumber int    `json:"issue_number"`
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	WorkerID    string `json:"worker_id"`
	ClaimedAt   string `json:"claimed_at"` // RFC3339
	Phase       string `json:"phase,omitempty"`
}

// WorkRequestOptions are optional filters for RequestWork.
type WorkRequestOptions struct {
	Project    string
	Labels     []string
	Priority   string
	Complexity string
}

// HeartbeatResult is returned by HeartbeatIssue.
type HeartbeatResult struct {
	ClaimDuration string `json:"claim_duration"`
	Status        string `json:"status"`
}

// ClaimInfo holds read-only info about a current claim.
type ClaimInfo struct {
	WorkerID  string `json:"worker_id"`
	ClaimedAt string `json:"claimed_at"` // RFC3339
	Duration  string `json:"duration"`
}

// WorkCoordinator defines the dispatcher interface for MCP work tools.
// The dispatcher implements this; the MCP package depends only on the interface.
type WorkCoordinator interface {
	// ClaimIssue claims a specific issue for an interactive worker.
	ClaimIssue(ctx context.Context, owner, repo string, number int, workerID string) (*ClaimResult, error)

	// RequestWork finds the next best queued issue matching filters and claims it.
	RequestWork(ctx context.Context, workerID string, opts *WorkRequestOptions) (*ClaimResult, *forge.Issue, error)

	// CompleteIssue marks a claimed issue as done (status:claimed -> status:needs-qc).
	CompleteIssue(ctx context.Context, owner, repo string, number int, workerID string, prNumber int, summary string) error

	// ReleaseIssue returns a claimed issue to the queue without completing it.
	ReleaseIssue(ctx context.Context, owner, repo string, number int, workerID string, reason string) error

	// HeartbeatIssue resets the heartbeat timer for a claimed issue.
	HeartbeatIssue(ctx context.Context, owner, repo string, number int, workerID string) (*HeartbeatResult, error)

	// GetClaimInfo returns claim info for an issue, or nil if not claimed.
	GetClaimInfo(owner, repo string, number int) *ClaimInfo
}

// defaultWorkerID generates a worker ID from the hostname when none is provided.
func defaultWorkerID() string {
	return "interactive:" + time.Now().Format("20060102T150405")
}
