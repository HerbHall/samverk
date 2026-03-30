package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// forgeClaimedIssue tracks in-memory claim state for interactive workers.
type forgeClaimedIssue struct {
	WorkerID      string
	Owner         string
	Repo          string
	ClaimedAt     time.Time
	LastHeartbeat time.Time
}

// ForgeWorkCoordinator implements WorkCoordinator using direct forge operations.
// This is used in the serve process where the dispatcher is not running.
// It manages claims in memory and transitions labels via the forge API.
type ForgeWorkCoordinator struct {
	projects *ProjectRegistry
	claimed  map[string]*forgeClaimedIssue // key: owner/repo#number
	mu       sync.Mutex
	logger   *zap.Logger
}

// NewForgeWorkCoordinator creates a ForgeWorkCoordinator backed by the project registry.
func NewForgeWorkCoordinator(projects *ProjectRegistry, logger *zap.Logger) *ForgeWorkCoordinator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ForgeWorkCoordinator{
		projects: projects,
		claimed:  make(map[string]*forgeClaimedIssue),
		logger:   logger,
	}
}

func claimKey(owner, repo string, number int) string {
	return fmt.Sprintf("%s/%s#%d", strings.ToLower(owner), strings.ToLower(repo), number)
}

func (f *ForgeWorkCoordinator) trackerFor(owner, repo string) forge.IssueTracker {
	if f.projects == nil {
		return nil
	}
	for _, p := range f.projects.List() {
		if strings.EqualFold(p.Owner, owner) && strings.EqualFold(p.Repo, repo) {
			return p.Tracker
		}
	}
	return nil
}

// ClaimIssue claims a specific issue for an interactive worker.
func (f *ForgeWorkCoordinator) ClaimIssue(ctx context.Context, owner, repo string, number int, workerID string) (*ClaimResult, error) {
	key := claimKey(owner, repo, number)

	f.mu.Lock()
	if existing, ok := f.claimed[key]; ok {
		f.mu.Unlock()
		return nil, fmt.Errorf("issue %s/%s#%d is already claimed by %s", owner, repo, number, existing.WorkerID)
	}
	f.mu.Unlock()

	tracker := f.trackerFor(owner, repo)
	if tracker == nil {
		return nil, fmt.Errorf("no tracker for %s/%s", owner, repo)
	}

	// Transition labels.
	_ = tracker.RemoveLabel(ctx, number, models.LabelStatusQueued)
	if err := tracker.AddLabels(ctx, number, models.LabelStatusClaimed); err != nil {
		return nil, fmt.Errorf("add claimed label to #%d: %w", number, err)
	}

	now := time.Now()
	f.mu.Lock()
	f.claimed[key] = &forgeClaimedIssue{
		WorkerID:      workerID,
		Owner:         owner,
		Repo:          repo,
		ClaimedAt:     now,
		LastHeartbeat: now,
	}
	f.mu.Unlock()

	// Post claim comment.
	comment := fmt.Sprintf("CLAIMED [%s] [%s]\nClaimed by interactive worker `%s`.",
		workerID, now.UTC().Format(time.RFC3339), workerID)
	if _, err := tracker.AddComment(ctx, number, comment); err != nil {
		f.logger.Warn("add claim comment", zap.Int("issue", number), zap.Error(err))
	}

	f.logger.Info("interactive claim",
		zap.Int("issue", number),
		zap.String("worker", workerID),
		zap.String("owner", owner),
		zap.String("repo", repo),
	)

	return &ClaimResult{
		IssueNumber: number,
		Owner:       owner,
		Repo:        repo,
		WorkerID:    workerID,
		ClaimedAt:   now.UTC().Format(time.RFC3339),
	}, nil
}

// RequestWork finds the next best queued issue and claims it.
func (f *ForgeWorkCoordinator) RequestWork(ctx context.Context, workerID string, opts *WorkRequestOptions) (*ClaimResult, *forge.Issue, error) {
	if f.projects == nil {
		return nil, nil, fmt.Errorf("no projects configured")
	}

	labels := []string{models.LabelStatusQueued}
	if opts != nil {
		if opts.Priority != "" {
			labels = append(labels, "priority:"+opts.Priority)
		}
		if opts.Complexity != "" {
			labels = append(labels, "complexity:"+opts.Complexity)
		}
		labels = append(labels, opts.Labels...)
	}

	for _, p := range f.projects.List() {
		if p.Phase == "inactive" {
			continue
		}
		if opts != nil && opts.Project != "" && !strings.EqualFold(p.Name, opts.Project) {
			continue
		}
		if p.Tracker == nil {
			continue
		}

		issues, err := p.Tracker.ListIssues(ctx, &forge.ListOptions{
			State:  forge.StateOpen,
			Labels: labels,
		})
		if err != nil {
			f.logger.Warn("list queued issues", zap.String("project", p.Name), zap.Error(err))
			continue
		}

		for _, issue := range issues {
			if issue.IsPullRequest {
				continue
			}
			key := claimKey(p.Owner, p.Repo, issue.Number)
			f.mu.Lock()
			_, alreadyClaimed := f.claimed[key]
			f.mu.Unlock()
			if alreadyClaimed {
				continue
			}

			result, claimErr := f.ClaimIssue(ctx, p.Owner, p.Repo, issue.Number, workerID)
			if claimErr != nil {
				f.logger.Warn("claim during request_work", zap.Int("issue", issue.Number), zap.Error(claimErr))
				continue
			}
			return result, issue, nil
		}
	}

	return nil, nil, fmt.Errorf("no queued issues found matching filters")
}

// CompleteIssue marks a claimed issue as done.
func (f *ForgeWorkCoordinator) CompleteIssue(ctx context.Context, owner, repo string, number int, workerID string, prNumber int, summary string) error {
	key := claimKey(owner, repo, number)

	f.mu.Lock()
	claimed, ok := f.claimed[key]
	if !ok {
		f.mu.Unlock()
		return fmt.Errorf("issue %s/%s#%d is not claimed", owner, repo, number)
	}
	if claimed.WorkerID != workerID {
		holder := claimed.WorkerID
		f.mu.Unlock()
		return fmt.Errorf("issue %s/%s#%d is claimed by %s, not %s", owner, repo, number, holder, workerID)
	}
	delete(f.claimed, key)
	f.mu.Unlock()

	tracker := f.trackerFor(owner, repo)
	if tracker == nil {
		return fmt.Errorf("no tracker for %s/%s", owner, repo)
	}

	_ = tracker.RemoveLabel(ctx, number, models.LabelStatusClaimed)
	_ = tracker.RemoveLabel(ctx, number, models.LabelStatusInProgress)
	if err := tracker.AddLabels(ctx, number, models.LabelStatusNeedsQc); err != nil {
		f.logger.Warn("add needs-qc label", zap.Int("issue", number), zap.Error(err))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "COMPLETE [%s] [%s]\n", workerID, time.Now().UTC().Format(time.RFC3339))
	if prNumber > 0 {
		fmt.Fprintf(&sb, "PR: #%d\n", prNumber)
	}
	if summary != "" {
		fmt.Fprintf(&sb, "Summary: %s\n", summary)
	}
	if _, err := tracker.AddComment(ctx, number, sb.String()); err != nil {
		f.logger.Warn("add completion comment", zap.Int("issue", number), zap.Error(err))
	}

	f.logger.Info("interactive complete",
		zap.Int("issue", number),
		zap.String("worker", workerID),
		zap.Int("pr", prNumber),
	)

	return nil
}

// ReleaseIssue returns a claimed issue to the queue.
func (f *ForgeWorkCoordinator) ReleaseIssue(ctx context.Context, owner, repo string, number int, workerID, reason string) error {
	key := claimKey(owner, repo, number)

	f.mu.Lock()
	claimed, ok := f.claimed[key]
	if !ok {
		f.mu.Unlock()
		return fmt.Errorf("issue %s/%s#%d is not claimed", owner, repo, number)
	}
	if claimed.WorkerID != workerID {
		holder := claimed.WorkerID
		f.mu.Unlock()
		return fmt.Errorf("issue %s/%s#%d is claimed by %s, not %s", owner, repo, number, holder, workerID)
	}
	delete(f.claimed, key)
	f.mu.Unlock()

	tracker := f.trackerFor(owner, repo)
	if tracker == nil {
		return fmt.Errorf("no tracker for %s/%s", owner, repo)
	}

	_ = tracker.RemoveLabel(ctx, number, models.LabelStatusClaimed)
	_ = tracker.RemoveLabel(ctx, number, models.LabelStatusInProgress)
	if err := tracker.AddLabels(ctx, number, models.LabelStatusQueued); err != nil {
		f.logger.Warn("add queued label", zap.Int("issue", number), zap.Error(err))
	}

	comment := fmt.Sprintf("RELEASE [%s] [%s]\nReason: %s",
		workerID, time.Now().UTC().Format(time.RFC3339), reason)
	if _, err := tracker.AddComment(ctx, number, comment); err != nil {
		f.logger.Warn("add release comment", zap.Int("issue", number), zap.Error(err))
	}

	f.logger.Info("interactive release",
		zap.Int("issue", number),
		zap.String("worker", workerID),
		zap.String("reason", reason),
	)

	return nil
}

// HeartbeatIssue resets the heartbeat timer for a claimed issue.
func (f *ForgeWorkCoordinator) HeartbeatIssue(_ context.Context, owner, repo string, number int, workerID string) (*HeartbeatResult, error) {
	key := claimKey(owner, repo, number)

	f.mu.Lock()
	defer f.mu.Unlock()

	claimed, ok := f.claimed[key]
	if !ok {
		return nil, fmt.Errorf("issue %s/%s#%d is not claimed", owner, repo, number)
	}
	if claimed.WorkerID != workerID {
		return nil, fmt.Errorf("issue %s/%s#%d is claimed by %s, not %s", owner, repo, number, claimed.WorkerID, workerID)
	}

	claimed.LastHeartbeat = time.Now()
	duration := time.Since(claimed.ClaimedAt).Truncate(time.Second)

	return &HeartbeatResult{
		ClaimDuration: duration.String(),
		Status:        "active",
	}, nil
}

// GetClaimInfo returns read-only claim information for an issue.
func (f *ForgeWorkCoordinator) GetClaimInfo(owner, repo string, number int) *ClaimInfo {
	key := claimKey(owner, repo, number)

	f.mu.Lock()
	defer f.mu.Unlock()

	claimed, ok := f.claimed[key]
	if !ok {
		return nil
	}

	return &ClaimInfo{
		WorkerID:  claimed.WorkerID,
		ClaimedAt: claimed.ClaimedAt.UTC().Format(time.RFC3339),
		Duration:  time.Since(claimed.ClaimedAt).Truncate(time.Second).String(),
	}
}
