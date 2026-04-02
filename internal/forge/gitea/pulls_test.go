package gitea

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	gogitea "code.gitea.io/sdk/gitea"

	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/pkg/models"
)

func TestCreatePullRequest(t *testing.T) {
	var gotBody gogitea.CreatePullRequestOption

	handler := http.NewServeMux()
	handler.HandleFunc("POST /api/v1/repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		created := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
		writeJSON(t, w, http.StatusCreated, &gogitea.PullRequest{
			Index: 10,
			Title: gotBody.Title,
			Body:  gotBody.Body,
			State: gogitea.StateOpen,
			Head:  &gogitea.PRBranchInfo{Ref: gotBody.Head},
			Base:  &gogitea.PRBranchInfo{Ref: gotBody.Base},
			Poster: &gogitea.User{UserName: "alice"},
			Mergeable: true,
			Created:   &created,
			Updated:   &created,
		})
	})

	c, _ := newTestClient(t, handler)

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
	if pr.Body != "Test body" {
		t.Errorf("Body = %q, want %q", pr.Body, "Test body")
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
	if pr.Author != "alice" {
		t.Errorf("Author = %q, want %q", pr.Author, "alice")
	}
	if !pr.Mergeable {
		t.Error("Mergeable = false, want true")
	}
}

func TestGetPullRequest(t *testing.T) {
	created := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v1/repos/owner/repo/pulls/1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogitea.PullRequest{
			Index:     1,
			Title:     "Existing PR",
			Body:      "Some description",
			State:     gogitea.StateOpen,
			Head:      &gogitea.PRBranchInfo{Ref: "feature/x", Sha: "deadbeef"},
			Base:      &gogitea.PRBranchInfo{Ref: "main"},
			Poster:    &gogitea.User{UserName: "bob"},
			Mergeable: true,
			Draft:     false,
			Labels:    []*gogitea.Label{{ID: 1, Name: models.LabelPriorityHigh}},
			Created:   &created,
			Updated:   &created,
		})
	})

	c, _ := newTestClient(t, handler)

	pr, err := c.GetPullRequest(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetPullRequest: %v", err)
	}

	if pr.Number != 1 {
		t.Errorf("Number = %d, want 1", pr.Number)
	}
	if pr.Title != "Existing PR" {
		t.Errorf("Title = %q, want %q", pr.Title, "Existing PR")
	}
	if pr.Head != "feature/x" {
		t.Errorf("Head = %q, want %q", pr.Head, "feature/x")
	}
	if pr.Author != "bob" {
		t.Errorf("Author = %q, want %q", pr.Author, "bob")
	}
	if !pr.Mergeable {
		t.Error("Mergeable = false, want true")
	}
	if len(pr.Labels) != 1 || pr.Labels[0] != models.LabelPriorityHigh {
		t.Errorf("Labels = %v, want [priority:high]", pr.Labels)
	}
}

func TestListPullRequests(t *testing.T) {
	created := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v1/repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		if state != "open" {
			t.Errorf("state param = %q, want %q", state, "open")
		}

		writeJSON(t, w, http.StatusOK, []*gogitea.PullRequest{
			{
				Index:   1,
				Title:   "PR one",
				State:   gogitea.StateOpen,
				Head:    &gogitea.PRBranchInfo{Ref: "branch-1"},
				Base:    &gogitea.PRBranchInfo{Ref: "main"},
				Created: &created,
				Updated: &created,
			},
			{
				Index:   2,
				Title:   "PR two",
				State:   gogitea.StateOpen,
				Head:    &gogitea.PRBranchInfo{Ref: "branch-2"},
				Base:    &gogitea.PRBranchInfo{Ref: "main"},
				Created: &created,
				Updated: &created,
			},
		})
	})

	c, _ := newTestClient(t, handler)

	prs, err := c.ListPullRequests(context.Background(), &forge.ListPROptions{
		State: forge.StateOpen,
	})
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}

	if len(prs) != 2 {
		t.Fatalf("len(prs) = %d, want 2", len(prs))
	}
	if prs[0].Title != "PR one" {
		t.Errorf("prs[0].Title = %q, want %q", prs[0].Title, "PR one")
	}
	if prs[1].Title != "PR two" {
		t.Errorf("prs[1].Title = %q, want %q", prs[1].Title, "PR two")
	}
}

func TestMergePullRequest(t *testing.T) {
	var gotOpt gogitea.MergePullRequestOption

	handler := http.NewServeMux()
	handler.HandleFunc("POST /api/v1/repos/owner/repo/pulls/1/merge", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotOpt); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	})

	c, _ := newTestClient(t, handler)

	err := c.MergePullRequest(context.Background(), 1, forge.MergeMethodSquash, "squash commit message")
	if err != nil {
		t.Fatalf("MergePullRequest: %v", err)
	}

	if gotOpt.Style != gogitea.MergeStyleSquash {
		t.Errorf("Style = %q, want %q", gotOpt.Style, gogitea.MergeStyleSquash)
	}
}

func TestMergePullRequestRebase(t *testing.T) {
	var gotOpt gogitea.MergePullRequestOption

	handler := http.NewServeMux()
	handler.HandleFunc("POST /api/v1/repos/owner/repo/pulls/2/merge", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotOpt); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	})

	c, _ := newTestClient(t, handler)

	err := c.MergePullRequest(context.Background(), 2, forge.MergeMethodRebase, "rebase commit message")
	if err != nil {
		t.Fatalf("MergePullRequest: %v", err)
	}

	if gotOpt.Style != gogitea.MergeStyleRebase {
		t.Errorf("Style = %q, want %q", gotOpt.Style, gogitea.MergeStyleRebase)
	}
}

func TestGetPRChecks(t *testing.T) {
	created := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	handler := http.NewServeMux()

	// GetPullRequest to get head SHA.
	handler.HandleFunc("GET /api/v1/repos/owner/repo/pulls/1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogitea.PullRequest{
			Index: 1,
			Title: "PR with checks",
			State: gogitea.StateOpen,
			Head:  &gogitea.PRBranchInfo{Ref: "feature/checks", Sha: "abc123"},
			Base:  &gogitea.PRBranchInfo{Ref: "main"},
			Created: &created,
			Updated: &created,
		})
	})

	// GetCombinedStatus for head SHA.
	handler.HandleFunc("GET /api/v1/repos/owner/repo/commits/abc123/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogitea.CombinedStatus{
			State: gogitea.StatusSuccess,
			SHA:   "abc123",
			Statuses: []*gogitea.Status{
				{Context: "ci/build", State: gogitea.StatusSuccess},
				{Context: "ci/test", State: gogitea.StatusSuccess},
			},
		})
	})

	c, _ := newTestClient(t, handler)

	checks, err := c.GetPRChecks(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetPRChecks: %v", err)
	}

	if len(checks) != 2 {
		t.Fatalf("len(checks) = %d, want 2", len(checks))
	}
	byName := make(map[string]forge.CheckStatus, len(checks))
	for _, c := range checks {
		byName[c.Name] = c.Status
	}
	if byName["ci/build"] != forge.CheckStatusSuccess {
		t.Errorf("ci/build status = %q, want %q", byName["ci/build"], forge.CheckStatusSuccess)
	}
	if byName["ci/test"] != forge.CheckStatusSuccess {
		t.Errorf("ci/test status = %q, want %q", byName["ci/test"], forge.CheckStatusSuccess)
	}
}

func TestGetPRChecksEmptyStatuses(t *testing.T) {
	created := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v1/repos/owner/repo/pulls/2", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogitea.PullRequest{
			Index:   2,
			Title:   "PR with combined status",
			State:   gogitea.StateOpen,
			Head:    &gogitea.PRBranchInfo{Ref: "feature/y", Sha: "def456"},
			Base:    &gogitea.PRBranchInfo{Ref: "main"},
			Created: &created,
			Updated: &created,
		})
	})

	handler.HandleFunc("GET /api/v1/repos/owner/repo/commits/def456/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogitea.CombinedStatus{
			State:    gogitea.StatusPending,
			SHA:      "def456",
			Statuses: []*gogitea.Status{},
		})
	})

	c, _ := newTestClient(t, handler)

	checks, err := c.GetPRChecks(context.Background(), 2)
	if err != nil {
		t.Fatalf("GetPRChecks: %v", err)
	}

	if len(checks) != 1 {
		t.Fatalf("len(checks) = %d, want 1", len(checks))
	}
	if checks[0].Name != "combined" {
		t.Errorf("checks[0].Name = %q, want %q", checks[0].Name, "combined")
	}
	if checks[0].Status != forge.CheckStatusPending {
		t.Errorf("checks[0].Status = %q, want %q", checks[0].Status, forge.CheckStatusPending)
	}
}

func TestGetPRChecksEmptyHeadSHA(t *testing.T) {
	created := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v1/repos/owner/repo/pulls/3", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogitea.PullRequest{
			Index:   3,
			Title:   "PR with deleted branch",
			State:   gogitea.StateClosed,
			Head:    &gogitea.PRBranchInfo{Ref: "deleted-branch", Sha: ""},
			Base:    &gogitea.PRBranchInfo{Ref: "main"},
			Created: &created,
			Updated: &created,
		})
	})

	c, _ := newTestClient(t, handler)

	checks, err := c.GetPRChecks(context.Background(), 3)
	if err != nil {
		t.Fatalf("GetPRChecks: %v", err)
	}

	if checks != nil {
		t.Errorf("checks = %v, want nil", checks)
	}
}

func TestGetPRChecksDeduplicatesStaleStatuses(t *testing.T) {
	created := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	staleTime := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	freshTime := time.Date(2026, 3, 7, 13, 0, 0, 0, time.UTC) // 1 hour later

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v1/repos/owner/repo/pulls/4", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogitea.PullRequest{
			Index:   4,
			Title:   "PR with stale statuses",
			State:   gogitea.StateOpen,
			Head:    &gogitea.PRBranchInfo{Ref: "feature/dedup", Sha: "dedup123"},
			Base:    &gogitea.PRBranchInfo{Ref: "main"},
			Created: &created,
			Updated: &created,
		})
	})

	// Simulate Gitea returning both stale (failure) and fresh (success)
	// statuses for the same context -- the exact scenario from PR #368.
	handler.HandleFunc("GET /api/v1/repos/owner/repo/commits/dedup123/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogitea.CombinedStatus{
			State: gogitea.StatusSuccess,
			SHA:   "dedup123",
			Statuses: []*gogitea.Status{
				{Context: "CI / Lint", State: gogitea.StatusSuccess, Created: freshTime},
				{Context: "CI / Lint", State: gogitea.StatusFailure, Created: staleTime},
				{Context: "CI / Build", State: gogitea.StatusSuccess, Created: freshTime},
				{Context: "CI / Build", State: gogitea.StatusPending, Created: staleTime},
			},
		})
	})

	c, _ := newTestClient(t, handler)

	checks, err := c.GetPRChecks(context.Background(), 4)
	if err != nil {
		t.Fatalf("GetPRChecks: %v", err)
	}

	// Should deduplicate to 2 checks (one per context), both success.
	if len(checks) != 2 {
		t.Fatalf("len(checks) = %d, want 2 (deduped)", len(checks))
	}
	byName := make(map[string]forge.CheckStatus, len(checks))
	for _, c := range checks {
		byName[c.Name] = c.Status
	}
	if byName["CI / Lint"] != forge.CheckStatusSuccess {
		t.Errorf("CI / Lint status = %q, want %q (stale failure should be filtered)", byName["CI / Lint"], forge.CheckStatusSuccess)
	}
	if byName["CI / Build"] != forge.CheckStatusSuccess {
		t.Errorf("CI / Build status = %q, want %q (stale pending should be filtered)", byName["CI / Build"], forge.CheckStatusSuccess)
	}
}

func TestListReviewComments(t *testing.T) {
	commentTime := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	handler := http.NewServeMux()

	// ListPullReviews.
	handler.HandleFunc("GET /api/v1/repos/owner/repo/pulls/1/reviews", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogitea.PullReview{
			{ID: 1, State: gogitea.ReviewStateComment, Body: ""},
		})
	})

	// ListPullReviewComments for review 1.
	handler.HandleFunc("GET /api/v1/repos/owner/repo/pulls/1/reviews/1/comments", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogitea.PullReviewComment{
			{
				ID:         1,
				Body:       "This line needs fixing",
				Reviewer:   &gogitea.User{UserName: "reviewer"},
				Path:       "main.go",
				LineNum:    42,
				OldLineNum: 40,
				Created:    commentTime,
			},
			{
				ID:       2,
				Body:     "Missing error check",
				Reviewer: &gogitea.User{UserName: "reviewer"},
				Path:     "handler.go",
				LineNum:  15,
				Created:  commentTime,
			},
		})
	})

	c, _ := newTestClient(t, handler)

	comments, err := c.ListReviewComments(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListReviewComments: %v", err)
	}

	if len(comments) != 2 {
		t.Fatalf("len(comments) = %d, want 2", len(comments))
	}

	// First comment: has OldLineNum set — StartLine should differ from EndLine.
	if comments[0].Author != "reviewer" {
		t.Errorf("comments[0].Author = %q, want %q", comments[0].Author, "reviewer")
	}
	if comments[0].Body != "This line needs fixing" {
		t.Errorf("comments[0].Body = %q, want %q", comments[0].Body, "This line needs fixing")
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
	if comments[0].Resolved {
		t.Error("comments[0].Resolved = true, want false")
	}

	// Second comment: no OldLineNum — StartLine should default to EndLine.
	if comments[1].StartLine != 15 {
		t.Errorf("comments[1].StartLine = %d, want 15 (should default to EndLine)", comments[1].StartLine)
	}
	if comments[1].EndLine != 15 {
		t.Errorf("comments[1].EndLine = %d, want 15", comments[1].EndLine)
	}
}

func TestListReviewCommentsSkipsEmptyBody(t *testing.T) {
	handler := http.NewServeMux()
	commentTime := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	handler.HandleFunc("GET /api/v1/repos/owner/repo/pulls/1/reviews", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogitea.PullReview{
			{ID: 1, State: gogitea.ReviewStateComment, Body: ""},
		})
	})

	handler.HandleFunc("GET /api/v1/repos/owner/repo/pulls/1/reviews/1/comments", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogitea.PullReviewComment{
			{
				ID:       1,
				Body:     "",
				Reviewer: &gogitea.User{UserName: "reviewer"},
				Path:     "main.go",
				LineNum:  10,
				Created:  commentTime,
			},
			{
				ID:       2,
				Body:     "Valid comment",
				Reviewer: &gogitea.User{UserName: "reviewer"},
				Path:     "main.go",
				LineNum:  20,
				Created:  commentTime,
			},
		})
	})

	c, _ := newTestClient(t, handler)

	comments, err := c.ListReviewComments(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListReviewComments: %v", err)
	}

	if len(comments) != 1 {
		t.Fatalf("len(comments) = %d, want 1 (empty body skipped)", len(comments))
	}
	if comments[0].Body != "Valid comment" {
		t.Errorf("comments[0].Body = %q, want %q", comments[0].Body, "Valid comment")
	}
}

func TestListReviewCommentsResolved(t *testing.T) {
	commentTime := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v1/repos/owner/repo/pulls/1/reviews", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogitea.PullReview{
			{ID: 1, State: gogitea.ReviewStateComment, Body: ""},
		})
	})

	handler.HandleFunc("GET /api/v1/repos/owner/repo/pulls/1/reviews/1/comments", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogitea.PullReviewComment{
			{
				ID:       1,
				Body:     "Resolved comment",
				Reviewer: &gogitea.User{UserName: "reviewer"},
				Resolver: &gogitea.User{UserName: "author"},
				Path:     "main.go",
				LineNum:  5,
				Created:  commentTime,
			},
		})
	})

	c, _ := newTestClient(t, handler)

	comments, err := c.ListReviewComments(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListReviewComments: %v", err)
	}

	if len(comments) != 1 {
		t.Fatalf("len(comments) = %d, want 1", len(comments))
	}
	if !comments[0].Resolved {
		t.Error("comments[0].Resolved = false, want true")
	}
}
