package autonomy

import "errors"

// ErrTierBlocked is returned when a Tier 3 action is blocked by autonomy policy.
var ErrTierBlocked = errors.New("action blocked by autonomy tier")

// Verdict represents whether an action should proceed.
type Verdict int

const (
	// VerdictAllow means the action proceeds immediately (Tier 1).
	VerdictAllow Verdict = iota

	// VerdictAllowWithLog means the action proceeds but is logged for audit (Tier 2).
	VerdictAllowWithLog

	// VerdictBlock means the action is blocked pending user confirmation (Tier 3).
	VerdictBlock
)

func (v Verdict) String() string {
	switch v {
	case VerdictAllow:
		return "allow"
	case VerdictAllowWithLog:
		return "allow_with_log"
	case VerdictBlock:
		return "block"
	default:
		return "unknown"
	}
}

// Decision captures the result of evaluating an action against the autonomy policy.
type Decision struct {
	Action  ActionType
	Tier    Tier
	Verdict Verdict
}

// Evaluate checks the policy for the given action and returns a Decision.
func Evaluate(policy AutonomyPolicy, action ActionType) Decision {
	tier := policy.TierFor(action)

	var verdict Verdict
	switch tier {
	case Tier1:
		verdict = VerdictAllow
	case Tier2:
		verdict = VerdictAllowWithLog
	default:
		verdict = VerdictBlock
	}

	return Decision{
		Action:  action,
		Tier:    tier,
		Verdict: verdict,
	}
}
