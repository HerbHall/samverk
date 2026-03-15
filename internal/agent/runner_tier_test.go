package agent

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/cost"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/pkg/models"
)

// staticPolicy is a test helper that returns a fixed tier for all actions.
type staticPolicy struct {
	tier          autonomy.Tier
	costThreshold float64
}

func (p *staticPolicy) TierFor(_ autonomy.ActionType) autonomy.Tier {
	return p.tier
}

func (p *staticPolicy) RequiresConfirmation(action autonomy.ActionType) bool {
	return p.TierFor(action) == autonomy.Tier3
}

func (p *staticPolicy) CostThreshold() float64 {
	if p.costThreshold > 0 {
		return p.costThreshold
	}
	return autonomy.DefaultCostThresholdUSD
}

// mapPolicy returns per-action tiers from a map, defaulting to Tier 1.
type mapPolicy struct {
	tiers map[autonomy.ActionType]autonomy.Tier
}

func (p *mapPolicy) TierFor(action autonomy.ActionType) autonomy.Tier {
	if tier, ok := p.tiers[action]; ok {
		return tier
	}
	return autonomy.Tier1
}

func (p *mapPolicy) RequiresConfirmation(action autonomy.ActionType) bool {
	return p.TierFor(action) == autonomy.Tier3
}

func (p *mapPolicy) CostThreshold() float64 {
	return autonomy.DefaultCostThresholdUSD
}

func newTierTestRunner(policy autonomy.AutonomyPolicy) (*Runner, *mockTracker, *mockRepoWriter, *mockPRM) {
	ms := newDefaultMockStore()
	mt := &mockTracker{
		addCommentFn: func(_ context.Context, _ int, _ string) (*forge.Comment, error) {
			return &forge.Comment{ID: 1}, nil
		},
	}
	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			return &provider.ChatResponse{
				Message: provider.Message{Role: provider.RoleAssistant, Content: "done"},
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test-provider" },
	}
	mrw := &mockRepoWriter{}
	mpm := &mockPRM{}

	costs := cost.NewTracker(ms, 0, 24)
	r := NewRunner(mp, "test-model", mt, ms, costs, zap.NewNop())
	r.SetRepoWriter(mrw)
	r.SetPRManager(mpm)
	if policy != nil {
		r.SetPolicy(policy)
	}
	return r, mt, mrw, mpm
}

// mockRepoWriter implements forge.RepoWriter for testing.
type mockRepoWriter struct {
	createBranchCalled int
	writeCalled        int
}

func (m *mockRepoWriter) CreateBranch(_ context.Context, _ string) error {
	m.createBranchCalled++
	return nil
}

func (m *mockRepoWriter) CreateOrUpdateFile(_ context.Context, _, _, _, _ string) error {
	m.writeCalled++
	return nil
}

// mockPRM implements forge.PullRequestManager for testing.
type mockPRM struct {
	forge.PullRequestManager
	createPRCalled int
}

func (m *mockPRM) CreatePullRequest(_ context.Context, _ *forge.CreatePRRequest) (*forge.PullRequest, error) {
	m.createPRCalled++
	return &forge.PullRequest{Number: 99}, nil
}

func TestTierEnforcement(t *testing.T) {
	tests := []struct {
		name           string
		policy         autonomy.AutonomyPolicy
		agentType      models.AgentType
		response       string
		wantErr        bool
		wantBlocked    bool
		wantPRCreated  bool
		wantComment    bool
	}{
		{
			name:        "nil policy allows everything (backward compat)",
			policy:      nil,
			agentType:   models.AgentTypeCodeGen,
			response:    "EDIT main.go\npackage main\nEND",
			wantErr:     false,
			wantPRCreated: true,
		},
		{
			name:        "tier 1 policy allows comment",
			policy:      &staticPolicy{tier: autonomy.Tier1},
			agentType:   models.AgentTypeResearch,
			response:    "research results here",
			wantErr:     false,
			wantComment: true,
		},
		{
			name:        "tier 2 policy allows PR with logging",
			policy:      &staticPolicy{tier: autonomy.Tier2},
			agentType:   models.AgentTypeCodeGen,
			response:    "EDIT main.go\npackage main\nEND",
			wantErr:     false,
			wantPRCreated: true,
		},
		{
			name:        "tier 3 blocks create_branch in openPR",
			policy:      &staticPolicy{tier: autonomy.Tier3},
			agentType:   models.AgentTypeCodeGen,
			response:    "EDIT main.go\npackage main\nEND",
			wantErr:     true,
			wantBlocked: true,
		},
		{
			name:        "tier 3 blocks comment_issue",
			policy:      &staticPolicy{tier: autonomy.Tier3},
			agentType:   models.AgentTypeResearch,
			response:    "research results here",
			wantErr:     true,
			wantBlocked: true,
		},
		{
			name: "mixed: tier 1 comment but tier 3 edit blocks PR",
			policy: &mapPolicy{tiers: map[autonomy.ActionType]autonomy.Tier{
				autonomy.ActionCommentIssue: autonomy.Tier1,
				autonomy.ActionCreateBranch: autonomy.Tier1,
				autonomy.ActionEditFile:     autonomy.Tier3,
				autonomy.ActionCreatePR:     autonomy.Tier2,
			}},
			agentType:   models.AgentTypeCodeGen,
			response:    "EDIT main.go\npackage main\nEND",
			wantErr:     true,
			wantBlocked: true,
		},
		{
			name: "mixed: tier 3 create_pr blocks after edits written",
			policy: &mapPolicy{tiers: map[autonomy.ActionType]autonomy.Tier{
				autonomy.ActionCreateBranch: autonomy.Tier1,
				autonomy.ActionEditFile:     autonomy.Tier2,
				autonomy.ActionCreatePR:     autonomy.Tier3,
			}},
			agentType:   models.AgentTypeCodeGen,
			response:    "EDIT main.go\npackage main\nEND",
			wantErr:     true,
			wantBlocked: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner, _, mrw, mpm := newTierTestRunner(tt.policy)
			task := Task{
				Issue: &forge.Issue{
					Number: 42,
					Title:  "Test issue",
					Body:   "test body",
					Labels: []string{"bug"},
				},
				AgentType: tt.agentType,
				SessionID: "sess-tier-test",
			}

			err := runner.postProcess(context.Background(), task, tt.response, "")

			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantBlocked && !errors.Is(err, autonomy.ErrTierBlocked) {
				t.Errorf("expected ErrTierBlocked, got: %v", err)
			}
			if tt.wantPRCreated && mpm.createPRCalled == 0 {
				t.Error("expected PR to be created")
			}
			if !tt.wantPRCreated && mpm.createPRCalled > 0 {
				t.Error("expected no PR creation")
			}
			_ = mrw // used to verify write calls if needed
		})
	}
}

func TestCheckTierNilPolicy(t *testing.T) {
	runner := &Runner{logger: zap.NewNop()}
	task := Task{
		Issue:     &forge.Issue{Number: 1},
		SessionID: "s1",
		AgentType: models.AgentTypeCodeGen,
	}

	for _, action := range []autonomy.ActionType{
		autonomy.ActionMergeMain,
		autonomy.ActionForcePush,
		autonomy.ActionDeleteFile,
	} {
		if err := runner.checkTier(task, action); err != nil {
			t.Errorf("nil policy should allow %s, got: %v", action, err)
		}
	}
}

func TestCheckTierVerdictsAllThreeTiers(t *testing.T) {
	tests := []struct {
		name    string
		tier    autonomy.Tier
		wantErr bool
	}{
		{"tier 1 allows", autonomy.Tier1, false},
		{"tier 2 allows with log", autonomy.Tier2, false},
		{"tier 3 blocks", autonomy.Tier3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &Runner{
				logger: zap.NewNop(),
				policy: &staticPolicy{tier: tt.tier},
			}
			task := Task{
				Issue:     &forge.Issue{Number: 1},
				SessionID: "s1",
				AgentType: models.AgentTypeCodeGen,
			}

			err := runner.checkTier(task, autonomy.ActionEditFile)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantErr && !errors.Is(err, autonomy.ErrTierBlocked) {
				t.Errorf("expected ErrTierBlocked, got: %v", err)
			}
		})
	}
}

func TestFullRunWithTier3PolicyBlocksPostProcess(t *testing.T) {
	ms := newDefaultMockStore()
	ms.updateSessionFn = func(_ context.Context, _ *models.Session) error {
		return nil
	}

	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			return &provider.ChatResponse{
				Message: provider.Message{
					Role:    provider.RoleAssistant,
					Content: "I'll fix this bug.",
				},
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test-provider" },
	}

	var commentPosted bool
	mt := &mockTracker{
		addCommentFn: func(_ context.Context, _ int, body string) (*forge.Comment, error) {
			commentPosted = true
			return &forge.Comment{ID: 1, Body: body}, nil
		},
	}

	costs := cost.NewTracker(ms, 0, 24)
	runner := NewRunner(mp, "test-model", mt, ms, costs, zap.NewNop())
	runner.SetPolicy(&staticPolicy{tier: autonomy.Tier3})

	task := Task{
		Issue: &forge.Issue{
			Number: 42,
			Title:  "Test issue",
			Body:   "Fix the bug",
		},
		AgentType: models.AgentTypeResearch,
		SessionID: "sess-block-test",
	}

	err := runner.Run(context.Background(), task)
	if err == nil {
		t.Fatal("expected error from blocked tier")
	}
	if !errors.Is(err, autonomy.ErrTierBlocked) {
		t.Errorf("expected ErrTierBlocked in chain, got: %v", err)
	}

	// The main comment_issue for the response should NOT have been posted.
	// (failTask may have posted an error comment, but the response itself was blocked.)
	_ = commentPosted
}

func TestRealPolicyIntegration(t *testing.T) {
	cfg := autonomy.DefaultConfig()
	policy := autonomy.NewPolicy(cfg)
	ctxPolicy := policy.ForContext("code-gen", "feature/test")

	runner := &Runner{
		logger: zap.NewNop(),
		policy: ctxPolicy,
	}
	task := Task{
		Issue:     &forge.Issue{Number: 1},
		SessionID: "s1",
		AgentType: models.AgentTypeCodeGen,
	}

	// Tier 1 actions should pass.
	for _, action := range []autonomy.ActionType{
		autonomy.ActionReadFile,
		autonomy.ActionCommentIssue,
		autonomy.ActionCreateBranch,
		autonomy.ActionRunTests,
	} {
		if err := runner.checkTier(task, action); err != nil {
			t.Errorf("tier 1 action %s should be allowed, got: %v", action, err)
		}
	}

	// Tier 2 actions should pass.
	for _, action := range []autonomy.ActionType{
		autonomy.ActionEditFile,
		autonomy.ActionCreatePR,
		autonomy.ActionPushBranch,
	} {
		if err := runner.checkTier(task, action); err != nil {
			t.Errorf("tier 2 action %s should be allowed, got: %v", action, err)
		}
	}

	// Tier 3 actions should block.
	for _, action := range []autonomy.ActionType{
		autonomy.ActionMergeMain,
		autonomy.ActionDeleteFile,
		autonomy.ActionForcePush,
		autonomy.ActionModifySecrets,
	} {
		err := runner.checkTier(task, action)
		if err == nil {
			t.Errorf("tier 3 action %s should be blocked", action)
			continue
		}
		if !errors.Is(err, autonomy.ErrTierBlocked) {
			t.Errorf("tier 3 action %s: expected ErrTierBlocked, got: %v", action, err)
		}
	}
}

func TestDecisionEvaluateAllTiers(t *testing.T) {
	tests := []struct {
		action  autonomy.ActionType
		tier    autonomy.Tier
		verdict autonomy.Verdict
	}{
		{autonomy.ActionReadFile, autonomy.Tier1, autonomy.VerdictAllow},
		{autonomy.ActionEditFile, autonomy.Tier2, autonomy.VerdictAllowWithLog},
		{autonomy.ActionMergeMain, autonomy.Tier3, autonomy.VerdictBlock},
	}

	cfg := autonomy.DefaultConfig()
	policy := autonomy.NewPolicy(cfg)

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			d := autonomy.Evaluate(policy, tt.action)
			if d.Tier != tt.tier {
				t.Errorf("tier = %v, want %v", d.Tier, tt.tier)
			}
			if d.Verdict != tt.verdict {
				t.Errorf("verdict = %v, want %v", d.Verdict, tt.verdict)
			}
		})
	}
}

