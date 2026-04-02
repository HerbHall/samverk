package dispatcher

import (
	"strings"
	"testing"

	"samverk.dev/samverk/pkg/models"
)

func TestExtractAcceptanceCriteria(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "no section returns empty",
			body: "Just a description with no criteria section.",
			want: []string{},
		},
		{
			name: "extracts unchecked items only",
			body: "## Acceptance Criteria\n- [ ] item one\n- [x] already done\n- [ ] item two\n",
			want: []string{"item one", "item two"},
		},
		{
			name: "stops at next heading",
			body: "## Acceptance Criteria\n- [ ] first\n## Other Section\n- [ ] should not appear\n",
			want: []string{"first"},
		},
		{
			name: "case-insensitive header match",
			body: "## acceptance criteria\n- [ ] lowercase header\n",
			want: []string{"lowercase header"},
		},
		{
			name: "empty section returns empty",
			body: "## Acceptance Criteria\n\nsome other text\n",
			want: []string{},
		},
		{
			name: "multiple criteria",
			body: "Some intro.\n\n## Acceptance Criteria\n\n- [ ] Criteria extracted from issue body\n- [ ] Keyword matching checks output\n- [ ] Coverage score computed\n",
			want: []string{"Criteria extracted from issue body", "Keyword matching checks output", "Coverage score computed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractAcceptanceCriteria(tt.body)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d criteria %v, want %d %v", len(got), got, len(tt.want), tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("criteria[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCriterionKeywords(t *testing.T) {
	tests := []struct {
		criterion string
		want      []string
	}{
		{
			criterion: "Acceptance criteria extracted from issue body",
			want:      []string{"acceptance", "criteria", "extracted", "from", "issue", "body"},
		},
		{
			criterion: "short",
			want:      []string{"short"},
		},
		{
			criterion: "a an the is",
			want:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.criterion, func(t *testing.T) {
			got := criterionKeywords(tt.criterion)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("keyword[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCheckCriteriaCoverage(t *testing.T) {
	tests := []struct {
		name          string
		criteria      []string
		output        string
		wantCovered   int
		wantMissing   []string
	}{
		{
			name:        "all covered",
			criteria:    []string{"extraction logic added", "keyword matching works"},
			output:      "I implemented extraction logic and keyword matching functionality.",
			wantCovered: 2,
			wantMissing: []string{},
		},
		{
			name:        "none covered",
			criteria:    []string{"implement widget rendering", "add database migration"},
			output:      "I reviewed the issue and found it interesting.",
			wantCovered: 0,
			wantMissing: []string{"implement widget rendering", "add database migration"},
		},
		{
			name:        "partial coverage",
			criteria:    []string{"extraction logic", "scoring computed", "tests added"},
			output:      "The extraction logic is implemented. Coverage scoring is computed correctly.",
			wantCovered: 2,
			wantMissing: []string{"tests added"},
		},
		{
			name:        "empty criteria returns zero covered",
			criteria:    []string{},
			output:      "some output",
			wantCovered: 0,
			wantMissing: []string{},
		},
		{
			name:        "case insensitive matching",
			criteria:    []string{"Acceptance Criteria Coverage"},
			output:      "The acceptance criteria coverage score is 80%.",
			wantCovered: 1,
			wantMissing: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCovered, gotMissing := checkCriteriaCoverage(tt.criteria, tt.output)
			if gotCovered != tt.wantCovered {
				t.Errorf("covered: got %d, want %d", gotCovered, tt.wantCovered)
			}
			if len(gotMissing) != len(tt.wantMissing) {
				t.Errorf("missing count: got %d %v, want %d %v", len(gotMissing), gotMissing, len(tt.wantMissing), tt.wantMissing)
				return
			}
			for i := range tt.wantMissing {
				if gotMissing[i] != tt.wantMissing[i] {
					t.Errorf("missing[%d]: got %q, want %q", i, gotMissing[i], tt.wantMissing[i])
				}
			}
		})
	}
}

func TestCoveragePctThreshold(t *testing.T) {
	// Verify the 50% threshold logic: 1/3 = 33% < 50%, 2/3 = 66% >= 50%.
	criteria := []string{"feature alpha implemented", "feature beta tested", "feature gamma documented"}
	lowOutput := "alpha was implemented here"
	highOutput := "alpha implemented and beta tested thoroughly"

	covLow, _ := checkCriteriaCoverage(criteria, lowOutput)
	pctLow := float64(covLow) / float64(len(criteria))
	if pctLow >= 0.5 {
		t.Errorf("expected low coverage < 50%%, got %.2f", pctLow)
	}

	covHigh, _ := checkCriteriaCoverage(criteria, highOutput)
	pctHigh := float64(covHigh) / float64(len(criteria))
	if pctHigh < 0.5 {
		t.Errorf("expected high coverage >= 50%%, got %.2f", pctHigh)
	}
}

func TestCheckOutputQuality(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		agentType    models.AgentType
		maxTurnsHit  bool
		turnsUsed    int
		wantPass     bool
		wantScore    float64
		wantReason   string
	}{
		{
			name:        "empty output fails",
			output:      "",
			agentType:   models.AgentTypeCodeGen,
			maxTurnsHit: false,
			turnsUsed:   0,
			wantPass:    false,
			wantScore:   0.0,
			wantReason:  "output too short",
		},
		{
			name:        "short output fails",
			output:      "Done.",
			agentType:   models.AgentTypeCodeGen,
			maxTurnsHit: false,
			turnsUsed:   0,
			wantPass:    false,
			wantScore:   0.0,
			wantReason:  "output too short",
		},
		{
			name:        "whitespace-only output fails",
			output:      "   \n\t\n   ",
			agentType:   models.AgentTypeResearch,
			maxTurnsHit: false,
			turnsUsed:   0,
			wantPass:    false,
			wantScore:   0.0,
			wantReason:  "output too short",
		},
		{
			name:        "natural continue phrase is NOT truncation (false positive fix)",
			output:      strings.Repeat("x", 100) + " I'll continue from where I left off next time.",
			agentType:   models.AgentTypeResearch,
			maxTurnsHit: false,
			turnsUsed:   0,
			wantPass:    true,
			wantScore:   1.0,
			wantReason:  "quality check passed",
		},
		{
			name:        "natural let me continue is NOT truncation (false positive fix)",
			output:      strings.Repeat("x", 100) + " Let me continue with the remaining items.",
			agentType:   models.AgentTypeDocs,
			maxTurnsHit: false,
			turnsUsed:   0,
			wantPass:    true,
			wantScore:   1.0,
			wantReason:  "quality check passed",
		},
		{
			name:        "truncated output with ellipsis marker",
			output:      strings.Repeat("x", 100) + " ...truncated due to length",
			agentType:   models.AgentTypeResearch,
			maxTurnsHit: false,
			turnsUsed:   0,
			wantPass:    false,
			wantScore:   0.3,
			wantReason:  "output appears truncated: ...truncated",
		},
		{
			name:        "truncated output with limit reached",
			output:      strings.Repeat("x", 100) + " output limit reached",
			agentType:   models.AgentTypeResearch,
			maxTurnsHit: false,
			turnsUsed:   0,
			wantPass:    false,
			wantScore:   0.3,
			wantReason:  "output appears truncated: output limit reached",
		},
		{
			name:        "truncated output with max turns message",
			output:      strings.Repeat("x", 100) + " reached the maximum number of turns",
			agentType:   models.AgentTypeCodeGen,
			maxTurnsHit: false,
			turnsUsed:   0,
			wantPass:    false,
			wantScore:   0.3,
			wantReason:  "output appears truncated: maximum number of turns",
		},
		{
			name:        "code-gen without code blocks fails",
			output:      "I analyzed the issue and determined the best approach is to refactor the module.",
			agentType:   models.AgentTypeCodeGen,
			maxTurnsHit: false,
			turnsUsed:   0,
			wantPass:    false,
			wantScore:   0.2,
			wantReason:  "code agent produced no code blocks",
		},
		{
			name:        "test agent without code blocks fails",
			output:      "The tests should cover edge cases including nil inputs and empty strings for validation.",
			agentType:   models.AgentTypeTest,
			maxTurnsHit: false,
			turnsUsed:   0,
			wantPass:    false,
			wantScore:   0.2,
			wantReason:  "code agent produced no code blocks",
		},
		{
			name:        "code-gen with backtick code blocks passes",
			output:      "Here is the implementation:\n```go\nfunc Hello() string { return \"hello\" }\n```\nThis handles the case.",
			agentType:   models.AgentTypeCodeGen,
			maxTurnsHit: false,
			turnsUsed:   0,
			wantPass:    true,
			wantScore:   1.0,
			wantReason:  "quality check passed",
		},
		{
			name:        "code-gen with EDIT block marker passes",
			output:      "Applied the following change:\nEDIT internal/server/handler.go\n- old line\n+ new line\nEND",
			agentType:   models.AgentTypeCodeGen,
			maxTurnsHit: false,
			turnsUsed:   0,
			wantPass:    true,
			wantScore:   1.0,
			wantReason:  "quality check passed",
		},
		{
			name:        "research agent with normal output passes",
			output:      "After reviewing the competitive landscape, the main alternatives are X, Y, and Z. Each has different trade-offs.",
			agentType:   models.AgentTypeResearch,
			maxTurnsHit: false,
			turnsUsed:   0,
			wantPass:    true,
			wantScore:   1.0,
			wantReason:  "quality check passed",
		},
		{
			name:        "docs agent with normal output passes",
			output:      "Updated the README with installation instructions, usage examples, and configuration reference documentation.",
			agentType:   models.AgentTypeDocs,
			maxTurnsHit: false,
			turnsUsed:   0,
			wantPass:    true,
			wantScore:   1.0,
			wantReason:  "quality check passed",
		},
		{
			name:        "non-code agent ignores missing code blocks",
			output:      "The infrastructure changes are deployed. Service is healthy and responding to health checks correctly.",
			agentType:   models.AgentTypeInfra,
			maxTurnsHit: false,
			turnsUsed:   0,
			wantPass:    true,
			wantScore:   1.0,
			wantReason:  "quality check passed",
		},
		{
			name:        "max turns hit with turns used",
			output:      strings.Repeat("x", 100) + " this is partial output",
			agentType:   models.AgentTypeCodeGen,
			maxTurnsHit: true,
			turnsUsed:   42,
			wantPass:    false,
			wantScore:   0.3,
			wantReason:  "output truncated: max turns hit (42 turns used)",
		},
		{
			name:        "max turns hit without turns used",
			output:      strings.Repeat("x", 100) + " this is partial output",
			agentType:   models.AgentTypeResearch,
			maxTurnsHit: true,
			turnsUsed:   0,
			wantPass:    false,
			wantScore:   0.3,
			wantReason:  "output truncated: max turns hit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkOutputQuality(tt.output, tt.agentType, tt.maxTurnsHit, tt.turnsUsed)
			if got.Pass != tt.wantPass {
				t.Errorf("Pass: got %v, want %v", got.Pass, tt.wantPass)
			}
			if got.Score != tt.wantScore {
				t.Errorf("Score: got %v, want %v", got.Score, tt.wantScore)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("Reason: got %q, want %q", got.Reason, tt.wantReason)
			}
		})
	}
}
