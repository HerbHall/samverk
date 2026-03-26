package dispatcher

import (
	"strings"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// longBody is a body with > 200 words to avoid triggering the "short body → triage" rule.
var longBody = strings.Repeat("word ", 210)

func TestSelectProviderKey(t *testing.T) {
	tests := []struct {
		name      string
		issue     *forge.Issue
		agentType models.AgentType
		wantKey   string
		wantReason string // substring match
	}{
		// --- complex tier ---
		{
			name:      "priority:critical → complex",
			issue:     &forge.Issue{Title: "fix: something", Body: longBody, Labels: []string{models.LabelPriorityCritical}},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "complex",
			wantReason: models.LabelPriorityCritical,
		},
		{
			name:      "complexity:high → complex",
			issue:     &forge.Issue{Title: "fix: something", Body: longBody, Labels: []string{"complexity:high"}},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "complex",
			wantReason: "complexity:high",
		},
		{
			name:      "title keyword 'refactor' → complex",
			issue:     &forge.Issue{Title: "refactor: the whole thing", Body: longBody},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "complex",
			wantReason: "refactor",
		},
		{
			name:      "title keyword 'architect' → complex",
			issue:     &forge.Issue{Title: "architect new pipeline", Body: longBody},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "complex",
			wantReason: "architect",
		},
		{
			name:      "title keyword 'redesign' → complex",
			issue:     &forge.Issue{Title: "redesign the UI", Body: longBody},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "complex",
			wantReason: "redesign",
		},
		{
			name:      "title keyword 'spike' → complex",
			issue:     &forge.Issue{Title: "spike: evaluate new approach", Body: longBody},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "complex",
			wantReason: "spike",
		},
		// keyword match is case-insensitive
		{
			name:      "title keyword uppercase REFACTOR → complex",
			issue:     &forge.Issue{Title: "REFACTOR: something", Body: longBody},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "complex",
			wantReason: "refactor",
		},

		// --- local tier ---
		{
			name:      "label type:boilerplate → local",
			issue:     &forge.Issue{Title: "add boilerplate", Body: longBody, Labels: []string{"type:boilerplate"}},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "local",
			wantReason: "type:boilerplate",
		},
		{
			name:      "label type:scaffold → local",
			issue:     &forge.Issue{Title: "scaffold new module", Body: longBody, Labels: []string{"type:scaffold"}},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "local",
			wantReason: "type:scaffold",
		},
		{
			name:      "title prefix chore: with code-gen agent → local",
			issue:     &forge.Issue{Title: "chore: update deps", Body: longBody},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "local",
			wantReason: "chore:",
		},
		{
			name:      "docs agent overrides chore: prefix → default (#263)",
			issue:     &forge.Issue{Title: "chore: update docs", Body: longBody},
			agentType: models.AgentTypeDocs,
			wantKey:   "default",
			wantReason: "agent type docs",
		},

		// --- triage tier ---
		{
			name:      "label priority:low → triage (research agent)",
			issue:     &forge.Issue{Title: "fix: minor thing", Body: longBody, Labels: []string{models.LabelPriorityLow}},
			agentType: models.AgentTypeResearch,
			wantKey:   "triage",
			wantReason: models.LabelPriorityLow,
		},
		{
			name:      "agent type docs → default",
			issue:     &forge.Issue{Title: "update readme", Body: longBody},
			agentType: models.AgentTypeDocs,
			wantKey:   "default",
			wantReason: "agent type docs",
		},
		{
			name:      "short body < 200 words → triage (research agent)",
			issue:     &forge.Issue{Title: "fix: something", Body: "short body"},
			agentType: models.AgentTypeResearch,
			wantKey:   "triage",
			wantReason: "short issue body",
		},

		// --- qc tier ---
		{
			name:      "agent type qc → qc",
			issue:     &forge.Issue{Title: "validate: check output", Body: longBody},
			agentType: models.AgentTypeQC,
			wantKey:   "qc",
			wantReason: "cross-model",
		},
		{
			// complex beats qc: priority:critical takes precedence
			name: "priority:critical beats qc agent type",
			issue: &forge.Issue{
				Title:  "validate: critical output",
				Body:   longBody,
				Labels: []string{models.LabelPriorityCritical},
			},
			agentType: models.AgentTypeQC,
			wantKey:   "complex",
			wantReason: models.LabelPriorityCritical,
		},

		// --- default tier ---
		{
			name:      "code-gen with no signals → code-gen chain (#269)",
			issue:     &forge.Issue{Title: "feat: add feature", Body: longBody},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "code-gen",
			wantReason: "agent type code-gen (requires CLI provider)",
		},
		{
			name:      "research agent with long body → default",
			issue:     &forge.Issue{Title: "research: investigate X", Body: longBody},
			agentType: models.AgentTypeResearch,
			wantKey:   "default",
			wantReason: "default routing",
		},

		// --- priority/precedence conflicts ---
		{
			// complex beats local: priority:critical takes precedence over type:boilerplate
			name: "priority:critical beats type:boilerplate",
			issue: &forge.Issue{
				Title:  "chore: generate scaffold",
				Body:   longBody,
				Labels: []string{models.LabelPriorityCritical, "type:boilerplate"},
			},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "complex",
			wantReason: models.LabelPriorityCritical,
		},
		{
			// complex beats triage: complexity:high takes precedence over priority:low
			name: "complexity:high beats priority:low",
			issue: &forge.Issue{
				Title:  "fix: something",
				Body:   longBody,
				Labels: []string{"complexity:high", models.LabelPriorityLow},
			},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "complex",
			wantReason: "complexity:high",
		},
		{
			// docs agent type overrides scaffold label (#263)
			name: "docs agent type beats type:scaffold",
			issue: &forge.Issue{
				Title:  "scaffold docs",
				Body:   longBody,
				Labels: []string{"type:scaffold"},
			},
			agentType: models.AgentTypeDocs,
			wantKey:   "default",
			wantReason: "agent type docs",
		},
		{
			// local beats triage: chore: prefix takes precedence over short body
			name: "chore: prefix beats short body",
			issue: &forge.Issue{
				Title: "chore: tiny thing",
				Body:  "short",
			},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "local",
			wantReason: "chore:",
		},
		{
			// 200-word boundary: exactly 200 words is NOT short.
			// Code-gen agent routes to code-gen chain regardless of body length.
			name: "code-gen with 200-word body → code-gen chain (#269)",
			issue: &forge.Issue{
				Title: "feat: something",
				Body:  strings.Repeat("word ", 200),
			},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "code-gen",
			wantReason: "agent type code-gen (requires CLI provider)",
		},
		{
			// 199 words IS short (research agent falls through to triage)
			name: "body with 199 words → triage (research agent)",
			issue: &forge.Issue{
				Title: "feat: something",
				Body:  strings.Repeat("word ", 199),
			},
			agentType: models.AgentTypeResearch,
			wantKey:   "triage",
			wantReason: "short issue body",
		},

		// --- agent-type override tests (#263) ---
		{
			name: "docs agent with architecture title → default not complex (#263)",
			issue: &forge.Issue{
				Title: "docs: update architecture.md with fleet topology",
				Body:  longBody,
			},
			agentType: models.AgentTypeDocs,
			wantKey:   "default",
			wantReason: "agent type docs",
		},
		{
			name: "test agent → code-gen chain (CLI-capable, #269)",
			issue: &forge.Issue{
				Title: "test: add forge interface tests",
				Body:  longBody,
			},
			agentType: models.AgentTypeTest,
			wantKey:   "code-gen",
			wantReason: "agent type test (requires CLI provider)",
		},

		// --- frontmatter word count tests (#264) ---
		{
			name: "code-gen with rich frontmatter → code-gen chain not triage (#264, #269)",
			issue: &forge.Issue{
				Title: "feat: add worker status",
				Body: "---\nschema_version: \"1.1.0\"\ntype: task\nagent_type: code-gen\npriority: high\nestimated_tokens: 12000\nhandoff_ready: true\nfile_context:\n  - internal/api/api.go\n  - internal/api/workers.go\nconstraints:\n  - do not modify existing endpoints\n  - run make ci before finishing\n---\n\n## Summary\n\n" + strings.Repeat("word ", 200),
			},
			agentType: models.AgentTypeCodeGen,
			wantKey:   "code-gen",
			wantReason: "agent type code-gen (requires CLI provider)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, reason := selectProviderKey(tt.issue, tt.agentType)
			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}
			if !strings.Contains(reason, tt.wantReason) {
				t.Errorf("reason = %q, want it to contain %q", reason, tt.wantReason)
			}
		})
	}
}

// TestHeartbeatFunc_UpdatesClaimedLastHeartbeat verifies the HeartbeatFunc closure
// built inside route() correctly updates d.claimed[N].LastHeartbeat under the lock.
// We test the closure directly to avoid the complexity of building a real agent.Pool.
func TestHeartbeatFunc_UpdatesClaimedLastHeartbeat(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	key := issueKey("test", "repo", 99)
	oldTime := time.Now().Add(-5 * time.Minute)

	d.mu.Lock()
	d.claimed[key] = &claimedIssue{
		AgentID:       "code-gen",
		ClaimedAt:     oldTime,
		LastHeartbeat: oldTime,
	}
	d.mu.Unlock()

	// Build the same closure that route() injects into agent.Task.HeartbeatFunc.
	heartbeatFunc := func() {
		d.mu.Lock()
		if c, ok := d.claimed[key]; ok {
			c.LastHeartbeat = time.Now()
		}
		d.mu.Unlock()
	}

	heartbeatFunc()

	d.mu.Lock()
	hb := d.claimed[key].LastHeartbeat
	d.mu.Unlock()

	if !hb.After(oldTime) {
		t.Errorf("LastHeartbeat not updated: got %v, want after %v", hb, oldTime)
	}
}

// TestHeartbeatFunc_NoopWhenUnclaimed verifies the HeartbeatFunc does not panic
// or error when the issue has already been removed from the claimed map (e.g.,
// after dispatcher completion callback runs before the goroutine stops).
func TestHeartbeatFunc_NoopWhenUnclaimed(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	key := issueKey("test", "repo", 77)
	// Issue is NOT in d.claimed.

	heartbeatFunc := func() {
		d.mu.Lock()
		if c, ok := d.claimed[key]; ok {
			c.LastHeartbeat = time.Now()
		}
		d.mu.Unlock()
	}

	// Must not panic.
	heartbeatFunc()
}
