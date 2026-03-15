package models

import (
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantFM   *IssueFrontmatter
		wantErr  bool
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
				DependsOn:       DependencyList{{Number: 121}, {Number: 122}},
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
			name:     "empty frontmatter block",
			body:     "---\n---\n\nBody after empty frontmatter.\n",
			wantFM:   &IssueFrontmatter{},
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
				DependsOn:     DependencyList{{Number: 10}, {Number: 20}, {Number: 30}},
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
		{
			name: "mixed depends_on with cross-project refs",
			body: "---\nschema_version: \"1.1.0\"\ntype: task\nagent_type: code-gen\npriority: normal\ndepends_on:\n  - 42\n  - \"HerbHall/devkit#15\"\n  - 43\n---\n\nMixed deps.\n",
			wantFM: &IssueFrontmatter{
				SchemaVersion: "1.1.0",
				Type:          IssueTypeTask,
				AgentType:     AgentTypeCodeGen,
				Priority:      PriorityNormal,
				DependsOn: DependencyList{
					{Number: 42},
					{Owner: "HerbHall", Repo: "devkit", Number: 15},
					{Number: 43},
				},
			},
			wantBody: "Mixed deps.\n",
		},
		{
			name: "coordination issue type with source_project",
			body: "---\nschema_version: \"1.1.0\"\ntype: coordination\nagent_type: code-gen\npriority: normal\nsource_project: HerbHall/samverk\ndepends_on:\n  - \"HerbHall/samverk#400\"\n---\n\nCoordination task.\n",
			wantFM: &IssueFrontmatter{
				SchemaVersion: "1.1.0",
				Type:          IssueTypeCoordination,
				AgentType:     AgentTypeCodeGen,
				Priority:      PriorityNormal,
				SourceProject: "HerbHall/samverk",
				DependsOn: DependencyList{
					{Owner: "HerbHall", Repo: "samverk", Number: 400},
				},
			},
			wantBody: "Coordination task.\n",
		},
		{
			name:    "invalid cross-project ref format",
			body:    "---\ndepends_on:\n  - \"badformat\"\n---\n\nbody\n",
			wantErr: true,
		},
		{
			name:    "cross-project ref with zero issue number",
			body:    "---\ndepends_on:\n  - \"owner/repo#0\"\n---\n\nbody\n",
			wantErr: true,
		},
		{
			name:    "cross-project ref with non-numeric issue number",
			body:    "---\ndepends_on:\n  - \"owner/repo#abc\"\n---\n\nbody\n",
			wantErr: true,
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
		DependsOn:       DependencyList{{Number: 10}, {Number: 20}},
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

func TestRenderFrontmatterRoundTrip_CrossProject(t *testing.T) {
	original := &IssueFrontmatter{
		SchemaVersion: SchemaVersion,
		Type:          IssueTypeCoordination,
		AgentType:     AgentTypeCodeGen,
		Priority:      PriorityNormal,
		SourceProject: "HerbHall/samverk",
		DependsOn: DependencyList{
			{Number: 42},
			{Owner: "HerbHall", Repo: "devkit", Number: 15},
		},
	}
	markdown := "## Summary\n\nCross-project round-trip.\n"

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

func TestParseCrossRef(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		want    Dependency
		wantErr bool
	}{
		{
			name: "valid cross-project ref",
			ref:  "HerbHall/devkit#15",
			want: Dependency{Owner: "HerbHall", Repo: "devkit", Number: 15},
		},
		{
			name: "valid with hyphenated names",
			ref:  "my-org/my-repo#999",
			want: Dependency{Owner: "my-org", Repo: "my-repo", Number: 999},
		},
		{
			name:    "missing hash separator",
			ref:     "owner/repo42",
			wantErr: true,
		},
		{
			name:    "missing slash in owner/repo",
			ref:     "ownerrepo#42",
			wantErr: true,
		},
		{
			name:    "empty owner",
			ref:     "/repo#42",
			wantErr: true,
		},
		{
			name:    "empty repo",
			ref:     "owner/#42",
			wantErr: true,
		},
		{
			name:    "non-numeric issue number",
			ref:     "owner/repo#abc",
			wantErr: true,
		},
		{
			name:    "zero issue number",
			ref:     "owner/repo#0",
			wantErr: true,
		},
		{
			name:    "negative issue number",
			ref:     "owner/repo#-5",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCrossRef(tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseCrossRef(%q) = %+v, want %+v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestDependencyList_LocalAndCrossProject(t *testing.T) {
	dl := DependencyList{
		{Number: 42},
		{Owner: "HerbHall", Repo: "devkit", Number: 15},
		{Number: 43},
		{Owner: "HerbHall", Repo: "samverk", Number: 400},
	}

	local := dl.LocalDeps()
	if len(local) != 2 || local[0] != 42 || local[1] != 43 {
		t.Errorf("LocalDeps() = %v, want [42 43]", local)
	}

	cross := dl.CrossProjectDeps()
	if len(cross) != 2 {
		t.Fatalf("CrossProjectDeps() length = %d, want 2", len(cross))
	}
	if cross[0].Owner != "HerbHall" || cross[0].Repo != "devkit" || cross[0].Number != 15 {
		t.Errorf("CrossProjectDeps()[0] = %+v, want HerbHall/devkit#15", cross[0])
	}
	if cross[1].Owner != "HerbHall" || cross[1].Repo != "samverk" || cross[1].Number != 400 {
		t.Errorf("CrossProjectDeps()[1] = %+v, want HerbHall/samverk#400", cross[1])
	}
}

func TestDependency_String(t *testing.T) {
	tests := []struct {
		dep  Dependency
		want string
	}{
		{Dependency{Number: 42}, "42"},
		{Dependency{Owner: "HerbHall", Repo: "devkit", Number: 15}, "HerbHall/devkit#15"},
	}
	for _, tt := range tests {
		got := tt.dep.String()
		if got != tt.want {
			t.Errorf("Dependency{%+v}.String() = %q, want %q", tt.dep, got, tt.want)
		}
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
	if got.SourceProject != want.SourceProject {
		t.Errorf("SourceProject: got %q, want %q", got.SourceProject, want.SourceProject)
	}
	if len(got.DependsOn) != len(want.DependsOn) {
		t.Errorf("DependsOn length: got %d, want %d", len(got.DependsOn), len(want.DependsOn))
	} else {
		for i := range want.DependsOn {
			if got.DependsOn[i] != want.DependsOn[i] {
				t.Errorf("DependsOn[%d]: got %+v, want %+v", i, got.DependsOn[i], want.DependsOn[i])
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
