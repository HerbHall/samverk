package autonomy

import "path"

// AutonomyPolicy evaluates the trust tier for agent actions.
type AutonomyPolicy interface {
	TierFor(action ActionType) Tier
	RequiresConfirmation(action ActionType) bool
	CostThreshold() float64
}

// Policy holds the resolved autonomy configuration.
type Policy struct {
	config   Config
	defaults map[ActionType]Tier
}

// NewPolicy creates a Policy from the given config.
func NewPolicy(cfg Config) *Policy {
	return &Policy{
		config:   cfg,
		defaults: SystemDefaults(),
	}
}

// ForContext returns a context-bound policy that resolves tiers using the
// full precedence chain: branch > agent > global > system default.
func (p *Policy) ForContext(agentType, branch string) AutonomyPolicy {
	return &contextPolicy{
		policy:    p,
		agentType: agentType,
		branch:    branch,
	}
}

// TierFor resolves the tier for an action using only global overrides
// and system defaults (no agent or branch context).
func (p *Policy) TierFor(action ActionType) Tier {
	if tier, ok := p.config.TierOverrides[action]; ok {
		return tier
	}
	if tier, ok := p.defaults[action]; ok {
		return tier
	}
	return Tier3
}

// RequiresConfirmation returns true if the action is Tier 3.
func (p *Policy) RequiresConfirmation(action ActionType) bool {
	return p.TierFor(action) == Tier3
}

// CostThreshold returns the API cost threshold in USD.
func (p *Policy) CostThreshold() float64 {
	if p.config.Defaults.APICostThresholdUSD > 0 {
		return p.config.Defaults.APICostThresholdUSD
	}
	return DefaultCostThresholdUSD
}

// contextPolicy resolves tiers using the full precedence chain.
type contextPolicy struct {
	policy    *Policy
	agentType string
	branch    string
}

// TierFor resolves the tier using: branch > agent > global > system default.
func (cp *contextPolicy) TierFor(action ActionType) Tier {
	// 1. Branch-specific override (highest precedence).
	if tier, ok := cp.branchTier(action); ok {
		return tier
	}

	// 2. Agent-type override.
	if agent, ok := cp.policy.config.Agents[cp.agentType]; ok {
		if tier, ok := agent.TierOverrides[action]; ok {
			return tier
		}
	}

	// 3. Global override + system default.
	return cp.policy.TierFor(action)
}

// RequiresConfirmation returns true if the action is Tier 3 in this context.
func (cp *contextPolicy) RequiresConfirmation(action ActionType) bool {
	return cp.TierFor(action) == Tier3
}

// CostThreshold returns the API cost threshold in USD.
func (cp *contextPolicy) CostThreshold() float64 {
	return cp.policy.CostThreshold()
}

// branchTier checks for a branch-specific tier override.
// It supports exact matches and glob-style patterns (e.g., "feature/*").
func (cp *contextPolicy) branchTier(action ActionType) (Tier, bool) {
	if cp.branch == "" {
		return 0, false
	}

	// Exact match first.
	if branch, ok := cp.policy.config.Branches[cp.branch]; ok {
		if tier, ok := branch.TierOverrides[action]; ok {
			return tier, true
		}
	}

	// Glob pattern match.
	for pattern, branch := range cp.policy.config.Branches {
		if pattern == cp.branch {
			continue
		}
		matched, err := path.Match(pattern, cp.branch)
		if err != nil || !matched {
			continue
		}
		if tier, ok := branch.TierOverrides[action]; ok {
			return tier, true
		}
	}

	return 0, false
}
