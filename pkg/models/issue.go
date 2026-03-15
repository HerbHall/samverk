package models

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is the current issue schema version.
const SchemaVersion = "1.1.0"

// IssueType identifies the communication purpose of an issue.
type IssueType string

const (
	IssueTypeTask         IssueType = "task"
	IssueTypeQuestion     IssueType = "question"
	IssueTypeResult       IssueType = "result"
	IssueTypeBlock        IssueType = "block"
	IssueTypeCoordination IssueType = "coordination"
)

// AgentType identifies which agent pool should handle an issue.
type AgentType string

const (
	AgentTypeOrchestrator AgentType = "orchestrator"
	AgentTypeDispatcher   AgentType = "dispatcher"
	AgentTypeCodeGen      AgentType = "code-gen"
	AgentTypeTest         AgentType = "test"
	AgentTypeDocs         AgentType = "docs"
	AgentTypeResearch     AgentType = "research"
	AgentTypeQC           AgentType = "qc"
	AgentTypeHuman        AgentType = "human"
	AgentTypeInfra        AgentType = "infra"
	AgentTypePC           AgentType = "pc"
)

// Priority determines scheduling order.
type Priority string

const (
	PriorityCritical Priority = "critical"
	PriorityHigh     Priority = "high"
	PriorityNormal   Priority = "normal"
	PriorityLow      Priority = "low"
)

// Status tracks the lifecycle state of an issue.
type Status string

const (
	StatusQueued     Status = "queued"
	StatusClaimed    Status = "claimed"
	StatusInProgress Status = "in-progress"
	StatusBlocked    Status = "blocked"
	StatusNeedsQC    Status = "needs-qc"
	StatusNeedsHuman   Status = "needs-human"
	StatusHumanPending Status = "human-pending"
	StatusDone         Status = "done"
)

// Complexity hints at whether work should run locally or in the cloud.
type Complexity string

const (
	ComplexityLocal     Complexity = "local"
	ComplexityCloud     Complexity = "cloud"
	ComplexityAmbiguous Complexity = "ambiguous"
)

// Dependency represents an issue dependency that may be same-project (int)
// or cross-project (owner/repo#number string).
type Dependency struct {
	Owner  string // empty for same-project
	Repo   string // empty for same-project
	Number int
}

// IsCrossProject returns true if the dependency references another project.
func (d Dependency) IsCrossProject() bool {
	return d.Owner != "" && d.Repo != ""
}

// String returns the canonical string form: "42" for same-project, "owner/repo#42" for cross-project.
func (d Dependency) String() string {
	if d.IsCrossProject() {
		return fmt.Sprintf("%s/%s#%d", d.Owner, d.Repo, d.Number)
	}
	return strconv.Itoa(d.Number)
}

// ParseCrossRef parses a cross-project reference string in the format "owner/repo#number".
// Returns an error if the format is invalid.
func ParseCrossRef(ref string) (Dependency, error) {
	hashIdx := strings.LastIndex(ref, "#")
	if hashIdx == -1 {
		return Dependency{}, fmt.Errorf("cross-project ref %q: missing '#' separator", ref)
	}

	ownerRepo := ref[:hashIdx]
	numStr := ref[hashIdx+1:]

	slashIdx := strings.Index(ownerRepo, "/")
	if slashIdx == -1 || slashIdx == 0 || slashIdx == len(ownerRepo)-1 {
		return Dependency{}, fmt.Errorf("cross-project ref %q: expected owner/repo format", ref)
	}

	num, err := strconv.Atoi(numStr)
	if err != nil || num <= 0 {
		return Dependency{}, fmt.Errorf("cross-project ref %q: invalid issue number %q", ref, numStr)
	}

	return Dependency{
		Owner:  ownerRepo[:slashIdx],
		Repo:   ownerRepo[slashIdx+1:],
		Number: num,
	}, nil
}

// DependencyList is a list of dependencies that supports mixed int and string YAML values.
type DependencyList []Dependency

// UnmarshalYAML implements the yaml.Unmarshaler interface for DependencyList.
// It handles both integer values (same-project) and string values (cross-project refs).
func (dl *DependencyList) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("depends_on: expected sequence, got %v", value.Kind)
	}

	result := make([]Dependency, 0, len(value.Content))
	for _, item := range value.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			// Try integer first (same-project dependency).
			if num, err := strconv.Atoi(item.Value); err == nil && num > 0 {
				result = append(result, Dependency{Number: num})
				continue
			}
			// Try cross-project reference string.
			dep, err := ParseCrossRef(item.Value)
			if err != nil {
				return fmt.Errorf("depends_on item %q: %w", item.Value, err)
			}
			result = append(result, dep)
		default:
			return fmt.Errorf("depends_on: expected scalar (int or string), got %v", item.Kind)
		}
	}

	*dl = result
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface for DependencyList.
func (dl DependencyList) MarshalYAML() (interface{}, error) {
	nodes := make([]*yaml.Node, 0, len(dl))
	for _, dep := range dl {
		node := &yaml.Node{Kind: yaml.ScalarNode}
		if dep.IsCrossProject() {
			node.Tag = "!!str"
			node.Value = dep.String()
		} else {
			node.Tag = "!!int"
			node.Value = strconv.Itoa(dep.Number)
		}
		nodes = append(nodes, node)
	}
	return &yaml.Node{
		Kind:    yaml.SequenceNode,
		Content: nodes,
	}, nil
}

// LocalDeps returns only same-project dependency issue numbers.
func (dl DependencyList) LocalDeps() []int {
	result := make([]int, 0, len(dl))
	for _, dep := range dl {
		if !dep.IsCrossProject() {
			result = append(result, dep.Number)
		}
	}
	return result
}

// CrossProjectDeps returns only cross-project dependencies.
func (dl DependencyList) CrossProjectDeps() []Dependency {
	result := make([]Dependency, 0)
	for _, dep := range dl {
		if dep.IsCrossProject() {
			result = append(result, dep)
		}
	}
	return result
}

// IssueFrontmatter represents the YAML frontmatter parsed from an issue body.
type IssueFrontmatter struct {
	SchemaVersion   string         `yaml:"schema_version"`
	Type            IssueType      `yaml:"type"`
	AgentType       AgentType      `yaml:"agent_type"`
	Priority        Priority       `yaml:"priority"`
	ParentIssue     int            `yaml:"parent_issue,omitempty"`
	DependsOn       DependencyList `yaml:"depends_on,omitempty"`
	SourceProject   string         `yaml:"source_project,omitempty"`
	EstimatedTokens int            `yaml:"estimated_tokens,omitempty"`
	ActualTokens    int            `yaml:"actual_tokens,omitempty"`
	ModelUsed       string         `yaml:"model_used,omitempty"`
	TimeoutMinutes  int            `yaml:"timeout_minutes,omitempty"`
}
