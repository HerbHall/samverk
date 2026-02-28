package autonomy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPolicy(t *testing.T) {
	policy := NewPolicy(DefaultConfig())

	tests := []struct {
		name   string
		action ActionType
		want   Tier
	}{
		{"read_file is tier1", ActionReadFile, Tier1},
		{"search is tier1", ActionSearch, Tier1},
		{"commit is tier1", ActionCommit, Tier1},
		{"edit_file is tier2", ActionEditFile, Tier2},
		{"create_pr is tier2", ActionCreatePR, Tier2},
		{"push_branch is tier2", ActionPushBranch, Tier2},
		{"merge_main is tier3", ActionMergeMain, Tier3},
		{"add_dependency is tier3", ActionAddDependency, Tier3},
		{"force_push is tier3", ActionForcePush, Tier3},
		{"modify_secrets is tier3", ActionModifySecrets, Tier3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := policy.TierFor(tt.action)
			if got != tt.want {
				t.Errorf("TierFor(%q) = %v, want %v", tt.action, got, tt.want)
			}
		})
	}
}

func TestDefaultCostThreshold(t *testing.T) {
	policy := NewPolicy(DefaultConfig())
	if got := policy.CostThreshold(); got != DefaultCostThresholdUSD {
		t.Errorf("CostThreshold() = %v, want %v", got, DefaultCostThresholdUSD)
	}
}

func TestRequiresConfirmation(t *testing.T) {
	policy := NewPolicy(DefaultConfig())

	if policy.RequiresConfirmation(ActionReadFile) {
		t.Error("ActionReadFile should not require confirmation")
	}
	if policy.RequiresConfirmation(ActionEditFile) {
		t.Error("ActionEditFile should not require confirmation")
	}
	if !policy.RequiresConfirmation(ActionMergeMain) {
		t.Error("ActionMergeMain should require confirmation")
	}
}

func TestGlobalOverrides(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TierOverrides = map[ActionType]Tier{
		ActionMergeStaging: Tier1,
		ActionDeleteTemp:   Tier3,
	}

	policy := NewPolicy(cfg)

	if got := policy.TierFor(ActionMergeStaging); got != Tier1 {
		t.Errorf("merge_staging override: got %v, want tier1", got)
	}
	if got := policy.TierFor(ActionDeleteTemp); got != Tier3 {
		t.Errorf("delete_temp override: got %v, want tier3", got)
	}
	// Non-overridden action keeps default.
	if got := policy.TierFor(ActionReadFile); got != Tier1 {
		t.Errorf("read_file default: got %v, want tier1", got)
	}
}

func TestAgentOverrides(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agents = map[string]ScopedOverrides{
		"qc": {
			TierOverrides: map[ActionType]Tier{
				ActionCloseIssue: Tier1,
			},
		},
		"code-gen": {
			TierOverrides: map[ActionType]Tier{
				ActionPushBranch: Tier3,
			},
		},
	}

	policy := NewPolicy(cfg)

	// QC agent: close_issue promoted to tier1.
	qc := policy.ForContext("qc", "")
	if got := qc.TierFor(ActionCloseIssue); got != Tier1 {
		t.Errorf("qc close_issue: got %v, want tier1", got)
	}

	// Code-gen agent: push_branch demoted to tier3.
	codeGen := policy.ForContext("code-gen", "")
	if got := codeGen.TierFor(ActionPushBranch); got != Tier3 {
		t.Errorf("code-gen push_branch: got %v, want tier3", got)
	}

	// Different agent type: not affected by QC overrides.
	docs := policy.ForContext("docs", "")
	if got := docs.TierFor(ActionCloseIssue); got != Tier2 {
		t.Errorf("docs close_issue: got %v, want tier2 (default)", got)
	}
}

func TestBranchOverrides(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Branches = map[string]ScopedOverrides{
		"main": {
			TierOverrides: map[ActionType]Tier{
				ActionPushBranch: Tier3,
			},
		},
		"feature/*": {
			TierOverrides: map[ActionType]Tier{
				ActionPushBranch: Tier1,
			},
		},
	}

	policy := NewPolicy(cfg)

	// Exact match: main.
	mainCtx := policy.ForContext("code-gen", "main")
	if got := mainCtx.TierFor(ActionPushBranch); got != Tier3 {
		t.Errorf("main push_branch: got %v, want tier3", got)
	}

	// Glob match: feature/*.
	featureCtx := policy.ForContext("code-gen", "feature/issue-42")
	if got := featureCtx.TierFor(ActionPushBranch); got != Tier1 {
		t.Errorf("feature/* push_branch: got %v, want tier1", got)
	}

	// No match: falls through to default.
	devCtx := policy.ForContext("code-gen", "dev")
	if got := devCtx.TierFor(ActionPushBranch); got != Tier2 {
		t.Errorf("dev push_branch: got %v, want tier2 (default)", got)
	}
}

func TestPrecedenceChain(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TierOverrides = map[ActionType]Tier{
		ActionPushBranch: Tier3, // global: demote to tier3
	}
	cfg.Agents = map[string]ScopedOverrides{
		"code-gen": {
			TierOverrides: map[ActionType]Tier{
				ActionPushBranch: Tier2, // agent: override to tier2
			},
		},
	}
	cfg.Branches = map[string]ScopedOverrides{
		"feature/*": {
			TierOverrides: map[ActionType]Tier{
				ActionPushBranch: Tier1, // branch: override to tier1
			},
		},
	}

	policy := NewPolicy(cfg)

	// Branch override wins over agent and global.
	ctx := policy.ForContext("code-gen", "feature/issue-42")
	if got := ctx.TierFor(ActionPushBranch); got != Tier1 {
		t.Errorf("branch > agent > global: got %v, want tier1", got)
	}

	// Without branch match, agent override wins over global.
	ctx2 := policy.ForContext("code-gen", "dev")
	if got := ctx2.TierFor(ActionPushBranch); got != Tier2 {
		t.Errorf("agent > global: got %v, want tier2", got)
	}

	// Without agent match, global override wins over system default.
	ctx3 := policy.ForContext("docs", "dev")
	if got := ctx3.TierFor(ActionPushBranch); got != Tier3 {
		t.Errorf("global > default: got %v, want tier3", got)
	}
}

func TestCustomCostThreshold(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Defaults.APICostThresholdUSD = 10.50

	policy := NewPolicy(cfg)
	if got := policy.CostThreshold(); got != 10.50 {
		t.Errorf("CostThreshold() = %v, want 10.50", got)
	}
}

func TestUnknownActionFallsToTier3(t *testing.T) {
	policy := NewPolicy(DefaultConfig())
	if got := policy.TierFor("nonexistent_action"); got != Tier3 {
		t.Errorf("unknown action: got %v, want tier3 (safe default)", got)
	}
}

func TestLoadOrDefault_NoFile(t *testing.T) {
	dir := t.TempDir()

	cfg, err := LoadOrDefault(dir)
	if err != nil {
		t.Fatalf("LoadOrDefault() error: %v", err)
	}

	if cfg.Defaults.APICostThresholdUSD != DefaultCostThresholdUSD {
		t.Errorf("cost threshold = %v, want %v", cfg.Defaults.APICostThresholdUSD, DefaultCostThresholdUSD)
	}

	// No overrides loaded.
	if len(cfg.TierOverrides) != 0 {
		t.Errorf("expected no tier overrides, got %d", len(cfg.TierOverrides))
	}
}

func TestLoadConfig_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".samverk")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `
defaults:
  api_cost_threshold_usd: 10.00

tier_overrides:
  merge_staging: tier1
  delete_temp: tier3

agents:
  qc:
    tier_overrides:
      close_issue: tier1

branches:
  main:
    tier_overrides:
      push_branch: tier3
  "feature/*":
    tier_overrides:
      push_branch: tier1
`
	configPath := filepath.Join(configDir, "autonomy.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadOrDefault(dir)
	if err != nil {
		t.Fatalf("LoadOrDefault() error: %v", err)
	}

	if cfg.Defaults.APICostThresholdUSD != 10.00 {
		t.Errorf("cost threshold = %v, want 10.00", cfg.Defaults.APICostThresholdUSD)
	}

	if tier, ok := cfg.TierOverrides[ActionMergeStaging]; !ok || tier != Tier1 {
		t.Errorf("merge_staging override: got %v, want tier1", tier)
	}

	if tier, ok := cfg.TierOverrides[ActionDeleteTemp]; !ok || tier != Tier3 {
		t.Errorf("delete_temp override: got %v, want tier3", tier)
	}

	qc, ok := cfg.Agents["qc"]
	if !ok {
		t.Fatal("expected qc agent config")
	}
	if tier, ok := qc.TierOverrides[ActionCloseIssue]; !ok || tier != Tier1 {
		t.Errorf("qc close_issue: got %v, want tier1", tier)
	}

	mainBranch, ok := cfg.Branches["main"]
	if !ok {
		t.Fatal("expected main branch config")
	}
	if tier, ok := mainBranch.TierOverrides[ActionPushBranch]; !ok || tier != Tier3 {
		t.Errorf("main push_branch: got %v, want tier3", tier)
	}
}

func TestLoadConfig_InvalidTier(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
tier_overrides:
  merge_staging: tier99
`
	path := filepath.Join(dir, "autonomy.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid tier")
	}
}

func TestLoadConfig_InvalidAction(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
tier_overrides:
  bogus_action: tier1
`
	path := filepath.Join(dir, "autonomy.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}

func TestTierString(t *testing.T) {
	tests := []struct {
		tier Tier
		want string
	}{
		{Tier1, "tier1"},
		{Tier2, "tier2"},
		{Tier3, "tier3"},
		{Tier(99), "tier(99)"},
	}

	for _, tt := range tests {
		if got := tt.tier.String(); got != tt.want {
			t.Errorf("Tier(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

func TestParseTier(t *testing.T) {
	tests := []struct {
		input   string
		want    Tier
		wantErr bool
	}{
		{"tier1", Tier1, false},
		{"tier2", Tier2, false},
		{"tier3", Tier3, false},
		{"1", Tier1, false},
		{"2", Tier2, false},
		{"3", Tier3, false},
		{"tier4", 0, true},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		got, err := ParseTier(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseTier(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseTier(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
