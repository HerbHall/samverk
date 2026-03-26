package dispatcher

import (
	"strings"
	"testing"

	"github.com/herbhall/samverk/pkg/models"
)

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
