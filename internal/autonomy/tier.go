package autonomy

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Tier represents the autonomy trust level for an agent action.
type Tier int

const (
	// Tier1 actions proceed immediately with no user involvement.
	Tier1 Tier = 1

	// Tier2 actions proceed but are surfaced in the check-in digest.
	Tier2 Tier = 2

	// Tier3 actions are queued for user confirmation before proceeding.
	Tier3 Tier = 3
)

func (t Tier) String() string {
	switch t {
	case Tier1:
		return "tier1"
	case Tier2:
		return "tier2"
	case Tier3:
		return "tier3"
	default:
		return fmt.Sprintf("tier(%d)", int(t))
	}
}

// ParseTier converts a string to a Tier value.
func ParseTier(s string) (Tier, error) {
	switch s {
	case "tier1", "1":
		return Tier1, nil
	case "tier2", "2":
		return Tier2, nil
	case "tier3", "3":
		return Tier3, nil
	default:
		return 0, fmt.Errorf("unknown tier: %q", s)
	}
}

// ActionType identifies a specific kind of agent action.
type ActionType string

const (
	// Tier 1 defaults (always autonomous).
	ActionReadFile     ActionType = "read_file"
	ActionSearch       ActionType = "search"
	ActionCreateFile   ActionType = "create_file"
	ActionOpenIssue    ActionType = "open_issue"
	ActionCommentIssue ActionType = "comment_issue"
	ActionLabelIssue   ActionType = "label_issue"
	ActionRunTests     ActionType = "run_tests"
	ActionCreateBranch ActionType = "create_branch"
	ActionCommit       ActionType = "commit"
	ActionRunLint      ActionType = "run_lint"
	ActionQueryAPI     ActionType = "query_api"

	// Tier 2 defaults (autonomous with logging).
	ActionEditFile     ActionType = "edit_file"
	ActionCloseIssue   ActionType = "close_issue"
	ActionCreatePR     ActionType = "create_pr"
	ActionPushBranch   ActionType = "push_branch"
	ActionMergeStaging ActionType = "merge_staging"
	ActionDeleteTemp   ActionType = "delete_temp"
	ActionAPICallPaid  ActionType = "api_call_paid"
	ActionRunService   ActionType = "run_service"

	// Tier 3 defaults (requires confirmation).
	ActionMergeMain      ActionType = "merge_main"
	ActionAddDependency  ActionType = "add_dependency"
	ActionModifyCI       ActionType = "modify_ci"
	ActionDeleteFile     ActionType = "delete_file"
	ActionForcePush      ActionType = "force_push"
	ActionAPICallExpense ActionType = "api_call_expensive"
	ActionCreateTag      ActionType = "create_tag"
	ActionRestructure    ActionType = "restructure"
	ActionModifySecrets  ActionType = "modify_secrets"
	ActionIrreversible   ActionType = "irreversible"
)

// SystemDefaults returns the built-in tier assignments for all known actions.
func SystemDefaults() map[ActionType]Tier {
	return map[ActionType]Tier{
		// Tier 1
		ActionReadFile:     Tier1,
		ActionSearch:       Tier1,
		ActionCreateFile:   Tier1,
		ActionOpenIssue:    Tier1,
		ActionCommentIssue: Tier1,
		ActionLabelIssue:   Tier1,
		ActionRunTests:     Tier1,
		ActionCreateBranch: Tier1,
		ActionCommit:       Tier1,
		ActionRunLint:      Tier1,
		ActionQueryAPI:     Tier1,

		// Tier 2
		ActionEditFile:     Tier2,
		ActionCloseIssue:   Tier2,
		ActionCreatePR:     Tier2,
		ActionPushBranch:   Tier2,
		ActionMergeStaging: Tier2,
		ActionDeleteTemp:   Tier2,
		ActionAPICallPaid:  Tier2,
		ActionRunService:   Tier2,

		// Tier 3
		ActionMergeMain:      Tier3,
		ActionAddDependency:  Tier3,
		ActionModifyCI:       Tier3,
		ActionDeleteFile:     Tier3,
		ActionForcePush:      Tier3,
		ActionAPICallExpense: Tier3,
		ActionCreateTag:      Tier3,
		ActionRestructure:    Tier3,
		ActionModifySecrets:  Tier3,
		ActionIrreversible:   Tier3,
	}
}

// DefaultCostThresholdUSD is the default API cost threshold in USD.
const DefaultCostThresholdUSD = 5.00

// UnmarshalYAML implements yaml.Unmarshaler for Tier.
func (t *Tier) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := ParseTier(s)
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// MarshalYAML implements yaml.Marshaler for Tier.
func (t Tier) MarshalYAML() (interface{}, error) {
	return t.String(), nil
}
