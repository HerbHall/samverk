package models

import (
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantFM  *IssueFrontmatter
		wantErr bool
		wantBody string
	}{
		{
			name: "valid frontmatter with all fields",
			body: "---\nschema_version: \"1.0.0\"\ntype: task\nagent_type: code-gen\npriority: normal\nparent_issue: 123\ndepends_on: [121, 122]\nestimated_tokens: 2000\nactual_tokens: 1800\nmodel_used: claude-opus-4\n---\n\n## Summary\n\nOne sentence description.\n",
			wantFM: &IssueFrontmatter{
				SchemaVersion:   "1.0.0",
				Type:            IssueTypeTask,
				AgentType:       AgentTypeCodeGen,
				Priority:        PriorityNormal,
				ParentIssue:     123,
				DependsOn:       []int{121, 122},
				EstimatedTokens: 2000,
				ActualTokens:    1800,
				ModelUsed:       "claude-opus-4",
			},
			wantBody: "## Summary\n\nOne sentence description.\n",
		},
		{
			name: "minimal required fields",
			body: "---\nschema_version: \"1.0.0\"\ntype: question\nagent_type: human\npriority: high\n---\n\nPlain body text.\n",
			wantFM: &IssueFrontmatter{
				SchemaVersion: "1.0.0",
				Type:          IssueTypeQuestion,
				AgentType:     AgentTypeHuman,
				Priority:      PriorityHigh,
			},
			wantBody: "Plain body text.\n",
		},
		{
			name:     "no frontmatter returns nil",
			body:     "## Summary\n\nJust a plain issue body with no frontmatter.\n",
			wantFM:   nil,
			wantBody: "## Summary\n\nJust a plain issue body with no frontmatter.\n",
		},
		{
			name:    "malformed YAML returns error",
			body:    "---\nschema_version: \"1.0.0\"\n  bad indent: [unmatched\n---\n\nbody\n",
			wantErr: true,
		},
		{
			name:    "missing closing delimiter returns error",
			body:    "---\nschema_version: \"1.0.0\"\ntype: task\n",
			wantErr: true,
		},
		{
			name: "empty frontmatter block",
			body: "---\n---\n\nBody after empty frontmatter.\n",
			wantFM: &IssueFrontmatter{},
			wantBody: "Body after empty frontmatter.\n",
		},
		{
			name: "depends_on array parsing",
			body: "---\nschema_version: \"1.0.0\"\ntype: task\nagent_type: code-gen\npriority: normal\ndepends_on:\n  - 10\n  - 20\n  - 30\n---\n\nTask with dependencies.\n",
			wantFM: &IssueFrontmatter{
				SchemaVersion: "1.0.0",
				Type:          IssueTypeTask,
				AgentType:     AgentTypeCodeGen,
				Priority:      PriorityNormal,
				DependsOn:     []int{10, 20, 30},
			},
			wantBody: "Task with dependencies.\n",
		},
		{
			name: "timeout_minutes parsing",
			body: "---\nschema_version: \"1.0.0\"\ntype: task\nagent_type: code-gen\npriority: normal\ntimeout_minutes: 30\n---\n\nTask with timeout.\n",
			wantFM: &IssueFrontmatter{
				SchemaVersion:  "1.0.0",
				Type:           IssueTypeTask,
				AgentType:      AgentTypeCodeGen,
				Priority:       PriorityNormal,
				TimeoutMinutes: 30,
			},
			wantBody: "Task with timeout.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseFrontmatter(tt.body)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantFM == nil {
				if result.Frontmatter != nil {
					t.Fatalf("expected nil Frontmatter, got %+v", result.Frontmatter)
				}
			} else {
				if result.Frontmatter == nil {
					t.Fatal("expected non-nil Frontmatter, got nil")
				}
				assertFrontmatterEqual(t, tt.wantFM, result.Frontmatter)
			}
			if result.Body != tt.wantBody {
				t.Errorf("Body mismatch:\n  got:  %q\n  want: %q", result.Body, tt.wantBody)
			}
		})
	}
}

func TestRenderFrontmatterRoundTrip(t *testing.T) {
	original := &IssueFrontmatter{
		SchemaVersion:   SchemaVersion,
		Type:            IssueTypeTask,
		AgentType:       AgentTypeCodeGen,
		Priority:        PriorityNormal,
		ParentIssue:     42,
		DependsOn:       []int{10, 20},
		EstimatedTokens: 5000,
	}
	markdown := "## Summary\n\nRound-trip test body.\n"

	rendered := RenderFrontmatter(original, markdown)
	result, err := ParseFrontmatter(rendered)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	if result.Frontmatter == nil {
		t.Fatal("round-trip: expected non-nil Frontmatter")
	}

	assertFrontmatterEqual(t, original, result.Frontmatter)

	if result.Body != markdown {
		t.Errorf("round-trip body mismatch:\n  got:  %q\n  want: %q", result.Body, markdown)
	}
}

func assertFrontmatterEqual(t *testing.T, want, got *IssueFrontmatter) {
	t.Helper()
	if got.SchemaVersion != want.SchemaVersion {
		t.Errorf("SchemaVersion: got %q, want %q", got.SchemaVersion, want.SchemaVersion)
	}
	if got.Type != want.Type {
		t.Errorf("Type: got %q, want %q", got.Type, want.Type)
	}
	if got.AgentType != want.AgentType {
		t.Errorf("AgentType: got %q, want %q", got.AgentType, want.AgentType)
	}
	if got.Priority != want.Priority {
		t.Errorf("Priority: got %q, want %q", got.Priority, want.Priority)
	}
	if got.ParentIssue != want.ParentIssue {
		t.Errorf("ParentIssue: got %d, want %d", got.ParentIssue, want.ParentIssue)
	}
	if len(got.DependsOn) != len(want.DependsOn) {
		t.Errorf("DependsOn length: got %d, want %d", len(got.DependsOn), len(want.DependsOn))
	} else {
		for i := range want.DependsOn {
			if got.DependsOn[i] != want.DependsOn[i] {
				t.Errorf("DependsOn[%d]: got %d, want %d", i, got.DependsOn[i], want.DependsOn[i])
			}
		}
	}
	if got.EstimatedTokens != want.EstimatedTokens {
		t.Errorf("EstimatedTokens: got %d, want %d", got.EstimatedTokens, want.EstimatedTokens)
	}
	if got.ActualTokens != want.ActualTokens {
		t.Errorf("ActualTokens: got %d, want %d", got.ActualTokens, want.ActualTokens)
	}
	if got.ModelUsed != want.ModelUsed {
		t.Errorf("ModelUsed: got %q, want %q", got.ModelUsed, want.ModelUsed)
	}
	if got.TimeoutMinutes != want.TimeoutMinutes {
		t.Errorf("TimeoutMinutes: got %d, want %d", got.TimeoutMinutes, want.TimeoutMinutes)
	}
}
