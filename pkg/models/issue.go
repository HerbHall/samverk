package models

// SchemaVersion is the current issue schema version.
const SchemaVersion = "1.0.0"

// IssueType identifies the communication purpose of an issue.
type IssueType string

const (
	IssueTypeTask     IssueType = "task"
	IssueTypeQuestion IssueType = "question"
	IssueTypeResult   IssueType = "result"
	IssueTypeBlock    IssueType = "block"
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

// IssueFrontmatter represents the YAML frontmatter parsed from an issue body.
type IssueFrontmatter struct {
	SchemaVersion   string     `yaml:"schema_version"`
	Type            IssueType  `yaml:"type"`
	AgentType       AgentType  `yaml:"agent_type"`
	Priority        Priority   `yaml:"priority"`
	ParentIssue     int        `yaml:"parent_issue,omitempty"`
	DependsOn       []int      `yaml:"depends_on,omitempty"`
	EstimatedTokens int        `yaml:"estimated_tokens,omitempty"`
	ActualTokens    int        `yaml:"actual_tokens,omitempty"`
	ModelUsed       string     `yaml:"model_used,omitempty"`
}
