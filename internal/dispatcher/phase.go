package dispatcher

import "github.com/herbhall/samverk/pkg/models"

// phaseAllowedAgents maps a project lifecycle phase to the set of agent types
// permitted to work in that phase. The "inactive" phase has an empty set,
// which causes the dispatcher to skip those trackers entirely.
//
// Phase matrix:
//
//	research    — research, docs
//	planning    — research, docs, code-gen (spikes only — enforced by label check)
//	design      — research, docs, code-gen
//	development — all agent types
//	deployed    — all agent types
//	maintenance — code-gen, test, docs, qc (fixes, security, chore only)
//	inactive    — none (tracker not watched)
var phaseAllowedAgents = map[string]map[models.AgentType]bool{
	"research": {
		models.AgentTypeResearch: true,
		models.AgentTypeDocs:     true,
	},
	"planning": {
		models.AgentTypeResearch: true,
		models.AgentTypeDocs:     true,
		models.AgentTypeCodeGen:  true,
	},
	"design": {
		models.AgentTypeResearch: true,
		models.AgentTypeDocs:     true,
		models.AgentTypeCodeGen:  true,
	},
	"development": {
		models.AgentTypeOrchestrator: true,
		models.AgentTypeDispatcher:   true,
		models.AgentTypeCodeGen:      true,
		models.AgentTypeTest:         true,
		models.AgentTypeDocs:         true,
		models.AgentTypeResearch:     true,
		models.AgentTypeQC:           true,
		models.AgentTypeInfra:        true,
		models.AgentTypePC:           true,
	},
	"deployed": {
		models.AgentTypeOrchestrator: true,
		models.AgentTypeDispatcher:   true,
		models.AgentTypeCodeGen:      true,
		models.AgentTypeTest:         true,
		models.AgentTypeDocs:         true,
		models.AgentTypeResearch:     true,
		models.AgentTypeQC:           true,
		models.AgentTypeInfra:        true,
		models.AgentTypePC:           true,
	},
	"maintenance": {
		models.AgentTypeCodeGen: true,
		models.AgentTypeTest:    true,
		models.AgentTypeDocs:    true,
		models.AgentTypeQC:      true,
	},
	"inactive": {},
}

// phaseAllowed reports whether agentType is permitted for the given project phase.
// Returns true when phase is empty (no phase configured) or when the phase is
// unknown (defensive: allow rather than silently drop). Human agents bypass
// this check — they are handled before the phase gate in route().
//
// Research agents bypass phase gating entirely (Decision 2 in planning-system-design.md):
// phase gating is per-issue, not per-project, and research must route regardless
// of the current project phase.
func phaseAllowed(phase string, agentType models.AgentType) bool {
	if phase == "" {
		return true
	}
	// Research agents are never blocked by project phase (Decision 2).
	if agentType == models.AgentTypeResearch {
		return true
	}
	allowed, phaseKnown := phaseAllowedAgents[phase]
	if !phaseKnown {
		return true
	}
	return allowed[agentType]
}
