package scaling

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "scaling*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer f.Close()
	if _, err = f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return f.Name()
}

func TestLoadConfigFile_EmptyPath(t *testing.T) {
	cfg, err := LoadConfigFile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	def := DefaultPolicyConfig()
	if cfg.MinWorkers != def.MinWorkers || cfg.MaxWorkers != def.MaxWorkers {
		t.Errorf("expected defaults, got min=%d max=%d", cfg.MinWorkers, cfg.MaxWorkers)
	}
}

func TestLoadConfigFile_MissingFile(t *testing.T) {
	cfg, err := LoadConfigFile(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("missing file should return defaults, got error: %v", err)
	}
	if cfg.MinWorkers != DefaultPolicyConfig().MinWorkers {
		t.Error("expected default config for missing file")
	}
}

func TestLoadConfigFile_FullConfig(t *testing.T) {
	path := writeConfig(t, `
scaling:
  enabled: true
  min_workers: 2
  max_workers: 10
  scale_up:
    memory_threshold: 70
    queue_depth_trigger: 3
    all_workers_busy: true
  scale_down:
    idle_worker_threshold: 3
`)
	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MinWorkers != 2 {
		t.Errorf("expected min_workers=2, got %d", cfg.MinWorkers)
	}
	if cfg.MaxWorkers != 10 {
		t.Errorf("expected max_workers=10, got %d", cfg.MaxWorkers)
	}
	if cfg.ScaleUp.MemoryThreshold != 70 {
		t.Errorf("expected memory_threshold=70, got %d", cfg.ScaleUp.MemoryThreshold)
	}
	if cfg.ScaleDown.IdleWorkerThreshold != 3 {
		t.Errorf("expected idle_worker_threshold=3, got %d", cfg.ScaleDown.IdleWorkerThreshold)
	}
}

func TestLoadConfigFile_NoScalingSection(t *testing.T) {
	path := writeConfig(t, `# no scaling section`)
	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should use defaults when section is absent.
	def := DefaultPolicyConfig()
	if cfg.MinWorkers != def.MinWorkers || cfg.MaxWorkers != def.MaxWorkers {
		t.Errorf("expected defaults for missing section, got min=%d max=%d", cfg.MinWorkers, cfg.MaxWorkers)
	}
}

func TestLoadConfigFile_InvalidYAML(t *testing.T) {
	path := writeConfig(t, `scaling: {{{bad yaml`)
	_, err := LoadConfigFile(path)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

func TestValidate_OK(t *testing.T) {
	if err := Validate(DefaultPolicyConfig()); err != nil {
		t.Errorf("default config should be valid: %v", err)
	}
}

func TestValidate_MinWorkersBelowOne(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.MinWorkers = 0
	if err := Validate(cfg); err == nil {
		t.Error("expected error for min_workers=0")
	}
}

func TestValidate_MaxBelowMin(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.MinWorkers = 5
	cfg.MaxWorkers = 3
	if err := Validate(cfg); err == nil {
		t.Error("expected error when max < min")
	}
}

func TestValidate_MemoryThresholdOutOfRange(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.ScaleUp.MemoryThreshold = 0
	if err := Validate(cfg); err == nil {
		t.Error("expected error for memory_threshold=0")
	}
	cfg.ScaleUp.MemoryThreshold = 101
	if err := Validate(cfg); err == nil {
		t.Error("expected error for memory_threshold=101")
	}
}

func TestValidate_QueueDepthBelowOne(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.ScaleUp.QueueDepthTrigger = 0
	if err := Validate(cfg); err == nil {
		t.Error("expected error for queue_depth_trigger=0")
	}
}

func TestValidate_IdleThresholdBelowOne(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.ScaleDown.IdleWorkerThreshold = 0
	if err := Validate(cfg); err == nil {
		t.Error("expected error for idle_worker_threshold=0")
	}
}
