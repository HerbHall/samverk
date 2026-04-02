package profile

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"samverk.dev/samverk/internal/store"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newTestStore(t *testing.T) *SQLiteProfileStore {
	t.Helper()
	db := newTestDB(t)
	ps, err := New(db)
	if err != nil {
		t.Fatalf("new profile store: %v", err)
	}
	return ps
}

func testProfile(overrides ...func(*Profile)) *Profile {
	p := &Profile{
		ProjectConventions: ProjectConventions{
			DirectoryStructure: map[string]string{
				"standard": "cmd/ internal/ pkg/",
			},
			FileNaming: map[string]string{
				"go": "snake_case",
			},
			Git: GitConfig{
				BranchPattern: "feature/issue-{number}-{description}",
				CommitFormat:  "conventional",
				MergeStrategy: "squash",
			},
		},
		TechnicalPreferences: TechnicalPreferences{
			Languages: map[string]any{
				"primary": "go",
			},
		},
		AIAgentConfig: AIAgentConfig{
			ModelRouting: map[string]any{
				"implementation": "sonnet",
			},
			Communication: map[string]any{
				"style": "concise",
			},
		},
		QualityStandards: QualityStandards{
			ErrorPolicy:  "fix-forward",
			ReviewPolicy: "independent-reviewer",
			CIRequirements: map[string]any{
				"lint": true,
			},
		},
	}
	for _, fn := range overrides {
		fn(p)
	}
	return p
}

func TestNew(t *testing.T) {
	db := newTestDB(t)
	ps, err := New(db)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// Verify the profiles table exists.
	var name string
	err = db.QueryRowContext(context.Background(),
		"SELECT name FROM sqlite_master WHERE type='table' AND name='profiles'",
	).Scan(&name)
	if err != nil {
		t.Fatalf("query table: %v", err)
	}
	if name != "profiles" {
		t.Errorf("table name = %q, want %q", name, "profiles")
	}
	_ = ps
}

func TestImport_Single(t *testing.T) {
	ps := newTestStore(t)
	ctx := context.Background()

	p := testProfile()
	if err := ps.Import(ctx, p); err != nil {
		t.Fatalf("import: %v", err)
	}

	v, err := ps.Version(ctx)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if v != 1 {
		t.Errorf("version = %d, want 1", v)
	}
}

func TestImport_Multiple(t *testing.T) {
	ps := newTestStore(t)
	ctx := context.Background()

	for i := range 3 {
		p := testProfile(func(p *Profile) {
			p.QualityStandards.ErrorPolicy = "policy-" + string(rune('a'+i))
		})
		if err := ps.Import(ctx, p); err != nil {
			t.Fatalf("import %d: %v", i, err)
		}
	}

	v, err := ps.Version(ctx)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if v != 3 {
		t.Errorf("version = %d, want 3", v)
	}
}

func TestGet_Empty(t *testing.T) {
	ps := newTestStore(t)
	ctx := context.Background()

	_, err := ps.Get(ctx)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestGet_ReturnsLatest(t *testing.T) {
	ps := newTestStore(t)
	ctx := context.Background()

	p1 := testProfile(func(p *Profile) {
		p.QualityStandards.ErrorPolicy = "first"
	})
	p2 := testProfile(func(p *Profile) {
		p.QualityStandards.ErrorPolicy = "second"
	})

	if err := ps.Import(ctx, p1); err != nil {
		t.Fatalf("import first: %v", err)
	}
	if err := ps.Import(ctx, p2); err != nil {
		t.Fatalf("import second: %v", err)
	}

	got, err := ps.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.QualityStandards.ErrorPolicy != "second" {
		t.Errorf("error_policy = %q, want %q", got.QualityStandards.ErrorPolicy, "second")
	}
}

func TestGetField_TopLevel(t *testing.T) {
	ps := newTestStore(t)
	ctx := context.Background()

	p := testProfile()
	if err := ps.Import(ctx, p); err != nil {
		t.Fatalf("import: %v", err)
	}

	val, err := ps.GetField(ctx, "quality_standards.error_policy")
	if err != nil {
		t.Fatalf("get field: %v", err)
	}

	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	if s != "fix-forward" {
		t.Errorf("value = %q, want %q", s, "fix-forward")
	}
}

func TestGetField_Nested(t *testing.T) {
	ps := newTestStore(t)
	ctx := context.Background()

	p := testProfile()
	if err := ps.Import(ctx, p); err != nil {
		t.Fatalf("import: %v", err)
	}

	val, err := ps.GetField(ctx, "project_conventions.git.branch_pattern")
	if err != nil {
		t.Fatalf("get field: %v", err)
	}

	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	if s != "feature/issue-{number}-{description}" {
		t.Errorf("value = %q, want %q", s, "feature/issue-{number}-{description}")
	}
}

func TestGetField_NotFound(t *testing.T) {
	ps := newTestStore(t)
	ctx := context.Background()

	p := testProfile()
	if err := ps.Import(ctx, p); err != nil {
		t.Fatalf("import: %v", err)
	}

	_, err := ps.GetField(ctx, "nonexistent.field")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestVersion_Empty(t *testing.T) {
	ps := newTestStore(t)
	ctx := context.Background()

	v, err := ps.Version(ctx)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if v != 0 {
		t.Errorf("version = %d, want 0", v)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")

	content := `
project_conventions:
  git:
    branch_pattern: "feature/{number}"
    commit_format: "conventional"
    merge_strategy: "squash"
  file_naming:
    go: "snake_case"
technical_preferences:
  languages:
    primary: "go"
quality_standards:
  error_policy: "fix-forward"
  review_policy: "independent-reviewer"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if p.ProjectConventions.Git.BranchPattern != "feature/{number}" {
		t.Errorf("branch_pattern = %q, want %q", p.ProjectConventions.Git.BranchPattern, "feature/{number}")
	}
	if p.ProjectConventions.Git.CommitFormat != "conventional" {
		t.Errorf("commit_format = %q, want %q", p.ProjectConventions.Git.CommitFormat, "conventional")
	}
	if p.QualityStandards.ErrorPolicy != "fix-forward" {
		t.Errorf("error_policy = %q, want %q", p.QualityStandards.ErrorPolicy, "fix-forward")
	}
	lang, ok := p.TechnicalPreferences.Languages["primary"]
	if !ok {
		t.Fatal("expected languages.primary to exist")
	}
	if lang != "go" {
		t.Errorf("languages.primary = %v, want %q", lang, "go")
	}
}
