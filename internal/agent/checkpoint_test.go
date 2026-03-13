package agent

import (
	"testing"

	"github.com/herbhall/samverk/internal/forge"
)

func TestFormatCheckpoint(t *testing.T) {
	got := FormatCheckpoint("sess-1", "partial work here")
	want := "CHECKPOINT [sess-1]\n\n```\npartial work here\n```"
	if got != want {
		t.Errorf("FormatCheckpoint =\n%s\nwant:\n%s", got, want)
	}
}

func TestFindLatestCheckpoint(t *testing.T) {
	tests := []struct {
		name     string
		comments []*forge.Comment
		want     string
	}{
		{
			name:     "no comments",
			comments: nil,
			want:     "",
		},
		{
			name: "no checkpoint comments",
			comments: []*forge.Comment{
				{Body: "This is a regular comment."},
				{Body: "Agent error: timeout"},
			},
			want: "",
		},
		{
			name: "single checkpoint",
			comments: []*forge.Comment{
				{Body: "CHECKPOINT [sess-1]\n\n```\nEDIT path/to/file.go\ncontent here\n```"},
			},
			want: "EDIT path/to/file.go\ncontent here",
		},
		{
			name: "multiple checkpoints returns latest",
			comments: []*forge.Comment{
				{Body: "CHECKPOINT [sess-1]\n\n```\nold content\n```"},
				{Body: "some other comment"},
				{Body: "CHECKPOINT [sess-2]\n\n```\nnewer content\n```"},
			},
			want: "newer content",
		},
		{
			name: "malformed checkpoint missing code fence",
			comments: []*forge.Comment{
				{Body: "CHECKPOINT [sess-1]\n\nno code fence here"},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindLatestCheckpoint(tt.comments)
			if got != tt.want {
				t.Errorf("FindLatestCheckpoint = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHashCheckpoint(t *testing.T) {
	hash1 := HashCheckpoint("content A")
	hash2 := HashCheckpoint("content B")
	hash3 := HashCheckpoint("content A")

	if hash1 == hash2 {
		t.Error("different content should produce different hashes")
	}
	if hash1 != hash3 {
		t.Error("same content should produce same hash")
	}
	if len(hash1) != 64 {
		t.Errorf("hash length = %d, want 64 (hex-encoded SHA-256)", len(hash1))
	}
}

func TestDeduplicateEdits(t *testing.T) {
	tests := []struct {
		name       string
		newEdits   []FileEdit
		checkpoint string
		wantCount  int
		wantPaths  []string
	}{
		{
			name:       "empty checkpoint returns all edits",
			newEdits:   []FileEdit{{Path: "a.go", Content: "aaa"}},
			checkpoint: "",
			wantCount:  1,
		},
		{
			name: "identical edit is deduped",
			newEdits: []FileEdit{
				{Path: "a.go", Content: "aaa"},
			},
			checkpoint: "EDIT a.go\naaa\nEND",
			wantCount:  0,
		},
		{
			name: "different content same path is kept",
			newEdits: []FileEdit{
				{Path: "a.go", Content: "bbb"},
			},
			checkpoint: "EDIT a.go\naaa\nEND",
			wantCount:  1,
			wantPaths:  []string{"a.go"},
		},
		{
			name: "mixed: one deduped one kept",
			newEdits: []FileEdit{
				{Path: "a.go", Content: "aaa"},
				{Path: "b.go", Content: "bbb"},
			},
			checkpoint: "EDIT a.go\naaa\nEND",
			wantCount:  1,
			wantPaths:  []string{"b.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeduplicateEdits(tt.newEdits, tt.checkpoint)
			if len(got) != tt.wantCount {
				t.Errorf("DeduplicateEdits returned %d edits, want %d", len(got), tt.wantCount)
			}
			for i, wantPath := range tt.wantPaths {
				if i < len(got) && got[i].Path != wantPath {
					t.Errorf("edit[%d].Path = %q, want %q", i, got[i].Path, wantPath)
				}
			}
		})
	}
}

func TestFormatProgress(t *testing.T) {
	got := FormatProgress("sess-1", "partial work")
	if !contains(got, "PROGRESS [sess-1]") {
		t.Error("expected PROGRESS prefix with session ID")
	}
	if !contains(got, "```\npartial work\n```") {
		t.Error("expected partial output in code fence")
	}
}

func TestFindLatestProgress(t *testing.T) {
	tests := []struct {
		name     string
		comments []*forge.Comment
		want     string
	}{
		{
			name:     "no comments",
			comments: nil,
			want:     "",
		},
		{
			name: "no progress comments",
			comments: []*forge.Comment{
				{Body: "regular comment"},
				{Body: "CHECKPOINT [sess-1]\n\n```\nold\n```"},
			},
			want: "",
		},
		{
			name: "single progress",
			comments: []*forge.Comment{
				{Body: "PROGRESS [sess-1] [2026-03-13T10:00:00Z]\n\n```\nwork done\n```"},
			},
			want: "work done",
		},
		{
			name: "multiple progress returns latest",
			comments: []*forge.Comment{
				{Body: "PROGRESS [sess-1] [2026-03-13T10:00:00Z]\n\n```\nold progress\n```"},
				{Body: "some other comment"},
				{Body: "PROGRESS [sess-1] [2026-03-13T10:30:00Z]\n\n```\nnew progress\n```"},
			},
			want: "new progress",
		},
		{
			name: "mixed checkpoint and progress",
			comments: []*forge.Comment{
				{Body: "CHECKPOINT [sess-1]\n\n```\ncheckpoint content\n```"},
				{Body: "PROGRESS [sess-1] [2026-03-13T10:30:00Z]\n\n```\nprogress content\n```"},
			},
			want: "progress content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindLatestProgress(tt.comments)
			if got != tt.want {
				t.Errorf("FindLatestProgress = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildResumePrompt(t *testing.T) {
	got := BuildResumePrompt("some prior work")
	if got == "" {
		t.Fatal("BuildResumePrompt returned empty string")
	}
	// Verify it contains the prior work wrapped in XML tags.
	if !contains(got, "<prior-work>") || !contains(got, "</prior-work>") {
		t.Error("expected <prior-work> tags in resume prompt")
	}
	if !contains(got, "some prior work") {
		t.Error("expected checkpoint content in resume prompt")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
