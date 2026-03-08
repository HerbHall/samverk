// Package forge provides the IssueTracker interface and implementations
// for GitHub, Gitea, and GitLab.
//
// The interface abstracts git forge operations so the dispatcher, agents,
// and MCP server work identically regardless of the underlying platform.
// See docs/forge-compatibility.md for the API surface comparison.
package forge

import (
	"context"
	"time"
)

// State represents the open/closed state of an issue.
type State string

const (
	StateOpen   State = "open"
	StateClosed State = "closed"
)

// EventType identifies what changed on the forge.
type EventType string

const (
	EventIssueOpened    EventType = "issue.opened"
	EventIssueClosed    EventType = "issue.closed"
	EventIssueLabeled   EventType = "issue.labeled"
	EventIssueAssigned  EventType = "issue.assigned"
	EventIssueCommented EventType = "issue.commented"
	EventIssueEdited    EventType = "issue.edited"
)

// IssueTracker is the platform-agnostic interface for git forge issue operations.
// Implementations exist for GitHub (internal/forge/github) and Gitea (planned).
type IssueTracker interface {
	// Issues
	CreateIssue(ctx context.Context, req *CreateIssueRequest) (*Issue, error)
	GetIssue(ctx context.Context, number int) (*Issue, error)
	UpdateIssue(ctx context.Context, number int, req *UpdateIssueRequest) (*Issue, error)
	ListIssues(ctx context.Context, opts *ListOptions) ([]*Issue, error)

	// Comments
	AddComment(ctx context.Context, number int, body string) (*Comment, error)
	ListComments(ctx context.Context, number int) ([]*Comment, error)

	// Labels
	SetLabels(ctx context.Context, number int, labels []string) error
	AddLabel(ctx context.Context, number int, label string) error
	RemoveLabel(ctx context.Context, number int, label string) error

	// Assignment
	Assign(ctx context.Context, number int, assignee string) error
	Unassign(ctx context.Context, number int, assignee string) error

	// Event watching (polling-based initially; webhook support planned via issue #8)
	Watch(ctx context.Context, handler func(Event)) error
}

// Issue represents a forge issue in Samverk's domain.
type Issue struct {
	Number        int
	Title         string
	Body          string
	State         State
	Labels        []string
	Assignees     []string
	IsPullRequest bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ClosedAt      *time.Time
}

// Comment represents a single comment on an issue.
type Comment struct {
	ID        int64
	Body      string
	Author    string
	CreatedAt time.Time
}

// Event represents a change detected on the forge.
type Event struct {
	Type          EventType
	IssueNumber   int
	Issue         *Issue
	Comment       *Comment
	Label         string
	Assignee      string
	IsPullRequest bool
}

// CreateIssueRequest contains the fields needed to create a new issue.
type CreateIssueRequest struct {
	Title     string
	Body      string
	Labels    []string
	Assignees []string
}

// UpdateIssueRequest contains the fields to update on an existing issue.
// Nil pointer fields are left unchanged.
type UpdateIssueRequest struct {
	Title *string
	Body  *string
	State *State
}

// ListOptions controls filtering and pagination for issue listing.
type ListOptions struct {
	State    State
	Labels   []string
	Assignee string
	Page     int
	PerPage  int
}

// MergeMethod specifies how a pull request should be merged.
type MergeMethod string

const (
	MergeMethodMerge  MergeMethod = "merge"
	MergeMethodSquash MergeMethod = "squash"
	MergeMethodRebase MergeMethod = "rebase"
)

// PullRequestManager provides pull request operations on a git forge.
type PullRequestManager interface {
	CreatePullRequest(ctx context.Context, req *CreatePRRequest) (*PullRequest, error)
	GetPullRequest(ctx context.Context, number int) (*PullRequest, error)
	ListPullRequests(ctx context.Context, opts *ListPROptions) ([]*PullRequest, error)
	MergePullRequest(ctx context.Context, number int, method MergeMethod, commitMsg string) error
}

// PullRequest represents a pull request in Samverk's domain.
type PullRequest struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     State     `json:"state"`
	Head      string    `json:"head"`
	Base      string    `json:"base"`
	Mergeable bool      `json:"mergeable"`
	Draft     bool      `json:"draft"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreatePRRequest contains the fields needed to create a pull request.
type CreatePRRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Head  string `json:"head"`
	Base  string `json:"base"`
}

// ListPROptions controls filtering for pull request listing.
type ListPROptions struct {
	State   State  `json:"state,omitempty"`
	Head    string `json:"head,omitempty"`
	Base    string `json:"base,omitempty"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}
