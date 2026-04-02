package github

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	gogithub "github.com/google/go-github/v68/github"

	"samverk.dev/samverk/internal/forge"
)

func TestCreatePullRequest(t *testing.T) {
	var gotBody gogithub.NewPullRequest

	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeJSON(t, w, http.StatusCreated, &gogithub.PullRequest{
			Number:    gogithub.Ptr(10),
			Title:     gotBody.Title,
			Body:      gotBody.Body,
			State:     gogithub.Ptr("open"),
			Draft:     gogithub.Ptr(false),
			Mergeable: gogithub.Ptr(true),
			Head:      &gogithub.PullRequestBranch{Ref: gotBody.Head},
			Base:      &gogithub.PullRequestBranch{Ref: gotBody.Base},
			CreatedAt: &gogithub.Timestamp{Time: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)},
			UpdatedAt: &gogithub.Timestamp{Time: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)},
		})
	})

	c, _ := newTestClient(t, mux)

	pr, err := c.CreatePullRequest(context.Background(), &forge.CreatePRRequest{
		Title: "Test PR",
		Body:  "Test body",
		Head:  "feature/test",
		Base:  "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}

	if pr.Number != 10 {
		t.Errorf("Number = %d, want 10", pr.Number)
	}
	if pr.Title != "Test PR" {
		t.Errorf("Title = %q, want %q", pr.Title, "Test PR")
	}
	if pr.Head != "feature/test" {
		t.Errorf("Head = %q, want %q", pr.Head, "feature/test")
	}
	if pr.Base != "main" {
		t.Errorf("Base = %q, want %q", pr.Base, "main")
	}
	if pr.State != forge.StateOpen {
		t.Errorf("State = %q, want %q", pr.State, forge.StateOpen)
	}
}

func TestGetPullRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/pulls/10", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.PullRequest{
			Number:    gogithub.Ptr(10),
			Title:     gogithub.Ptr("Existing PR"),
			State:     gogithub.Ptr("open"),
			Mergeable: gogithub.Ptr(true),
			Head:      &gogithub.PullRequestBranch{Ref: gogithub.Ptr("feature/x")},
			Base:      &gogithub.PullRequestBranch{Ref: gogithub.Ptr("main")},
			CreatedAt: &gogithub.Timestamp{Time: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)},
			UpdatedAt: &gogithub.Timestamp{Time: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)},
		})
	})

	c, _ := newTestClient(t, mux)

	pr, err := c.GetPullRequest(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPullRequest: %v", err)
	}

	if pr.Number != 10 {
		t.Errorf("Number = %d, want 10", pr.Number)
	}
	if pr.Title != "Existing PR" {
		t.Errorf("Title = %q, want %q", pr.Title, "Existing PR")
	}
	if !pr.Mergeable {
		t.Error("Mergeable = false, want true")
	}
}

func TestListPullRequests(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/pulls", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogithub.PullRequest{
			{
				Number:    gogithub.Ptr(1),
				Title:     gogithub.Ptr("PR one"),
				State:     gogithub.Ptr("open"),
				Head:      &gogithub.PullRequestBranch{Ref: gogithub.Ptr("branch-1")},
				Base:      &gogithub.PullRequestBranch{Ref: gogithub.Ptr("main")},
				CreatedAt: &gogithub.Timestamp{Time: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)},
				UpdatedAt: &gogithub.Timestamp{Time: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)},
			},
			{
				Number:    gogithub.Ptr(2),
				Title:     gogithub.Ptr("PR two"),
				State:     gogithub.Ptr("open"),
				Head:      &gogithub.PullRequestBranch{Ref: gogithub.Ptr("branch-2")},
				Base:      &gogithub.PullRequestBranch{Ref: gogithub.Ptr("main")},
				CreatedAt: &gogithub.Timestamp{Time: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)},
				UpdatedAt: &gogithub.Timestamp{Time: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)},
			},
		})
	})

	c, _ := newTestClient(t, mux)

	prs, err := c.ListPullRequests(context.Background(), &forge.ListPROptions{State: forge.StateOpen})
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}

	if len(prs) != 2 {
		t.Fatalf("len(prs) = %d, want 2", len(prs))
	}
	if prs[0].Title != "PR one" {
		t.Errorf("prs[0].Title = %q, want %q", prs[0].Title, "PR one")
	}
}

func TestMergePullRequest(t *testing.T) {
	var gotMethod string

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /repos/owner/repo/pulls/10/merge", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotMethod, _ = body["merge_method"].(string)

		writeJSON(t, w, http.StatusOK, &gogithub.PullRequestMergeResult{
			Merged: gogithub.Ptr(true),
			SHA:    gogithub.Ptr("abc123"),
		})
	})

	c, _ := newTestClient(t, mux)

	err := c.MergePullRequest(context.Background(), 10, forge.MergeMethodSquash, "squash merge")
	if err != nil {
		t.Fatalf("MergePullRequest: %v", err)
	}

	if gotMethod != "squash" {
		t.Errorf("merge_method = %q, want %q", gotMethod, "squash")
	}
}

func TestGetPRChecks_CheckRunsOnly(t *testing.T) {
	headSHA := "abc123def456"

	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/pulls/10", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.PullRequest{
			Number: gogithub.Ptr(10),
			Head: &gogithub.PullRequestBranch{
				SHA: gogithub.Ptr(headSHA),
			},
		})
	})
	mux.HandleFunc("GET /repos/owner/repo/commits/"+headSHA+"/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.CombinedStatus{
			Statuses: []*gogithub.RepoStatus{}, // no legacy statuses
		})
	})
	mux.HandleFunc("GET /repos/owner/repo/commits/"+headSHA+"/check-runs", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.ListCheckRunsResults{
			Total: gogithub.Ptr(3),
			CheckRuns: []*gogithub.CheckRun{
				{Name: gogithub.Ptr("Build"), Status: gogithub.Ptr("completed"), Conclusion: gogithub.Ptr("success")},
				{Name: gogithub.Ptr("Test"), Status: gogithub.Ptr("completed"), Conclusion: gogithub.Ptr("success")},
				{Name: gogithub.Ptr("Lint"), Status: gogithub.Ptr("completed"), Conclusion: gogithub.Ptr("failure")},
			},
		})
	})

	c, _ := newTestClient(t, mux)
	checks, err := c.GetPRChecks(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPRChecks: %v", err)
	}

	if len(checks) != 3 {
		t.Fatalf("len(checks) = %d, want 3", len(checks))
	}

	byName := make(map[string]forge.CheckStatus, len(checks))
	for _, ch := range checks {
		byName[ch.Name] = ch.Status
	}

	if byName["Build"] != forge.CheckStatusSuccess {
		t.Errorf("Build = %q, want success", byName["Build"])
	}
	if byName["Test"] != forge.CheckStatusSuccess {
		t.Errorf("Test = %q, want success", byName["Test"])
	}
	if byName["Lint"] != forge.CheckStatusFailure {
		t.Errorf("Lint = %q, want failure", byName["Lint"])
	}
}

func TestGetPRChecks_MergesBothSources(t *testing.T) {
	headSHA := "abc123def456"

	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/pulls/10", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.PullRequest{
			Number: gogithub.Ptr(10),
			Head:   &gogithub.PullRequestBranch{SHA: gogithub.Ptr(headSHA)},
		})
	})
	mux.HandleFunc("GET /repos/owner/repo/commits/"+headSHA+"/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.CombinedStatus{
			Statuses: []*gogithub.RepoStatus{
				{Context: gogithub.Ptr("external-ci"), State: gogithub.Ptr("success")},
				{Context: gogithub.Ptr("Build"), State: gogithub.Ptr("pending")}, // will be overwritten by check run
			},
		})
	})
	mux.HandleFunc("GET /repos/owner/repo/commits/"+headSHA+"/check-runs", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.ListCheckRunsResults{
			Total: gogithub.Ptr(1),
			CheckRuns: []*gogithub.CheckRun{
				{Name: gogithub.Ptr("Build"), Status: gogithub.Ptr("completed"), Conclusion: gogithub.Ptr("success")},
			},
		})
	})

	c, _ := newTestClient(t, mux)
	checks, err := c.GetPRChecks(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPRChecks: %v", err)
	}

	if len(checks) != 2 {
		t.Fatalf("len(checks) = %d, want 2", len(checks))
	}

	byName := make(map[string]forge.CheckStatus, len(checks))
	for _, ch := range checks {
		byName[ch.Name] = ch.Status
	}

	// Check run should overwrite legacy status for "Build".
	if byName["Build"] != forge.CheckStatusSuccess {
		t.Errorf("Build = %q, want success (check run should overwrite legacy pending)", byName["Build"])
	}
	if byName["external-ci"] != forge.CheckStatusSuccess {
		t.Errorf("external-ci = %q, want success", byName["external-ci"])
	}
}

func TestGetPRChecks_PendingCheckRun(t *testing.T) {
	headSHA := "abc123def456"

	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/pulls/10", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.PullRequest{
			Number: gogithub.Ptr(10),
			Head:   &gogithub.PullRequestBranch{SHA: gogithub.Ptr(headSHA)},
		})
	})
	mux.HandleFunc("GET /repos/owner/repo/commits/"+headSHA+"/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.CombinedStatus{})
	})
	mux.HandleFunc("GET /repos/owner/repo/commits/"+headSHA+"/check-runs", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.ListCheckRunsResults{
			Total: gogithub.Ptr(1),
			CheckRuns: []*gogithub.CheckRun{
				{Name: gogithub.Ptr("Build"), Status: gogithub.Ptr("in_progress"), Conclusion: gogithub.Ptr("")},
			},
		})
	})

	c, _ := newTestClient(t, mux)
	checks, err := c.GetPRChecks(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPRChecks: %v", err)
	}

	if len(checks) != 1 {
		t.Fatalf("len(checks) = %d, want 1", len(checks))
	}
	if checks[0].Status != forge.CheckStatusPending {
		t.Errorf("Status = %q, want pending", checks[0].Status)
	}
}

func TestGetPRChecks_NoBothSources(t *testing.T) {
	headSHA := "abc123def456"

	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/pulls/10", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.PullRequest{
			Number: gogithub.Ptr(10),
			Head:   &gogithub.PullRequestBranch{SHA: gogithub.Ptr(headSHA)},
		})
	})
	mux.HandleFunc("GET /repos/owner/repo/commits/"+headSHA+"/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.CombinedStatus{})
	})
	mux.HandleFunc("GET /repos/owner/repo/commits/"+headSHA+"/check-runs", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.ListCheckRunsResults{Total: gogithub.Ptr(0)})
	})

	c, _ := newTestClient(t, mux)
	checks, err := c.GetPRChecks(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPRChecks: %v", err)
	}

	if len(checks) != 0 {
		t.Fatalf("len(checks) = %d, want 0", len(checks))
	}
}

func TestMapCheckRunStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		conclusion string
		want       forge.CheckStatus
	}{
		{"completed success", "completed", "success", forge.CheckStatusSuccess},
		{"completed failure", "completed", "failure", forge.CheckStatusFailure},
		{"completed neutral", "completed", "neutral", forge.CheckStatusSuccess},
		{"completed skipped", "completed", "skipped", forge.CheckStatusSuccess},
		{"completed cancelled", "completed", "cancelled", forge.CheckStatusFailure},
		{"completed timed_out", "completed", "timed_out", forge.CheckStatusFailure},
		{"completed action_required", "completed", "action_required", forge.CheckStatusFailure},
		{"in_progress", "in_progress", "", forge.CheckStatusPending},
		{"queued", "queued", "", forge.CheckStatusPending},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapCheckRunStatus(tt.status, tt.conclusion)
			if got != tt.want {
				t.Errorf("mapCheckRunStatus(%q, %q) = %q, want %q", tt.status, tt.conclusion, got, tt.want)
			}
		})
	}
}

func TestMapCommitStatus(t *testing.T) {
	tests := []struct {
		state string
		want  forge.CheckStatus
	}{
		{"success", forge.CheckStatusSuccess},
		{"failure", forge.CheckStatusFailure},
		{"error", forge.CheckStatusFailure},
		{"pending", forge.CheckStatusPending},
		{"unknown", forge.CheckStatusPending},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := mapCommitStatus(tt.state)
			if got != tt.want {
				t.Errorf("mapCommitStatus(%q) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestListReviewComments(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/pulls/10/comments", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogithub.PullRequestComment{
			{
				ID:        gogithub.Ptr(int64(1)),
				Body:      gogithub.Ptr("Use constants here"),
				Path:      gogithub.Ptr("main.go"),
				Line:      gogithub.Ptr(42),
				StartLine: gogithub.Ptr(40),
				User:      &gogithub.User{Login: gogithub.Ptr("copilot")},
				CreatedAt: &gogithub.Timestamp{Time: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)},
			},
			{
				ID:        gogithub.Ptr(int64(2)),
				Body:      gogithub.Ptr("Missing error check"),
				Path:      gogithub.Ptr("handler.go"),
				Line:      gogithub.Ptr(15),
				User:      &gogithub.User{Login: gogithub.Ptr("copilot")},
				CreatedAt: &gogithub.Timestamp{Time: time.Date(2026, 3, 7, 13, 0, 0, 0, time.UTC)},
			},
		})
	})

	c, _ := newTestClient(t, mux)

	comments, err := c.ListReviewComments(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListReviewComments: %v", err)
	}

	if len(comments) != 2 {
		t.Fatalf("len(comments) = %d, want 2", len(comments))
	}

	// First comment: has StartLine set.
	if comments[0].Author != "copilot" {
		t.Errorf("comments[0].Author = %q, want %q", comments[0].Author, "copilot")
	}
	if comments[0].Path != "main.go" {
		t.Errorf("comments[0].Path = %q, want %q", comments[0].Path, "main.go")
	}
	if comments[0].StartLine != 40 {
		t.Errorf("comments[0].StartLine = %d, want 40", comments[0].StartLine)
	}
	if comments[0].EndLine != 42 {
		t.Errorf("comments[0].EndLine = %d, want 42", comments[0].EndLine)
	}

	// Second comment: no StartLine — should default to EndLine.
	if comments[1].StartLine != 15 {
		t.Errorf("comments[1].StartLine = %d, want 15 (should default to EndLine)", comments[1].StartLine)
	}
	if comments[1].EndLine != 15 {
		t.Errorf("comments[1].EndLine = %d, want 15", comments[1].EndLine)
	}
}
