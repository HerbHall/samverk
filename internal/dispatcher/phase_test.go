package dispatcher

import (
	"context"
	"strings"
	"testing"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// frontmatterBody builds a minimal issue body with the given agent_type in frontmatter.
func frontmatterBody(agentType string) string {
	return "---\nschema_version: \"1.1.0\"\ntype: task\nagent_type: " + agentType +
		"\npriority: normal\n---\n\n## Summary\n\n" + strings.Repeat("word ", 210) + "\n"
}

// TestPhaseGate_RouteSkipsBlockedAgentType verifies that route() leaves an issue
// unclaimed when the project phase does not allow the issue's agent type.
func TestPhaseGate_RouteSkipsBlockedAgentType(t *testing.T) {
	tracker := newMockTracker()
	issue := &forge.Issue{
		Number: 1,
		Title:  "fix: something",
		Body:   frontmatterBody("code-gen"),
		State:  forge.StateOpen,
		Labels: []string{"status:queued"},
	}
	tracker.issues[1] = issue

	d := newTestDispatcher(tracker)
	resolver := newMockProjectResolver()
	resolver.addProject("test", "repo", tracker)
	resolver.addPhase("test", "repo", "research") // code-gen not allowed in research
	d.projects = resolver

	err := d.route(context.Background(), "test", "repo", issue, models.AgentTypeCodeGen, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Issue must not be claimed — no "status:claimed" label added.
	d.mu.Lock()
	_, claimed := d.claimed[issueKey("test", "repo", 1)]
	d.mu.Unlock()
	if claimed {
		t.Error("expected issue to remain unclaimed when agent type is blocked by phase")
	}
}

// TestPhaseGate_RouteAllowsPermittedAgentType verifies that route() proceeds normally
// when the project phase allows the issue's agent type.
func TestPhaseGate_RouteAllowsPermittedAgentType(t *testing.T) {
	tracker := newMockTracker()
	issue := &forge.Issue{
		Number: 2,
		Title:  "research: competitive analysis",
		Body:   frontmatterBody("research"),
		State:  forge.StateOpen,
		Labels: []string{"status:queued"},
	}
	tracker.issues[2] = issue

	d := newTestDispatcher(tracker)
	resolver := newMockProjectResolver()
	resolver.addProject("test", "repo", tracker)
	resolver.addPhase("test", "repo", "research") // research allowed
	d.projects = resolver

	err := d.route(context.Background(), "test", "repo", issue, models.AgentTypeResearch, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Issue should be claimed.
	d.mu.Lock()
	_, claimed := d.claimed[issueKey("test", "repo", 2)]
	d.mu.Unlock()
	if !claimed {
		t.Error("expected issue to be claimed when agent type is allowed by phase")
	}
}

func TestPhaseAllowed(t *testing.T) {
	tests := []struct {
		name      string
		phase     string
		agentType models.AgentType
		want      bool
	}{
		// Empty phase: allow all.
		{"empty phase allows code-gen", "", models.AgentTypeCodeGen, true},
		{"empty phase allows research", "", models.AgentTypeResearch, true},

		// Unknown phase: allow all (defensive).
		{"unknown phase allows code-gen", "unknown", models.AgentTypeCodeGen, true},

		// research phase.
		{"research allows research", "research", models.AgentTypeResearch, true},
		{"research allows docs", "research", models.AgentTypeDocs, true},
		{"research blocks code-gen", "research", models.AgentTypeCodeGen, false},
		{"research blocks test", "research", models.AgentTypeTest, false},
		{"research blocks qc", "research", models.AgentTypeQC, false},

		// planning phase.
		{"planning allows research", "planning", models.AgentTypeResearch, true},
		{"planning allows docs", "planning", models.AgentTypeDocs, true},
		{"planning allows code-gen", "planning", models.AgentTypeCodeGen, true},
		{"planning blocks test", "planning", models.AgentTypeTest, false},
		{"planning blocks qc", "planning", models.AgentTypeQC, false},

		// design phase.
		{"design allows research", "design", models.AgentTypeResearch, true},
		{"design allows docs", "design", models.AgentTypeDocs, true},
		{"design allows code-gen", "design", models.AgentTypeCodeGen, true},
		{"design blocks test", "design", models.AgentTypeTest, false},

		// development phase — all known types allowed.
		{"development allows code-gen", "development", models.AgentTypeCodeGen, true},
		{"development allows test", "development", models.AgentTypeTest, true},
		{"development allows qc", "development", models.AgentTypeQC, true},
		{"development allows research", "development", models.AgentTypeResearch, true},
		{"development allows docs", "development", models.AgentTypeDocs, true},
		{"development allows orchestrator", "development", models.AgentTypeOrchestrator, true},

		// deployed phase — same as development.
		{"deployed allows code-gen", "deployed", models.AgentTypeCodeGen, true},
		{"deployed allows test", "deployed", models.AgentTypeTest, true},
		{"deployed allows qc", "deployed", models.AgentTypeQC, true},

		// maintenance phase — limited set.
		{"maintenance allows code-gen", "maintenance", models.AgentTypeCodeGen, true},
		{"maintenance allows test", "maintenance", models.AgentTypeTest, true},
		{"maintenance allows docs", "maintenance", models.AgentTypeDocs, true},
		{"maintenance allows qc", "maintenance", models.AgentTypeQC, true},
		{"maintenance blocks orchestrator", "maintenance", models.AgentTypeOrchestrator, false},
		{"maintenance blocks research", "maintenance", models.AgentTypeResearch, false},

		// inactive phase — nothing allowed.
		{"inactive blocks code-gen", "inactive", models.AgentTypeCodeGen, false},
		{"inactive blocks test", "inactive", models.AgentTypeTest, false},
		{"inactive blocks docs", "inactive", models.AgentTypeDocs, false},
		{"inactive blocks research", "inactive", models.AgentTypeResearch, false},
		{"inactive blocks qc", "inactive", models.AgentTypeQC, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := phaseAllowed(tc.phase, tc.agentType)
			if got != tc.want {
				t.Errorf("phaseAllowed(%q, %q) = %v, want %v", tc.phase, tc.agentType, got, tc.want)
			}
		})
	}
}
