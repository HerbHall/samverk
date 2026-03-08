package autonomy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the full autonomy configuration from .samverk/autonomy.yaml.
type Config struct {
	Defaults      Defaults                   `yaml:"defaults"`
	TierOverrides map[ActionType]Tier        `yaml:"tier_overrides"`
	Agents        map[string]ScopedOverrides `yaml:"agents"`
	Branches      map[string]ScopedOverrides `yaml:"branches"`
	Merge         MergeConfig                `yaml:"merge"`
}

// MergeConfig controls auto-merge behavior for agent PRs.
type MergeConfig struct {
	AutoMergeOnCIPass     bool     `yaml:"auto_merge_on_ci_pass"`
	TrustedAuthors        []string `yaml:"trusted_authors"`
	TrustedReviewers      []string `yaml:"trusted_reviewers"`
	RequireAllChecksPass  bool     `yaml:"require_all_checks_pass"`
	ExcludeLabels         []string `yaml:"exclude_labels"`
}

// Defaults holds global default values.
type Defaults struct {
	APICostThresholdUSD float64 `yaml:"api_cost_threshold_usd"`
}

// ScopedOverrides holds tier overrides for a specific agent type or branch pattern.
type ScopedOverrides struct {
	TierOverrides map[ActionType]Tier `yaml:"tier_overrides"`
}

// DefaultConfig returns a Config with system defaults and no overrides.
func DefaultConfig() Config {
	return Config{
		Defaults: Defaults{
			APICostThresholdUSD: DefaultCostThresholdUSD,
		},
	}
}

// LoadConfig reads and parses an autonomy config from the given path.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed internally, not from user input
	if err != nil {
		return Config{}, fmt.Errorf("read autonomy config: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse autonomy config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("invalid autonomy config: %w", err)
	}

	return cfg, nil
}

// LoadOrDefault loads the autonomy config from projectDir/.samverk/autonomy.yaml.
// If the file does not exist, it returns the default config.
func LoadOrDefault(projectDir string) (Config, error) {
	path := filepath.Join(projectDir, ".samverk", "autonomy.yaml")

	cfg, err := LoadConfig(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DefaultConfig(), nil
		}
		return Config{}, err
	}

	return cfg, nil
}

// validate checks that all tier values and action types in the config are valid.
func (c *Config) validate() error {
	defaults := SystemDefaults()

	for action, tier := range c.TierOverrides {
		if _, ok := defaults[action]; !ok {
			return fmt.Errorf("unknown action type in tier_overrides: %q", action)
		}
		if tier < Tier1 || tier > Tier3 {
			return fmt.Errorf("invalid tier %d for action %q", tier, action)
		}
	}

	for name, agent := range c.Agents {
		for action, tier := range agent.TierOverrides {
			if _, ok := defaults[action]; !ok {
				return fmt.Errorf("unknown action type in agents.%s: %q", name, action)
			}
			if tier < Tier1 || tier > Tier3 {
				return fmt.Errorf("invalid tier %d for action %q in agents.%s", tier, action, name)
			}
		}
	}

	for pattern, branch := range c.Branches {
		for action, tier := range branch.TierOverrides {
			if _, ok := defaults[action]; !ok {
				return fmt.Errorf("unknown action type in branches.%s: %q", pattern, action)
			}
			if tier < Tier1 || tier > Tier3 {
				return fmt.Errorf("invalid tier %d for action %q in branches.%s", tier, action, pattern)
			}
		}
	}

	return nil
}
