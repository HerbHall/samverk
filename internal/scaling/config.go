package scaling

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// fileConfig is the top-level YAML document structure.
// The scaling section maps to PolicyConfig.
type fileConfig struct {
	Scaling PolicyConfig `yaml:"scaling"`
}

// LoadConfigFile reads a YAML config file and returns the scaling policy
// configuration. If the file does not exist, DefaultPolicyConfig is returned.
// CLI flag overrides should be applied by the caller after this call.
func LoadConfigFile(path string) (PolicyConfig, error) {
	if path == "" {
		return DefaultPolicyConfig(), nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from CLI flag, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultPolicyConfig(), nil
		}
		return PolicyConfig{}, fmt.Errorf("scaling: read config %q: %w", path, err)
	}

	fc := fileConfig{Scaling: DefaultPolicyConfig()}
	if err = yaml.Unmarshal(data, &fc); err != nil {
		return PolicyConfig{}, fmt.Errorf("scaling: parse config %q: %w", path, err)
	}

	if err = Validate(fc.Scaling); err != nil {
		return PolicyConfig{}, fmt.Errorf("scaling: invalid config %q: %w", path, err)
	}
	return fc.Scaling, nil
}

// Validate checks that cfg contains internally consistent values.
func Validate(cfg PolicyConfig) error {
	if cfg.MinWorkers < 1 {
		return fmt.Errorf("min_workers must be >= 1, got %d", cfg.MinWorkers)
	}
	if cfg.MaxWorkers < cfg.MinWorkers {
		return fmt.Errorf("max_workers (%d) must be >= min_workers (%d)", cfg.MaxWorkers, cfg.MinWorkers)
	}
	if cfg.ScaleUp.MemoryThreshold < 1 || cfg.ScaleUp.MemoryThreshold > 100 {
		return fmt.Errorf("scale_up.memory_threshold must be 1–100, got %d", cfg.ScaleUp.MemoryThreshold)
	}
	if cfg.ScaleUp.QueueDepthTrigger < 1 {
		return fmt.Errorf("scale_up.queue_depth_trigger must be >= 1, got %d", cfg.ScaleUp.QueueDepthTrigger)
	}
	if cfg.ScaleDown.IdleWorkerThreshold < 1 {
		return fmt.Errorf("scale_down.idle_worker_threshold must be >= 1, got %d", cfg.ScaleDown.IdleWorkerThreshold)
	}
	return nil
}
