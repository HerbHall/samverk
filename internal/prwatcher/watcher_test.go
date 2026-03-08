package prwatcher

import (
	"testing"

	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/forge"
)

func TestIsEligible(t *testing.T) {
	baseCfg := autonomy.MergeConfig{
		AutoMergeOnCIPass: true,
		TrustedAuthors:    []string{"bot", "agent"},
		ExcludeLabels:     []string{"do-not-merge", "wip"},
	}

	tests := []struct {
		name string
		pr   *forge.PullRequest
		want bool
	}{
		{
			name: "eligible PR",
			pr: &forge.PullRequest{
				Number:    1,
				Author:    "bot",
				Draft:     false,
				Mergeable: true,
				Labels:    []string{"auto"},
			},
			want: true,
		},
		{
			name: "draft PR",
			pr: &forge.PullRequest{
				Number:    2,
				Author:    "bot",
				Draft:     true,
				Mergeable: true,
			},
			want: false,
		},
		{
			name: "not mergeable",
			pr: &forge.PullRequest{
				Number:    3,
				Author:    "bot",
				Draft:     false,
				Mergeable: false,
			},
			want: false,
		},
		{
			name: "untrusted author",
			pr: &forge.PullRequest{
				Number:    4,
				Author:    "stranger",
				Draft:     false,
				Mergeable: true,
			},
			want: false,
		},
		{
			name: "excluded label",
			pr: &forge.PullRequest{
				Number:    5,
				Author:    "agent",
				Draft:     false,
				Mergeable: true,
				Labels:    []string{"approved", "do-not-merge"},
			},
			want: false,
		},
	}

	w := &Watcher{mergeCfg: baseCfg}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := w.isEligible(tt.pr)
			if got != tt.want {
				t.Errorf("isEligible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTrustedAuthor_EmptyList(t *testing.T) {
	w := &Watcher{mergeCfg: autonomy.MergeConfig{TrustedAuthors: nil}}
	if !w.isTrustedAuthor("anyone") {
		t.Error("empty trusted list should trust all authors")
	}
}

func TestAllChecksPassed(t *testing.T) {
	w := &Watcher{}

	tests := []struct {
		name   string
		checks []forge.Check
		want   bool
	}{
		{
			name:   "empty checks - not passed",
			checks: nil,
			want:   false,
		},
		{
			name: "all success",
			checks: []forge.Check{
				{Name: "ci/build", Status: forge.CheckStatusSuccess},
				{Name: "ci/test", Status: forge.CheckStatusSuccess},
			},
			want: true,
		},
		{
			name: "one pending",
			checks: []forge.Check{
				{Name: "ci/build", Status: forge.CheckStatusSuccess},
				{Name: "ci/test", Status: forge.CheckStatusPending},
			},
			want: false,
		},
		{
			name: "one failure",
			checks: []forge.Check{
				{Name: "ci/build", Status: forge.CheckStatusFailure},
				{Name: "ci/test", Status: forge.CheckStatusSuccess},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := w.allChecksPassed(tt.checks)
			if got != tt.want {
				t.Errorf("allChecksPassed() = %v, want %v", got, tt.want)
			}
		})
	}
}
