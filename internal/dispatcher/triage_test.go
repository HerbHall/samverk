package dispatcher

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"

	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/internal/provider"
	"samverk.dev/samverk/internal/store"
	"samverk.dev/samverk/pkg/models"
)

// mockProvider implements provider.Provider for triage testing.
type mockProvider struct {
	response string
	err      error
}

func (m *mockProvider) Chat(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &provider.ChatResponse{
		Message: provider.Message{
			Role:    provider.RoleAssistant,
			Content: m.response,
		},
	}, nil
}

func (m *mockProvider) Healthy(_ context.Context) bool { return true }
func (m *mockProvider) Name() string                   { return "mock" }

func newTestTriageAgent(t *testing.T, tracker *mockTracker, prov provider.Provider) (*TriageAgent, *store.SQLiteStore) {
	t.Helper()
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	entries := []TrackerEntry{{Owner: "test", Repo: "repo", Tracker: tracker}}
	cfg := DefaultTriageConfig()
	cfg.Interval = 100 * time.Millisecond // fast for tests

	logger := zap.NewNop()
	disp := New(entries, nil, st, nil, nil, logger)
	ta := NewTriageAgent(entries, st, prov, cfg, disp, logger)
	return ta, st
}

func TestCheckHardBlocks_LabelBlock(t *testing.T) {
	tracker := newMockTracker()
	ta, _ := newTestTriageAgent(t, tracker, nil)

	issue := &forge.Issue{
		Number: 1,
		Title:  "fix login page",
		Body:   "refactor login component",
		Labels: []string{models.LabelStatusNeedsHuman, models.LabelHumanReview},
	}

	reason := ta.checkHardBlocks(issue)
	if reason == "" {
		t.Fatal("expected hard block for human:review label")
	}
	// The specific message depends on which check fires first; just verify it's non-empty.
	_ = reason
}

func TestCheckHardBlocks_KeywordBlock(t *testing.T) {
	tracker := newMockTracker()
	ta, _ := newTestTriageAgent(t, tracker, nil)

	issue := &forge.Issue{
		Number: 2,
		Title:  "update authentication flow",
		Body:   "change the OAuth2 authentication handler",
		Labels: []string{models.LabelStatusNeedsHuman},
	}

	reason := ta.checkHardBlocks(issue)
	if reason == "" {
		t.Fatal("expected hard block for authentication keyword")
	}
}

func TestCheckHardBlocks_NoBlock(t *testing.T) {
	tracker := newMockTracker()
	ta, _ := newTestTriageAgent(t, tracker, nil)

	issue := &forge.Issue{
		Number: 3,
		Title:  "add unit tests for parser",
		Body:   "increase test coverage for the parser module",
		Labels: []string{models.LabelStatusNeedsHuman},
	}

	reason := ta.checkHardBlocks(issue)
	if reason != "" {
		t.Errorf("expected no hard block, got: %s", reason)
	}
}

func TestCheckHardBlocks_ConfiguredLabel(t *testing.T) {
	tracker := newMockTracker()
	ta, _ := newTestTriageAgent(t, tracker, nil)

	issue := &forge.Issue{
		Number: 4,
		Title:  "security audit",
		Body:   "review permissions",
		Labels: []string{models.LabelStatusNeedsHuman, "security"},
	}

	reason := ta.checkHardBlocks(issue)
	if reason == "" {
		t.Fatal("expected hard block for security label")
	}
}

func TestEvaluateIssue_HardBlockSkipsProvider(t *testing.T) {
	tracker := newMockTracker()
	tracker.issues[1] = &forge.Issue{
		Number: 1,
		Title:  "database migration",
		Body:   "alter the users table schema",
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusNeedsHuman},
	}

	// Provider should never be called.
	prov := &mockProvider{err: nil, response: "should not be called"}
	ta, st := newTestTriageAgent(t, tracker, prov)

	entry := TrackerEntry{Owner: "test", Repo: "repo", Tracker: tracker}
	verdict, err := ta.evaluateIssue(context.Background(), entry, tracker.issues[1])
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if verdict != VerdictConfirmedHuman {
		t.Errorf("verdict = %s, want %s", verdict, VerdictConfirmedHuman)
	}

	// Verify decision was persisted.
	decision, err := st.GetTriageDecisionForIssue(context.Background(), 1)
	if err != nil {
		t.Fatalf("get decision: %v", err)
	}
	if decision.Verdict != string(VerdictConfirmedHuman) {
		t.Errorf("stored verdict = %s, want %s", decision.Verdict, VerdictConfirmedHuman)
	}
}

func TestEvaluateIssue_AutoUnblock(t *testing.T) {
	tracker := newMockTracker()
	tracker.issues[10] = &forge.Issue{
		Number: 10,
		Title:  "add unit tests for parser",
		Body:   "increase test coverage for the parser module",
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusNeedsHuman},
	}

	evalResult := triageEvalResult{
		Verdict:   "auto_unblock",
		Reasoning: "issue is a test task, all criteria pass",
		CriteriaResults: []triageCriterion{
			{Criterion: "recoverable", Passed: true, Detail: "feature branch"},
			{Criterion: "non_destructive", Passed: true, Detail: "test-only"},
			{Criterion: "spec_aligned", Passed: true, Detail: "matches existing architecture"},
			{Criterion: "decomposable", Passed: true, Detail: "single task"},
			{Criterion: "pre_approved_scope", Passed: true, Detail: "test type"},
		},
		DocsConsulted: []string{"docs/testing.md"},
	}
	respJSON, _ := json.Marshal(evalResult)

	prov := &mockProvider{response: string(respJSON)}
	ta, st := newTestTriageAgent(t, tracker, prov)

	entry := TrackerEntry{Owner: "test", Repo: "repo", Tracker: tracker}
	verdict, err := ta.evaluateIssue(context.Background(), entry, tracker.issues[10])
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if verdict != VerdictAutoUnblock {
		t.Errorf("verdict = %s, want %s", verdict, VerdictAutoUnblock)
	}

	// Verify labels were updated.
	issue := tracker.issues[10]
	hasQueued := false
	hasNeedsHuman := false
	for _, l := range issue.Labels {
		if l == models.LabelStatusQueued {
			hasQueued = true
		}
		if l == models.LabelStatusNeedsHuman {
			hasNeedsHuman = true
		}
	}
	if !hasQueued {
		t.Error("expected status:queued label after auto-unblock")
	}
	if hasNeedsHuman {
		t.Error("expected needs-human label to be removed after auto-unblock")
	}

	// Verify decision was persisted.
	decision, err := st.GetTriageDecisionForIssue(context.Background(), 10)
	if err != nil {
		t.Fatalf("get decision: %v", err)
	}
	if decision.Verdict != string(VerdictAutoUnblock) {
		t.Errorf("stored verdict = %s, want %s", decision.Verdict, VerdictAutoUnblock)
	}
}

func TestEvaluateIssue_Decompose(t *testing.T) {
	tracker := newMockTracker()
	tracker.issues[20] = &forge.Issue{
		Number: 20,
		Title:  "refactor entire module",
		Body:   "large refactoring task",
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusNeedsHuman},
	}

	evalResult := triageEvalResult{
		Verdict:   "decompose",
		Reasoning: "issue is too complex but parts are automatable",
		CriteriaResults: []triageCriterion{
			{Criterion: "decomposable", Passed: true, Detail: "can be split"},
		},
		SubIssues: []triageSubIssue{
			{Title: "part 1: extract interface", Body: "step one", AgentType: "code-gen"},
			{Title: "part 2: update tests", Body: "step two", AgentType: "test"},
		},
	}
	respJSON, _ := json.Marshal(evalResult)

	prov := &mockProvider{response: string(respJSON)}
	ta, st := newTestTriageAgent(t, tracker, prov)

	entry := TrackerEntry{Owner: "test", Repo: "repo", Tracker: tracker}
	verdict, err := ta.evaluateIssue(context.Background(), entry, tracker.issues[20])
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if verdict != VerdictDecompose {
		t.Errorf("verdict = %s, want %s", verdict, VerdictDecompose)
	}

	// Verify sub-issues were created (mockTracker starts at issue 1 for new creates).
	tracker.mu.Lock()
	numIssues := len(tracker.issues)
	tracker.mu.Unlock()
	// Original issue + 2 sub-issues = 3 total.
	if numIssues != 3 {
		t.Errorf("expected 3 issues (1 original + 2 subs), got %d", numIssues)
	}

	// Verify decision was persisted.
	decision, err := st.GetTriageDecisionForIssue(context.Background(), 20)
	if err != nil {
		t.Fatalf("get decision: %v", err)
	}
	if decision.Verdict != string(VerdictDecompose) {
		t.Errorf("stored verdict = %s, want %s", decision.Verdict, VerdictDecompose)
	}
}

func TestRunCycle_MaxUnblocksCap(t *testing.T) {
	tracker := newMockTracker()
	// Create 5 needs-human issues that would all pass evaluation.
	for i := 1; i <= 5; i++ {
		tracker.issues[i] = &forge.Issue{
			Number: i,
			Title:  "add tests",
			Body:   "simple test task",
			State:  forge.StateOpen,
			Labels: []string{models.LabelStatusNeedsHuman},
		}
	}

	evalResult := triageEvalResult{
		Verdict:   "auto_unblock",
		Reasoning: "safe to automate",
		CriteriaResults: []triageCriterion{
			{Criterion: "recoverable", Passed: true},
		},
	}
	respJSON, _ := json.Marshal(evalResult)
	prov := &mockProvider{response: string(respJSON)}

	ta, st := newTestTriageAgent(t, tracker, prov)
	ta.config.MaxUnblocksPerCycle = 3

	ta.runCycle(context.Background())

	// Verify only 3 were unblocked.
	decisions, err := st.ListTriageDecisions(context.Background(), time.Now().Add(-time.Hour), 0)
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}

	unblocked := 0
	for i := range decisions {
		if decisions[i].Verdict == string(VerdictAutoUnblock) {
			unblocked++
		}
	}
	if unblocked > 3 {
		t.Errorf("expected at most 3 auto-unblocks, got %d", unblocked)
	}
}

func TestBuildTriagePrompt(t *testing.T) {
	input := TriagePromptInput{
		IssueNumber:    42,
		IssueTitle:     "fix widget",
		IssueBody:      "the widget is broken",
		Comments:       []string{"tried approach A"},
		FailureHistory: []string{"timeout after 30min"},
		Labels:         []string{"status:needs-human", "agent:code-gen"},
		HardBlockRules: []string{"authentication", "infrastructure"},
		AutoApproveTypes: []string{"code-gen", "test"},
	}

	prompt := BuildTriagePrompt(input)
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	// Verify key sections are present.
	for _, section := range []string{
		"Issue #42",
		"fix widget",
		"the widget is broken",
		"tried approach A",
		"timeout after 30min",
		"authentication",
		"infrastructure",
		"auto_unblock",
		"decompose",
		"confirmed_human",
	} {
		if !contains(prompt, section) {
			t.Errorf("prompt missing expected section: %q", section)
		}
	}
}

func TestTriageConfig_Defaults(t *testing.T) {
	cfg := DefaultTriageConfig()

	if cfg.Interval != 2*time.Hour {
		t.Errorf("interval = %v, want 2h", cfg.Interval)
	}
	if cfg.MaxUnblocksPerCycle != 3 {
		t.Errorf("max_unblocks = %d, want 3", cfg.MaxUnblocksPerCycle)
	}
	if cfg.DigestInterval != 24*time.Hour {
		t.Errorf("digest_interval = %v, want 24h", cfg.DigestInterval)
	}
	if cfg.Provider != "claude-opus" {
		t.Errorf("provider = %q, want %q", cfg.Provider, "claude-opus")
	}
	if len(cfg.HardBlockLabels) == 0 {
		t.Error("expected non-empty hard_block_labels")
	}
	if len(cfg.HardBlockKeywords) == 0 {
		t.Error("expected non-empty hard_block_keywords")
	}
	if len(cfg.AutoApproveTypes) == 0 {
		t.Error("expected non-empty auto_approve_types")
	}
}

func TestTriageConfig_LoadYAML(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `triage:
  interval_minutes: 30
  max_unblocks_per_cycle: 5
  digest_interval_hours: 12
  hard_block_labels:
    - "custom:block"
  hard_block_keywords:
    - "custom keyword"
  auto_approve_types:
    - "docs"
  provider: "claude-haiku"
`
	path := dir + "/triage.yaml"
	if err := writeTestFile(t, path, yamlContent); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadTriageConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Interval != 30*time.Minute {
		t.Errorf("interval = %v, want 30m", cfg.Interval)
	}
	if cfg.MaxUnblocksPerCycle != 5 {
		t.Errorf("max_unblocks = %d, want 5", cfg.MaxUnblocksPerCycle)
	}
	if cfg.DigestInterval != 12*time.Hour {
		t.Errorf("digest_interval = %v, want 12h", cfg.DigestInterval)
	}
	if len(cfg.HardBlockLabels) != 1 || cfg.HardBlockLabels[0] != "custom:block" {
		t.Errorf("hard_block_labels = %v", cfg.HardBlockLabels)
	}
	if len(cfg.HardBlockKeywords) != 1 || cfg.HardBlockKeywords[0] != "custom keyword" {
		t.Errorf("hard_block_keywords = %v", cfg.HardBlockKeywords)
	}
	if len(cfg.AutoApproveTypes) != 1 || cfg.AutoApproveTypes[0] != "docs" {
		t.Errorf("auto_approve_types = %v", cfg.AutoApproveTypes)
	}
	if cfg.Provider != "claude-haiku" {
		t.Errorf("provider = %q, want %q", cfg.Provider, "claude-haiku")
	}
}

func writeTestFile(t *testing.T, path, content string) error {
	t.Helper()
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate short = %q", got)
	}
	if got := truncate("hello world", 5); got != "hello..." {
		t.Errorf("truncate long = %q, want %q", got, "hello...")
	}
}

func TestRecentlyEvaluated(t *testing.T) {
	tracker := newMockTracker()
	ta, st := newTestTriageAgent(t, tracker, nil)
	ta.config.Interval = 1 * time.Hour // long enough that a just-created decision is "recent"
	ctx := context.Background()

	// No decision yet: should not be recently evaluated.
	if ta.recentlyEvaluated(ctx, 1) {
		t.Error("expected not recently evaluated with no decisions")
	}

	// Save a decision.
	d := &store.TriageDecision{
		IssueNumber:      1,
		Project:          "test/repo",
		Verdict:          "confirmed_human",
		Reasoning:        "test",
		CriteriaMet:      "[]",
		DocsReferenced:   "[]",
		SubIssuesCreated: "[]",
		CreatedAt:        time.Now().UTC(),
	}
	if err := st.SaveTriageDecision(ctx, d); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Should now be recently evaluated.
	if !ta.recentlyEvaluated(ctx, 1) {
		t.Error("expected recently evaluated after saving decision")
	}
}
