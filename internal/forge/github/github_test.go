package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gogithub "github.com/google/go-github/v68/github"

	"github.com/herbhall/samverk/internal/forge"
)

// newTestClient creates a Client pointed at a test HTTP server.
func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := New("owner", "repo", nil)
	c.SetBaseURL(srv.URL + "/")

	return c, srv
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

func TestCreateIssue(t *testing.T) {
	var gotBody gogithub.IssueRequest

	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/owner/repo/issues", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeJSON(t, w, http.StatusCreated, &gogithub.Issue{
			Number:    gogithub.Ptr(42),
			Title:     gotBody.Title,
			Body:      gotBody.Body,
			State:     gogithub.Ptr("open"),
			CreatedAt: &gogithub.Timestamp{Time: time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)},
			UpdatedAt: &gogithub.Timestamp{Time: time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)},
		})
	})

	c, _ := newTestClient(t, mux)

	issue, err := c.CreateIssue(context.Background(), &forge.CreateIssueRequest{
		Title:  "Test issue",
		Body:   "Test body",
		Labels: []string{"bug"},
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
	if gotBody.Labels == nil || len(*gotBody.Labels) != 1 || (*gotBody.Labels)[0] != "bug" {
		t.Errorf("request labels = %v, want [bug]", gotBody.Labels)
	}
}

func TestGetIssue(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/issues/1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.Issue{
			Number: gogithub.Ptr(1),
			Title:  gogithub.Ptr("Found issue"),
			Body:   gogithub.Ptr("Body text"),
			State:  gogithub.Ptr("open"),
			Labels: []*gogithub.Label{
				{Name: gogithub.Ptr("priority:high")},
				{Name: gogithub.Ptr("type:task")},
			},
			Assignees: []*gogithub.User{
				{Login: gogithub.Ptr("agent-1")},
			},
			CreatedAt: &gogithub.Timestamp{Time: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
			UpdatedAt: &gogithub.Timestamp{Time: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
		})
	})

	c, _ := newTestClient(t, mux)

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

func TestGetIssue_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/issues/999", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, &gogithub.ErrorResponse{
			Message: "Not Found",
		})
	})

	c, _ := newTestClient(t, mux)

	_, err := c.GetIssue(context.Background(), 999)
	if err == nil {
		t.Fatal("GetIssue(999): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "get issue #999") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "get issue #999")
	}
}

func TestUpdateIssue(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /repos/owner/repo/issues/5", func(w http.ResponseWriter, r *http.Request) {
		var body gogithub.IssueRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}

		title := "Updated"
		if body.Title != nil {
			title = *body.Title
		}

		writeJSON(t, w, http.StatusOK, &gogithub.Issue{
			Number:    gogithub.Ptr(5),
			Title:     gogithub.Ptr(title),
			State:     body.State,
			CreatedAt: &gogithub.Timestamp{Time: time.Now()},
			UpdatedAt: &gogithub.Timestamp{Time: time.Now()},
		})
	})

	c, _ := newTestClient(t, mux)

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
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/issues", func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		if state != "open" {
			t.Errorf("state param = %q, want %q", state, "open")
		}

		writeJSON(t, w, http.StatusOK, []*gogithub.Issue{
			{
				Number:    gogithub.Ptr(1),
				Title:     gogithub.Ptr("First"),
				State:     gogithub.Ptr("open"),
				CreatedAt: &gogithub.Timestamp{Time: time.Now()},
				UpdatedAt: &gogithub.Timestamp{Time: time.Now()},
			},
			{
				Number:    gogithub.Ptr(2),
				Title:     gogithub.Ptr("Second"),
				State:     gogithub.Ptr("open"),
				CreatedAt: &gogithub.Timestamp{Time: time.Now()},
				UpdatedAt: &gogithub.Timestamp{Time: time.Now()},
			},
		})
	})

	c, _ := newTestClient(t, mux)

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
	var gotBody gogithub.IssueComment

	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/owner/repo/issues/10/comments", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode: %v", err)
		}

		writeJSON(t, w, http.StatusCreated, &gogithub.IssueComment{
			ID:        gogithub.Ptr(int64(100)),
			Body:      gotBody.Body,
			User:      &gogithub.User{Login: gogithub.Ptr("samverk-bot")},
			CreatedAt: &gogithub.Timestamp{Time: time.Now()},
		})
	})

	c, _ := newTestClient(t, mux)

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
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/issues/3/comments", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogithub.IssueComment{
			{
				ID:        gogithub.Ptr(int64(1)),
				Body:      gogithub.Ptr("First comment"),
				User:      &gogithub.User{Login: gogithub.Ptr("user1")},
				CreatedAt: &gogithub.Timestamp{Time: time.Now()},
			},
			{
				ID:        gogithub.Ptr(int64(2)),
				Body:      gogithub.Ptr("Second comment"),
				User:      &gogithub.User{Login: gogithub.Ptr("user2")},
				CreatedAt: &gogithub.Timestamp{Time: time.Now()},
			},
		})
	})

	c, _ := newTestClient(t, mux)

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
	var gotLabels []string

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /repos/owner/repo/issues/7/labels", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotLabels); err != nil {
			t.Fatalf("decode: %v", err)
		}

		writeJSON(t, w, http.StatusOK, []*gogithub.Label{
			{Name: gogithub.Ptr("priority:high")},
			{Name: gogithub.Ptr("status:claimed")},
		})
	})

	c, _ := newTestClient(t, mux)

	err := c.SetLabels(context.Background(), 7, []string{"priority:high", "status:claimed"})
	if err != nil {
		t.Fatalf("SetLabels: %v", err)
	}

	if len(gotLabels) != 2 {
		t.Errorf("sent %d labels, want 2", len(gotLabels))
	}
}

func TestAddLabel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/owner/repo/issues/7/labels", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogithub.Label{
			{Name: gogithub.Ptr("new-label")},
		})
	})

	c, _ := newTestClient(t, mux)

	err := c.AddLabel(context.Background(), 7, "new-label")
	if err != nil {
		t.Fatalf("AddLabel: %v", err)
	}
}

func TestRemoveLabel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /repos/owner/repo/issues/7/labels/old-label", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	c, _ := newTestClient(t, mux)

	err := c.RemoveLabel(context.Background(), 7, "old-label")
	if err != nil {
		t.Fatalf("RemoveLabel: %v", err)
	}
}

func TestAssign(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/owner/repo/issues/3/assignees", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.Issue{
			Number:    gogithub.Ptr(3),
			Assignees: []*gogithub.User{{Login: gogithub.Ptr("agent-1")}},
		})
	})

	c, _ := newTestClient(t, mux)

	err := c.Assign(context.Background(), 3, "agent-1")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
}

func TestUnassign(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /repos/owner/repo/issues/3/assignees", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.Issue{
			Number: gogithub.Ptr(3),
		})
	})

	c, _ := newTestClient(t, mux)

	err := c.Unassign(context.Background(), 3, "agent-1")
	if err != nil {
		t.Fatalf("Unassign: %v", err)
	}
}

func TestWatch_DetectsNewAndClosedIssues(t *testing.T) {
	callCount := 0

	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/issues", func(w http.ResponseWriter, _ *http.Request) {
		callCount++

		switch callCount {
		case 1:
			// Initial load: one issue.
			writeJSON(t, w, http.StatusOK, []*gogithub.Issue{
				{
					Number:    gogithub.Ptr(1),
					Title:     gogithub.Ptr("Existing"),
					State:     gogithub.Ptr("open"),
					CreatedAt: &gogithub.Timestamp{Time: time.Now()},
					UpdatedAt: &gogithub.Timestamp{Time: time.Now()},
				},
			})
		case 2:
			// Second poll: new issue #2 appeared, issue #1 gone (closed).
			writeJSON(t, w, http.StatusOK, []*gogithub.Issue{
				{
					Number:    gogithub.Ptr(2),
					Title:     gogithub.Ptr("New issue"),
					State:     gogithub.Ptr("open"),
					CreatedAt: &gogithub.Timestamp{Time: time.Now()},
					UpdatedAt: &gogithub.Timestamp{Time: time.Now()},
				},
			})
		default:
			writeJSON(t, w, http.StatusOK, []*gogithub.Issue{})
		}
	})

	c, _ := newTestClient(t, mux)
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

	if len(events) < 2 {
		t.Fatalf("got %d events, want at least 2 (opened + closed)", len(events))
	}

	// Should see: issue #2 opened, issue #1 closed (order may vary).
	var sawOpened, sawClosed bool
	for _, e := range events {
		switch {
		case e.Type == forge.EventIssueOpened && e.IssueNumber == 2:
			sawOpened = true
		case e.Type == forge.EventIssueClosed && e.IssueNumber == 1:
			sawClosed = true
		}
	}

	if !sawOpened {
		t.Error("missing EventIssueOpened for issue #2")
	}
	if !sawClosed {
		t.Error("missing EventIssueClosed for issue #1")
	}
}

func TestStringSliceEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", []string{}, []string{}, true},
		{"equal", []string{"a", "b"}, []string{"a", "b"}, true},
		{"different length", []string{"a"}, []string{"a", "b"}, false},
		{"different content", []string{"a", "b"}, []string{"a", "c"}, false},
		{"different order", []string{"a", "b"}, []string{"b", "a"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringSliceEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("stringSliceEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
