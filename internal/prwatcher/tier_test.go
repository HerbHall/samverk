package prwatcher

import (
	"testing"

	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/pkg/models"
)

func TestClassifyPRTier(t *testing.T) {
	tests := []struct {
		name        string
		pr          *forge.PullRequest
		issueLabels []string
		want        PRTier
	}{
		{
			name: "tier 3: priority critical label",
			pr:   &forge.PullRequest{Title: "feat: add widget"},
			issueLabels: []string{models.LabelPriorityCritical},
			want: PRTier3,
		},
		{
			name: "tier 3: complexity high label",
			pr:   &forge.PullRequest{Title: "refactor: rework core"},
			issueLabels: []string{"complexity:high"},
			want: PRTier3,
		},
		{
			name: "tier 3: type architecture label",
			pr:   &forge.PullRequest{Title: "feat: new module"},
			issueLabels: []string{"type:architecture"},
			want: PRTier3,
		},
		{
			name: "tier 3: breaking in title",
			pr:   &forge.PullRequest{Title: "breaking: remove old API"},
			want: PRTier3,
		},
		{
			name: "tier 3: security in title",
			pr:   &forge.PullRequest{Title: "fix: security vulnerability in auth"},
			want: PRTier3,
		},
		{
			name: "tier 3: deploy in title",
			pr:   &forge.PullRequest{Title: "chore: deploy to production"},
			want: PRTier3,
		},
		{
			name: "tier 1: agent docs label",
			pr:   &forge.PullRequest{Title: "update readme"},
			issueLabels: []string{models.LabelAgentDocs},
			want: PRTier1,
		},
		{
			name: "tier 1: type chore label",
			pr:   &forge.PullRequest{Title: "bump version"},
			issueLabels: []string{"type:chore"},
			want: PRTier1,
		},
		{
			name: "tier 1: autorelease pending label",
			pr:   &forge.PullRequest{
				Title:  "chore(main): release 1.0.0",
				Labels: []string{"autorelease: pending"},
			},
			want: PRTier1,
		},
		{
			name: "tier 1: docs prefix",
			pr:   &forge.PullRequest{Title: "docs: update README"},
			want: PRTier1,
		},
		{
			name: "tier 1: chore prefix",
			pr:   &forge.PullRequest{Title: "chore: update deps"},
			want: PRTier1,
		},
		{
			name: "tier 1: ci prefix",
			pr:   &forge.PullRequest{Title: "ci: add lint job"},
			want: PRTier1,
		},
		{
			name: "tier 1: style prefix",
			pr:   &forge.PullRequest{Title: "style: format code"},
			want: PRTier1,
		},
		{
			name: "tier 2: feat prefix",
			pr:   &forge.PullRequest{Title: "feat: add widget"},
			want: PRTier2,
		},
		{
			name: "tier 2: fix prefix",
			pr:   &forge.PullRequest{Title: "fix: null pointer"},
			want: PRTier2,
		},
		{
			name: "tier 2: refactor prefix",
			pr:   &forge.PullRequest{Title: "refactor: clean up handlers"},
			want: PRTier2,
		},
		{
			name: "tier 2: agent code-gen label",
			pr:   &forge.PullRequest{Title: "implement feature"},
			issueLabels: []string{models.LabelAgentCodeGen},
			want: PRTier2,
		},
		{
			name: "tier 2: agent test label",
			pr:   &forge.PullRequest{Title: "add tests"},
			issueLabels: []string{models.LabelAgentTest},
			want: PRTier2,
		},
		{
			name: "tier 2: feat( scoped prefix",
			pr:   &forge.PullRequest{Title: "feat(mcp): add new tool"},
			want: PRTier2,
		},
		{
			name: "tier 2: fix( scoped prefix",
			pr:   &forge.PullRequest{Title: "fix(auth): handle timeout"},
			want: PRTier2,
		},
		{
			name: "tier 2: default unknown title",
			pr:   &forge.PullRequest{Title: "some changes"},
			want: PRTier2,
		},
		{
			name: "tier 3 wins over tier 1",
			pr:   &forge.PullRequest{Title: "docs: security advisory"},
			want: PRTier3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyPRTier(tt.pr, tt.issueLabels)
			if got != tt.want {
				t.Errorf("ClassifyPRTier() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPRTierString(t *testing.T) {
	tests := []struct {
		tier PRTier
		want string
	}{
		{PRTier1, "tier-1"},
		{PRTier2, "tier-2"},
		{PRTier3, "tier-3"},
		{PRTierUnknown, "unknown"},
	}
	for _, tt := range tests {
		if got := tt.tier.String(); got != tt.want {
			t.Errorf("PRTier(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

func TestPRTierDescription(t *testing.T) {
	if d := PRTier1.Description(); d == "" {
		t.Error("PRTier1.Description() should not be empty")
	}
	if d := PRTier2.Description(); d == "" {
		t.Error("PRTier2.Description() should not be empty")
	}
	if d := PRTier3.Description(); d == "" {
		t.Error("PRTier3.Description() should not be empty")
	}
}
