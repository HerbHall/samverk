package profile

// Profile represents the full user profile with 4 sections.
type Profile struct {
	ProjectConventions   ProjectConventions   `yaml:"project_conventions" json:"project_conventions"`
	TechnicalPreferences TechnicalPreferences `yaml:"technical_preferences" json:"technical_preferences"`
	AIAgentConfig        AIAgentConfig        `yaml:"ai_agent_configuration" json:"ai_agent_configuration"`
	QualityStandards     QualityStandards     `yaml:"quality_standards" json:"quality_standards"`
}

// ProjectConventions defines how projects are structured, named, and governed.
type ProjectConventions struct {
	DirectoryStructure map[string]string `yaml:"directory_structure,omitempty" json:"directory_structure,omitempty"`
	FileNaming         map[string]string `yaml:"file_naming,omitempty" json:"file_naming,omitempty"`
	Git                GitConfig         `yaml:"git,omitempty" json:"git,omitempty"`
}

// GitConfig holds git workflow preferences.
type GitConfig struct {
	BranchPattern string `yaml:"branch_pattern,omitempty" json:"branch_pattern,omitempty"`
	CommitFormat  string `yaml:"commit_format,omitempty" json:"commit_format,omitempty"`
	MergeStrategy string `yaml:"merge_strategy,omitempty" json:"merge_strategy,omitempty"`
}

// TechnicalPreferences captures language, framework, testing, and build preferences.
type TechnicalPreferences struct {
	Languages  map[string]any `yaml:"languages,omitempty" json:"languages,omitempty"`
	Frameworks map[string]any `yaml:"frameworks,omitempty" json:"frameworks,omitempty"`
	Testing    map[string]any `yaml:"testing,omitempty" json:"testing,omitempty"`
	Build      map[string]any `yaml:"build,omitempty" json:"build,omitempty"`
	Linting    map[string]any `yaml:"linting,omitempty" json:"linting,omitempty"`
}

// AIAgentConfig defines how agents behave, what tools they have, and what knowledge they carry.
type AIAgentConfig struct {
	TrustTiers    map[string]any `yaml:"trust_tiers,omitempty" json:"trust_tiers,omitempty"`
	ModelRouting  map[string]any `yaml:"model_routing,omitempty" json:"model_routing,omitempty"`
	Communication map[string]any `yaml:"communication,omitempty" json:"communication,omitempty"`
}

// QualityStandards defines error handling, review, and CI policies.
type QualityStandards struct {
	ErrorPolicy    string         `yaml:"error_policy,omitempty" json:"error_policy,omitempty"`
	ReviewPolicy   string         `yaml:"review_policy,omitempty" json:"review_policy,omitempty"`
	CIRequirements map[string]any `yaml:"ci_requirements,omitempty" json:"ci_requirements,omitempty"`
}
