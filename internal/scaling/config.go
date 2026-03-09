package scaling

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// FileConfig is the top-level YAML structure for a file that contains a
// `scaling:` section (e.g. server.yaml).
type FileConfig struct {
	Scaling PolicyConfig `yaml:"scaling"`
}

// LoadConfigFile reads the `scaling:` section from a YAML file.
// If the file does not exist, it returns DefaultPolicyConfig() silently.
// If the file exists but has no `scaling:` key, the defaults still apply.
func LoadConfigFile(path string) (PolicyConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from CLI flag, not user input
	if os.IsNotExist(err) {
		return DefaultPolicyConfig(), nil
	}
	if err != nil {
		return DefaultPolicyConfig(), fmt.Errorf("read scaling config %s: %w", path, err)
	}

	var fc FileConfig
	// Pre-fill with defaults so missing keys keep their default values.
	fc.Scaling = DefaultPolicyConfig()
	if err = yaml.Unmarshal(data, &fc); err != nil {
		return DefaultPolicyConfig(), fmt.Errorf("parse scaling config %s: %w", path, err)
	}
	if err = Validate(fc.Scaling); err != nil {
		return DefaultPolicyConfig(), fmt.Errorf("invalid scaling config: %w", err)
	}
	return fc.Scaling, nil
}

// Validate checks the PolicyConfig for invalid values and returns an error
// describing the first problem found.
func Validate(cfg PolicyConfig) error {
	if cfg.MinWorkers < 1 {
		return fmt.Errorf("min_workers must be >= 1, got %d", cfg.MinWorkers)
	}
	if cfg.MaxWorkers < cfg.MinWorkers {
		return fmt.Errorf("max_workers (%d) must be >= min_workers (%d)", cfg.MaxWorkers, cfg.MinWorkers)
	}
	if cfg.ScaleUp.MemoryThreshold < 0 || cfg.ScaleUp.MemoryThreshold > 100 {
		return fmt.Errorf("scale_up.memory_threshold must be 0–100, got %d", cfg.ScaleUp.MemoryThreshold)
	}
	if cfg.ScaleUp.QueueDepthTrigger < 1 {
		return fmt.Errorf("scale_up.queue_depth_trigger must be >= 1, got %d", cfg.ScaleUp.QueueDepthTrigger)
	}
	if cfg.ScaleDown.IdleWorkerThreshold < 1 {
		return fmt.Errorf("scale_down.idle_worker_threshold must be >= 1, got %d", cfg.ScaleDown.IdleWorkerThreshold)
	}
	if cfg.EvaluationInterval != 0 && cfg.EvaluationInterval < time.Second {
		return fmt.Errorf("evaluation_interval must be >= 1s, got %s", cfg.EvaluationInterval)
	}
	return nil
}
