package dispatcher

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all dispatcher runtime settings.
type Config struct {
	HeartbeatInterval          time.Duration `yaml:"heartbeat_interval"`
	HeartbeatTimeoutMultiplier float64       `yaml:"heartbeat_timeout_multiplier"`
	MaxConsecutiveFailures     int           `yaml:"max_consecutive_failures"`
	DependencyRecheckInterval  time.Duration `yaml:"dependency_recheck_interval"`
	HeartbeatCheckInterval     time.Duration `yaml:"heartbeat_check_interval"`
	DecompositionThreshold     time.Duration `yaml:"decomposition_threshold"`
	DecompositionModel         string        `yaml:"decomposition_model"`
}

// configFile is the on-disk YAML representation with friendly duration fields.
type configFile struct {
	HeartbeatIntervalMinutes       int     `yaml:"heartbeat_interval_minutes"`
	HeartbeatTimeoutMultiplier     float64 `yaml:"heartbeat_timeout_multiplier"`
	MaxConsecutiveFailures         int     `yaml:"max_consecutive_failures"`
	DependencyRecheckSeconds       int     `yaml:"dependency_recheck_seconds"`
	HeartbeatCheckSeconds          int     `yaml:"heartbeat_check_seconds"`
	DecompositionThresholdMinutes  int     `yaml:"decomposition_threshold_minutes"`
	DecompositionModel             string  `yaml:"decomposition_model"`
}

// DefaultConfig returns production-ready defaults for a self-hosted deployment.
func DefaultConfig() *Config {
	return &Config{
		HeartbeatInterval:          20 * time.Minute,
		HeartbeatTimeoutMultiplier: 1.5,
		MaxConsecutiveFailures:     3,
		DependencyRecheckInterval:  2 * time.Minute,
		HeartbeatCheckInterval:     60 * time.Second,
	}
}

// LoadConfig reads a YAML config file and merges it with defaults.
// Missing fields keep their default values.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: config path is from trusted CLI/server config
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cf configFile
	if err = yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg := DefaultConfig()

	if cf.HeartbeatIntervalMinutes > 0 {
		cfg.HeartbeatInterval = time.Duration(cf.HeartbeatIntervalMinutes) * time.Minute
	}
	if cf.HeartbeatTimeoutMultiplier > 0 {
		cfg.HeartbeatTimeoutMultiplier = cf.HeartbeatTimeoutMultiplier
	}
	if cf.MaxConsecutiveFailures > 0 {
		cfg.MaxConsecutiveFailures = cf.MaxConsecutiveFailures
	}
	if cf.DependencyRecheckSeconds > 0 {
		cfg.DependencyRecheckInterval = time.Duration(cf.DependencyRecheckSeconds) * time.Second
	}
	if cf.HeartbeatCheckSeconds > 0 {
		cfg.HeartbeatCheckInterval = time.Duration(cf.HeartbeatCheckSeconds) * time.Second
	}
	if cf.DecompositionThresholdMinutes > 0 {
		cfg.DecompositionThreshold = time.Duration(cf.DecompositionThresholdMinutes) * time.Minute
	}
	if cf.DecompositionModel != "" {
		cfg.DecompositionModel = cf.DecompositionModel
	}

	return cfg, nil
}
