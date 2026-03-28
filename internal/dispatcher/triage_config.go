package dispatcher

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// TriageConfig holds all settings for the autonomous triage agent.
type TriageConfig struct {
	Interval           time.Duration `yaml:"-"`
	MaxUnblocksPerCycle int          `yaml:"-"`
	DigestInterval     time.Duration `yaml:"-"`
	HardBlockLabels    []string      `yaml:"-"`
	HardBlockKeywords  []string      `yaml:"-"`
	AutoApproveTypes   []string      `yaml:"-"`
	Provider           string        `yaml:"-"`
}

// triageConfigFile is the YAML on-disk representation.
type triageConfigFile struct {
	Triage struct {
		IntervalMinutes     int      `yaml:"interval_minutes"`
		MaxUnblocksPerCycle int      `yaml:"max_unblocks_per_cycle"`
		DigestIntervalHours int      `yaml:"digest_interval_hours"`
		HardBlockLabels     []string `yaml:"hard_block_labels"`
		HardBlockKeywords   []string `yaml:"hard_block_keywords"`
		AutoApproveTypes    []string `yaml:"auto_approve_types"`
		Provider            string   `yaml:"provider"`
	} `yaml:"triage"`
}

// DefaultTriageConfig returns production-ready defaults for the triage agent.
func DefaultTriageConfig() *TriageConfig {
	return &TriageConfig{
		Interval:           2 * time.Hour,
		MaxUnblocksPerCycle: 3,
		DigestInterval:     24 * time.Hour,
		HardBlockLabels: []string{
			"human:review",
			"security",
		},
		HardBlockKeywords: []string{
			"authentication",
			"authorization",
			"cost decision",
			"infrastructure",
			"database migration",
		},
		AutoApproveTypes: []string{
			"code-gen",
			"test",
			"docs",
		},
		Provider: "claude-opus",
	}
}

// LoadTriageConfig reads a YAML config file and merges with defaults.
func LoadTriageConfig(path string) (*TriageConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: config path is from trusted CLI/server config
	if err != nil {
		return nil, fmt.Errorf("read triage config: %w", err)
	}

	var cf triageConfigFile
	if err = yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parse triage config: %w", err)
	}

	cfg := DefaultTriageConfig()

	if cf.Triage.IntervalMinutes > 0 {
		cfg.Interval = time.Duration(cf.Triage.IntervalMinutes) * time.Minute
	}
	if cf.Triage.MaxUnblocksPerCycle > 0 {
		cfg.MaxUnblocksPerCycle = cf.Triage.MaxUnblocksPerCycle
	}
	if cf.Triage.DigestIntervalHours > 0 {
		cfg.DigestInterval = time.Duration(cf.Triage.DigestIntervalHours) * time.Hour
	}
	if len(cf.Triage.HardBlockLabels) > 0 {
		cfg.HardBlockLabels = cf.Triage.HardBlockLabels
	}
	if len(cf.Triage.HardBlockKeywords) > 0 {
		cfg.HardBlockKeywords = cf.Triage.HardBlockKeywords
	}
	if len(cf.Triage.AutoApproveTypes) > 0 {
		cfg.AutoApproveTypes = cf.Triage.AutoApproveTypes
	}
	if cf.Triage.Provider != "" {
		cfg.Provider = cf.Triage.Provider
	}

	return cfg, nil
}
