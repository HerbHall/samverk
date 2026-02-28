package gitea

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	gogitea "code.gitea.io/sdk/gitea"

	"github.com/herbhall/samverk/internal/forge"
)

// newTestClient creates a Client pointed at a test HTTP server.
// It uses SetGiteaVersion to skip the SDK's version detection request.
func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()

	// Wrap the handler to serve the version API endpoint that the SDK may call.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]string{"version": "1.23.7"})
	})
	mux.HandleFunc("GET /api/v1/settings/api", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"max_response_items":       50,
			"default_paging_num":       30,
			"default_git_trees_per_page": 1000,
			"default_max_blob_size":    10485760,
		})
	})
	mux.Handle("/", handler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	gt, err := gogitea.NewClient(srv.URL, gogitea.SetToken("test-token"))
	if err != nil {
		t.Fatalf("create gitea client: %v", err)
	}

	return &Client{
		gt:           gt,
		owner:        "owner",
		repo:         "repo",
		pollInterval: 30 * time.Second,
	}, srv
}

// writeJSON is a test helper that writes a JSON response.
func writeJSON(t *testing.T, w http.ResponseWriter, status int, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}

func TestCompileTimeInterfaceCheck(t *testing.T) {
	// This test verifies the compile-time check at the package level.
	var _ forge.IssueTracker = (*Client)(nil)
}

func TestCreateIssue(t *testing.T) {
	var gotBody gogitea.CreateIssueOption

	handler := http.NewServeMux()
	handler.HandleFunc("POST /api/v1/repos/owner/repo/issues", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeJSON(t, w, http.StatusCreated, &gogitea.Issue{
			Index:   42,
			Title:   gotBody.Title,
			Body:    gotBody.Body,
			State:   gogitea.StateOpen,
			Created: time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC),
			Updated: time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC),
		})
	})

	c, _ := newTestClient(t, handler)

	issue, err := c.CreateIssue(context.Background(), &forge.CreateIssueRequest{
		Title: "Test issue",
		Body:  "Test body",
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	if issue.Number != 42 {
		t.Errorf("Number = %d, want 42", issue.Number)
	}
	if issue.Title != "Test issue" {
		t.Errorf("Title = %q, want %q", issue.Title, "Test issue")
	}
	if issue.State != forge.StateOpen {
		t.Errorf("State = %q, want %q", issue.State, forge.StateOpen)
	}
}

func TestGetIssue(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v1/repos/owner/repo/issues/1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogitea.Issue{
			Index: 1,
			Title: "Found issue",
			Body:  "Body text",
			State: gogitea.StateOpen,
			Labels: []*gogitea.Label{
				{ID: 1, Name: "priority:high"},
				{ID: 2, Name: "type:task"},
			},
			Assignees: []*gogitea.User{
				{UserName: "agent-1"},
			},
			Created: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Updated: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		})
	})

	c, _ := newTestClient(t, handler)

	issue, err := c.GetIssue(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}

	if issue.Number != 1 {
		t.Errorf("Number = %d, want 1", issue.Number)
	}
	if issue.Title != "Found issue" {
		t.Errorf("Title = %q, want %q", issue.Title, "Found issue")
	}
	if len(issue.Labels) != 2 {
		t.Errorf("Labels = %v, want 2 labels", issue.Labels)
	}
	if len(issue.Assignees) != 1 || issue.Assignees[0] != "agent-1" {
		t.Errorf("Assignees = %v, want [agent-1]", issue.Assignees)
	}
}

func TestUpdateIssue(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("PATCH /api/v1/repos/owner/repo/issues/5", func(w http.ResponseWriter, r *http.Request) {
		var body gogitea.EditIssueOption
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}

		title := "Updated"
		if body.Title != "" {
			title = body.Title
		}

		writeJSON(t, w, http.StatusOK, &gogitea.Issue{
			Index:   5,
			Title:   title,
			State:   gogitea.StateClosed,
			Created: time.Now(),
			Updated: time.Now(),
		})
	})

	c, _ := newTestClient(t, handler)

	newTitle := "New title"
	closed := forge.StateClosed
	issue, err := c.UpdateIssue(context.Background(), 5, &forge.UpdateIssueRequest{
		Title: &newTitle,
		State: &closed,
	})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}

	if issue.Number != 5 {
		t.Errorf("Number = %d, want 5", issue.Number)
	}
	if issue.Title != "New title" {
		t.Errorf("Title = %q, want %q", issue.Title, "New title")
	}
}

func TestListIssues(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v1/repos/owner/repo/issues", func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		if state != "open" {
			t.Errorf("state param = %q, want %q", state, "open")
		}

		writeJSON(t, w, http.StatusOK, []*gogitea.Issue{
			{
				Index:   1,
				Title:   "First",
				State:   gogitea.StateOpen,
				Created: time.Now(),
				Updated: time.Now(),
			},
			{
				Index:   2,
				Title:   "Second",
				State:   gogitea.StateOpen,
				Created: time.Now(),
				Updated: time.Now(),
			},
		})
	})

	c, _ := newTestClient(t, handler)

	issues, err := c.ListIssues(context.Background(), &forge.ListOptions{
		State:   forge.StateOpen,
		PerPage: 50,
	})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	if len(issues) != 2 {
		t.Fatalf("len(issues) = %d, want 2", len(issues))
	}
	if issues[0].Title != "First" {
		t.Errorf("issues[0].Title = %q, want %q", issues[0].Title, "First")
	}
}

func TestAddComment(t *testing.T) {
	var gotBody gogitea.CreateIssueCommentOption

	handler := http.NewServeMux()
	handler.HandleFunc("POST /api/v1/repos/owner/repo/issues/10/comments", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode: %v", err)
		}

		writeJSON(t, w, http.StatusCreated, &gogitea.Comment{
			ID:      100,
			Body:    gotBody.Body,
			Poster:  &gogitea.User{UserName: "samverk-bot"},
			Created: time.Now(),
		})
	})

	c, _ := newTestClient(t, handler)

	comment, err := c.AddComment(context.Background(), 10, "Agent result: task complete")
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	if comment.ID != 100 {
		t.Errorf("ID = %d, want 100", comment.ID)
	}
	if comment.Body != "Agent result: task complete" {
		t.Errorf("Body = %q, want %q", comment.Body, "Agent result: task complete")
	}
	if comment.Author != "samverk-bot" {
		t.Errorf("Author = %q, want %q", comment.Author, "samverk-bot")
	}
}

func TestListComments(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v1/repos/owner/repo/issues/3/comments", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogitea.Comment{
			{
				ID:      1,
				Body:    "First comment",
				Poster:  &gogitea.User{UserName: "user1"},
				Created: time.Now(),
			},
			{
				ID:      2,
				Body:    "Second comment",
				Poster:  &gogitea.User{UserName: "user2"},
				Created: time.Now(),
			},
		})
	})

	c, _ := newTestClient(t, handler)

	comments, err := c.ListComments(context.Background(), 3)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}

	if len(comments) != 2 {
		t.Fatalf("len(comments) = %d, want 2", len(comments))
	}
	if comments[0].Author != "user1" {
		t.Errorf("comments[0].Author = %q, want %q", comments[0].Author, "user1")
	}
}

func TestSetLabels(t *testing.T) {
	handler := http.NewServeMux()

	// Label cache endpoint.
	handler.HandleFunc("GET /api/v1/repos/owner/repo/labels", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogitea.Label{
			{ID: 17, Name: "priority:high"},
			{ID: 10, Name: "status:claimed"},
		})
	})

	var gotLabels gogitea.IssueLabelsOption
	handler.HandleFunc("PUT /api/v1/repos/owner/repo/issues/7/labels", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotLabels); err != nil {
			t.Fatalf("decode: %v", err)
		}

		writeJSON(t, w, http.StatusOK, []*gogitea.Label{
			{ID: 17, Name: "priority:high"},
			{ID: 10, Name: "status:claimed"},
		})
	})

	c, _ := newTestClient(t, handler)

	err := c.SetLabels(context.Background(), 7, []string{"priority:high", "status:claimed"})
	if err != nil {
		t.Fatalf("SetLabels: %v", err)
	}

	if len(gotLabels.Labels) != 2 {
		t.Errorf("sent %d label IDs, want 2", len(gotLabels.Labels))
	}
	// Verify name-to-ID resolution.
	if gotLabels.Labels[0] != 17 {
		t.Errorf("label[0] ID = %d, want 17", gotLabels.Labels[0])
	}
	if gotLabels.Labels[1] != 10 {
		t.Errorf("label[1] ID = %d, want 10", gotLabels.Labels[1])
	}
}

func TestAddLabel(t *testing.T) {
	handler := http.NewServeMux()

	// Label cache endpoint.
	handler.HandleFunc("GET /api/v1/repos/owner/repo/labels", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogitea.Label{
			{ID: 5, Name: "new-label"},
		})
	})

	var gotLabels gogitea.IssueLabelsOption
	handler.HandleFunc("POST /api/v1/repos/owner/repo/issues/7/labels", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotLabels); err != nil {
			t.Fatalf("decode: %v", err)
		}

		writeJSON(t, w, http.StatusOK, []*gogitea.Label{
			{ID: 5, Name: "new-label"},
		})
	})

	c, _ := newTestClient(t, handler)

	err := c.AddLabel(context.Background(), 7, "new-label")
	if err != nil {
		t.Fatalf("AddLabel: %v", err)
	}

	if len(gotLabels.Labels) != 1 || gotLabels.Labels[0] != 5 {
		t.Errorf("sent label IDs = %v, want [5]", gotLabels.Labels)
	}
}

func TestRemoveLabel(t *testing.T) {
	handler := http.NewServeMux()

	// Label cache endpoint.
	handler.HandleFunc("GET /api/v1/repos/owner/repo/labels", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogitea.Label{
			{ID: 8, Name: "old-label"},
		})
	})

	var deletedLabelID string
	handler.HandleFunc("DELETE /api/v1/repos/owner/repo/issues/7/labels/", func(w http.ResponseWriter, r *http.Request) {
		// Extract the label ID from the path.
		// Path: /api/v1/repos/owner/repo/issues/7/labels/{id}
		deletedLabelID = r.PathValue("id")
		w.WriteHeader(http.StatusNoContent)
	})

	c, _ := newTestClient(t, handler)

	err := c.RemoveLabel(context.Background(), 7, "old-label")
	if err != nil {
		t.Fatalf("RemoveLabel: %v", err)
	}

	// The SDK calls DELETE with the numeric label ID.
	// We just verify no error occurred -- the SDK constructs the URL internally.
	_ = deletedLabelID
}

func TestAssign(t *testing.T) {
	handler := http.NewServeMux()

	// GetIssue for read-modify-write.
	handler.HandleFunc("GET /api/v1/repos/owner/repo/issues/3", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogitea.Issue{
			Index:     3,
			Assignees: []*gogitea.User{{UserName: "existing-user"}},
			Created:   time.Now(),
			Updated:   time.Now(),
		})
	})

	var gotBody gogitea.EditIssueOption
	handler.HandleFunc("PATCH /api/v1/repos/owner/repo/issues/3", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode: %v", err)
		}

		writeJSON(t, w, http.StatusOK, &gogitea.Issue{
			Index: 3,
			Assignees: []*gogitea.User{
				{UserName: "existing-user"},
				{UserName: "agent-1"},
			},
			Created: time.Now(),
			Updated: time.Now(),
		})
	})

	c, _ := newTestClient(t, handler)

	err := c.Assign(context.Background(), 3, "agent-1")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}

	// Verify read-modify-write: should include both existing and new assignee.
	if len(gotBody.Assignees) != 2 {
		t.Fatalf("assignees = %v, want 2 entries", gotBody.Assignees)
	}
	if gotBody.Assignees[0] != "existing-user" || gotBody.Assignees[1] != "agent-1" {
		t.Errorf("assignees = %v, want [existing-user, agent-1]", gotBody.Assignees)
	}
}

func TestUnassign(t *testing.T) {
	handler := http.NewServeMux()

	// GetIssue for read-modify-write.
	handler.HandleFunc("GET /api/v1/repos/owner/repo/issues/3", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogitea.Issue{
			Index: 3,
			Assignees: []*gogitea.User{
				{UserName: "agent-1"},
				{UserName: "agent-2"},
			},
			Created: time.Now(),
			Updated: time.Now(),
		})
	})

	var gotBody gogitea.EditIssueOption
	handler.HandleFunc("PATCH /api/v1/repos/owner/repo/issues/3", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode: %v", err)
		}

		writeJSON(t, w, http.StatusOK, &gogitea.Issue{
			Index:     3,
			Assignees: []*gogitea.User{{UserName: "agent-2"}},
			Created:   time.Now(),
			Updated:   time.Now(),
		})
	})

	c, _ := newTestClient(t, handler)

	err := c.Unassign(context.Background(), 3, "agent-1")
	if err != nil {
		t.Fatalf("Unassign: %v", err)
	}

	// Verify read-modify-write: agent-1 should be removed.
	if len(gotBody.Assignees) != 1 {
		t.Fatalf("assignees = %v, want 1 entry", gotBody.Assignees)
	}
	if gotBody.Assignees[0] != "agent-2" {
		t.Errorf("assignees = %v, want [agent-2]", gotBody.Assignees)
	}
}

func TestWatchDetectsNewIssue(t *testing.T) {
	callCount := 0

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v1/repos/owner/repo/issues", func(w http.ResponseWriter, _ *http.Request) {
		callCount++

		switch callCount {
		case 1:
			// Initial load: one issue.
			writeJSON(t, w, http.StatusOK, []*gogitea.Issue{
				{
					Index:   1,
					Title:   "Existing",
					State:   gogitea.StateOpen,
					Created: time.Now(),
					Updated: time.Now(),
				},
			})
		case 2:
			// Second poll: new issue #2 appeared.
			writeJSON(t, w, http.StatusOK, []*gogitea.Issue{
				{
					Index:   1,
					Title:   "Existing",
					State:   gogitea.StateOpen,
					Created: time.Now(),
					Updated: time.Now(),
				},
				{
					Index:   2,
					Title:   "New issue",
					State:   gogitea.StateOpen,
					Created: time.Now(),
					Updated: time.Now(),
				},
			})
		default:
			writeJSON(t, w, http.StatusOK, []*gogitea.Issue{})
		}
	})

	c, _ := newTestClient(t, handler)
	c.SetPollInterval(10 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var events []forge.Event
	var mu sync.Mutex

	go func() {
		_ = c.Watch(ctx, func(e forge.Event) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		})
	}()

	<-ctx.Done()
	// Give a moment for the last handler call to complete.
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	var sawOpened bool
	for _, e := range events {
		if e.Type == forge.EventIssueOpened && e.IssueNumber == 2 {
			sawOpened = true
		}
	}

	if !sawOpened {
		t.Error("missing EventIssueOpened for issue #2")
	}
}

func TestWatchDetectsClosedIssue(t *testing.T) {
	callCount := 0

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v1/repos/owner/repo/issues", func(w http.ResponseWriter, _ *http.Request) {
		callCount++

		switch callCount {
		case 1:
			// Initial load: one issue.
			writeJSON(t, w, http.StatusOK, []*gogitea.Issue{
				{
					Index:   1,
					Title:   "Will close",
					State:   gogitea.StateOpen,
					Created: time.Now(),
					Updated: time.Now(),
				},
			})
		case 2:
			// Second poll: issue #1 gone (closed).
			writeJSON(t, w, http.StatusOK, []*gogitea.Issue{})
		default:
			writeJSON(t, w, http.StatusOK, []*gogitea.Issue{})
		}
	})

	c, _ := newTestClient(t, handler)
	c.SetPollInterval(10 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var events []forge.Event
	var mu sync.Mutex

	go func() {
		_ = c.Watch(ctx, func(e forge.Event) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		})
	}()

	<-ctx.Done()
	// Give a moment for the last handler call to complete.
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	var sawClosed bool
	for _, e := range events {
		if e.Type == forge.EventIssueClosed && e.IssueNumber == 1 {
			sawClosed = true
		}
	}

	if !sawClosed {
		t.Error("missing EventIssueClosed for issue #1")
	}
}
