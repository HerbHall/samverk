package dispatcher

import (
	"testing"
	"time"

	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/pkg/models"
)

func TestPriorityWeight(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   int
	}{
		{"critical", []string{models.LabelPriorityCritical}, 100},
		{"high", []string{models.LabelPriorityHigh}, 75},
		{"normal", []string{models.LabelPriorityNormal}, 50},
		{"low", []string{models.LabelPriorityLow}, 25},
		{"no priority label defaults to normal", []string{"feat"}, 50},
		{"multiple labels picks priority", []string{"feat", models.LabelPriorityHigh, "agent:code-gen"}, 75},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := priorityWeight(tt.labels)
			if got != tt.want {
				t.Errorf("priorityWeight(%v) = %d, want %d", tt.labels, got, tt.want)
			}
		})
	}
}

func TestAgeBonus(t *testing.T) {
	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		createdAt time.Time
		want      int
	}{
		{"brand new", now, 0},
		{"1 day old", now.Add(-24 * time.Hour), 5},
		{"3 days old", now.Add(-3 * 24 * time.Hour), 15},
		{"7 days old", now.Add(-7 * 24 * time.Hour), 35},
		{"10 days old (capped)", now.Add(-10 * 24 * time.Hour), 50},
		{"30 days old (still capped)", now.Add(-30 * 24 * time.Hour), 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ageBonus(tt.createdAt, now)
			if got != tt.want {
				t.Errorf("ageBonus(%v) = %d, want %d", tt.createdAt, got, tt.want)
			}
		})
	}
}

func TestSortByPriority(t *testing.T) {
	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)

	issues := []*forge.Issue{
		{Number: 1, Labels: []string{models.LabelPriorityNormal}, CreatedAt: now},                          // expected score 50
		{Number: 2, Labels: []string{models.LabelPriorityCritical}, CreatedAt: now},                       // expected score 100
		{Number: 3, Labels: []string{models.LabelPriorityLow}, CreatedAt: now.Add(-10 * 24 * time.Hour)}, // expected score 75
		{Number: 4, Labels: []string{models.LabelPriorityHigh}, CreatedAt: now.Add(-1 * 24 * time.Hour)}, // expected score 80
	}

	sortByPriority(issues, now)

	expectedOrder := []int{2, 4, 3, 1} // 100, 80, 75, 50
	for i, want := range expectedOrder {
		if issues[i].Number != want {
			t.Errorf("position %d: got issue #%d, want #%d", i, issues[i].Number, want)
		}
	}
}

func TestSortByPriority_StableForEqualScores(t *testing.T) {
	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)

	// Two issues with identical scores -- original order preserved.
	issues := []*forge.Issue{
		{Number: 10, Labels: []string{models.LabelPriorityNormal}, CreatedAt: now},
		{Number: 20, Labels: []string{models.LabelPriorityNormal}, CreatedAt: now},
	}

	sortByPriority(issues, now)

	if issues[0].Number != 10 || issues[1].Number != 20 {
		t.Errorf("stable sort broken: got [%d, %d], want [10, 20]", issues[0].Number, issues[1].Number)
	}
}

func TestSortByPriority_OldNormalMatchesFreshCritical(t *testing.T) {
	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)

	// 10-day-old normal: 50 + 50 = 100
	// fresh critical:    100 + 0 = 100
	oldNormal := &forge.Issue{Number: 1, Labels: []string{models.LabelPriorityNormal}, CreatedAt: now.Add(-10 * 24 * time.Hour)}
	freshCritical := &forge.Issue{Number: 2, Labels: []string{models.LabelPriorityCritical}, CreatedAt: now}

	scoreOld := issueScore(oldNormal, now)
	scoreFresh := issueScore(freshCritical, now)

	if scoreOld != scoreFresh {
		t.Errorf("10-day normal score (%d) should equal fresh critical score (%d)", scoreOld, scoreFresh)
	}
}

func TestShouldPromoteChain(t *testing.T) {
	threeDaysAgo := time.Now().Add(-4 * 24 * time.Hour) // > 3 days
	justNow := time.Now()

	tests := []struct {
		name        string
		chain       string
		createdAt   time.Time
		idleWorkers int
		wantChain   string
		wantOK      bool
	}{
		{
			name:        "local -> code-gen when old and idle",
			chain:       "local",
			createdAt:   threeDaysAgo,
			idleWorkers: 2,
			wantChain:   "code-gen",
			wantOK:      true,
		},
		{
			name:        "triage -> default when old and idle",
			chain:       "triage",
			createdAt:   threeDaysAgo,
			idleWorkers: 1,
			wantChain:   "default",
			wantOK:      true,
		},
		{
			name:        "no promotion when issue is new",
			chain:       "local",
			createdAt:   justNow,
			idleWorkers: 5,
			wantChain:   "local",
			wantOK:      false,
		},
		{
			name:        "no promotion when no idle workers",
			chain:       "local",
			createdAt:   threeDaysAgo,
			idleWorkers: 0,
			wantChain:   "local",
			wantOK:      false,
		},
		{
			name:        "no promotion for non-Ollama chains",
			chain:       "code-gen",
			createdAt:   threeDaysAgo,
			idleWorkers: 5,
			wantChain:   "code-gen",
			wantOK:      false,
		},
		{
			name:        "no promotion for complex chain",
			chain:       "complex",
			createdAt:   threeDaysAgo,
			idleWorkers: 5,
			wantChain:   "complex",
			wantOK:      false,
		},
		{
			name:        "no promotion for default chain",
			chain:       "default",
			createdAt:   threeDaysAgo,
			idleWorkers: 5,
			wantChain:   "default",
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := &forge.Issue{CreatedAt: tt.createdAt}
			gotChain, gotOK := shouldPromoteChain(tt.chain, issue, tt.idleWorkers)
			if gotChain != tt.wantChain || gotOK != tt.wantOK {
				t.Errorf("shouldPromoteChain(%q) = (%q, %v), want (%q, %v)",
					tt.chain, gotChain, gotOK, tt.wantChain, tt.wantOK)
			}
		})
	}
}

func TestIsOllamaChain(t *testing.T) {
	tests := []struct {
		chain string
		want  bool
	}{
		{"local", true},
		{"triage", true},
		{"code-gen", false},
		{"default", false},
		{"complex", false},
	}
	for _, tt := range tests {
		if got := isOllamaChain(tt.chain); got != tt.want {
			t.Errorf("isOllamaChain(%q) = %v, want %v", tt.chain, got, tt.want)
		}
	}
}
