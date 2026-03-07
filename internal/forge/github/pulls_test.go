package github

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	gogithub "github.com/google/go-github/v68/github"

	"github.com/herbhall/samverk/internal/forge"
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
