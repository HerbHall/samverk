package scaling

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "scaling-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err = f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	_ = f.Close()
	return f.Name()
}

func TestLoadConfigFile_Missing(t *testing.T) {
	cfg, err := LoadConfigFile(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	def := DefaultPolicyConfig()
	if cfg.MinWorkers != def.MinWorkers || cfg.MaxWorkers != def.MaxWorkers {
		t.Errorf("missing file: got %+v, want defaults %+v", cfg, def)
	}
}

func TestLoadConfigFile_Empty(t *testing.T) {
	path := writeYAML(t, "")
	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("empty file should not error: %v", err)
	}
	def := DefaultPolicyConfig()
	if cfg.MinWorkers != def.MinWorkers {
		t.Errorf("empty file: MinWorkers = %d, want %d", cfg.MinWorkers, def.MinWorkers)
	}
}

func TestLoadConfigFile_FullSection(t *testing.T) {
	yaml := `
scaling:
  enabled: true
  min_workers: 2
  max_workers: 12
  evaluation_interval: 45s
  cooldown_after_scale: 3m
  scale_up:
    memory_threshold: 70
    queue_depth_trigger: 3
    all_workers_busy: false
  scale_down:
    idle_worker_threshold: 4
    idle_duration: 90s
`
	cfg, err := LoadConfigFile(writeYAML(t, yaml))
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if !cfg.Enabled {
		t.Error("enabled should be true")
	}
	if cfg.MinWorkers != 2 {
		t.Errorf("min_workers = %d, want 2", cfg.MinWorkers)
	}
	if cfg.MaxWorkers != 12 {
		t.Errorf("max_workers = %d, want 12", cfg.MaxWorkers)
	}
	if cfg.EvaluationInterval != 45*time.Second {
		t.Errorf("evaluation_interval = %s, want 45s", cfg.EvaluationInterval)
	}
	if cfg.CooldownAfterScale != 3*time.Minute {
		t.Errorf("cooldown_after_scale = %s, want 3m", cfg.CooldownAfterScale)
	}
	if cfg.ScaleUp.MemoryThreshold != 70 {
		t.Errorf("scale_up.memory_threshold = %d, want 70", cfg.ScaleUp.MemoryThreshold)
	}
	if cfg.ScaleUp.QueueDepthTrigger != 3 {
		t.Errorf("scale_up.queue_depth_trigger = %d, want 3", cfg.ScaleUp.QueueDepthTrigger)
	}
	if cfg.ScaleUp.AllWorkersBusy {
		t.Error("scale_up.all_workers_busy should be false")
	}
	if cfg.ScaleDown.IdleWorkerThreshold != 4 {
		t.Errorf("scale_down.idle_worker_threshold = %d, want 4", cfg.ScaleDown.IdleWorkerThreshold)
	}
	if cfg.ScaleDown.IdleDuration != 90*time.Second {
		t.Errorf("scale_down.idle_duration = %s, want 90s", cfg.ScaleDown.IdleDuration)
	}
}

func TestLoadConfigFile_NoScalingSection(t *testing.T) {
	yaml := `projects: []`
	cfg, err := LoadConfigFile(writeYAML(t, yaml))
	if err != nil {
		t.Fatalf("file without scaling section should not error: %v", err)
	}
	def := DefaultPolicyConfig()
	// enabled defaults to false but other fields should be defaults
	if cfg.MaxWorkers != def.MaxWorkers {
		t.Errorf("no scaling section: MaxWorkers = %d, want %d", cfg.MaxWorkers, def.MaxWorkers)
	}
}

func TestLoadConfigFile_InvalidYAML(t *testing.T) {
	path := writeYAML(t, "scaling: [invalid")
	if _, err := LoadConfigFile(path); err == nil {
		t.Error("invalid YAML should return error")
	}
}

// --- Validate ---

func TestValidate_Valid(t *testing.T) {
	if err := Validate(DefaultPolicyConfig()); err != nil {
		t.Errorf("default config should be valid: %v", err)
	}
}

func TestValidate_MinWorkersZero(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.MinWorkers = 0
	if err := Validate(cfg); err == nil {
		t.Error("min_workers=0 should fail validation")
	}
}

func TestValidate_MaxLessThanMin(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.MinWorkers = 5
	cfg.MaxWorkers = 3
	if err := Validate(cfg); err == nil {
		t.Error("max < min should fail validation")
	}
}

func TestValidate_MemoryThresholdOutOfRange(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.ScaleUp.MemoryThreshold = 150
	if err := Validate(cfg); err == nil {
		t.Error("memory_threshold=150 should fail validation")
	}
}

func TestValidate_QueueDepthTriggerZero(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.ScaleUp.QueueDepthTrigger = 0
	if err := Validate(cfg); err == nil {
		t.Error("queue_depth_trigger=0 should fail validation")
	}
}

func TestValidate_IdleWorkerThresholdZero(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.ScaleDown.IdleWorkerThreshold = 0
	if err := Validate(cfg); err == nil {
		t.Error("idle_worker_threshold=0 should fail validation")
	}
}

func TestValidate_EvaluationIntervalTooShort(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.EvaluationInterval = 500 * time.Millisecond
	if err := Validate(cfg); err == nil {
		t.Error("evaluation_interval < 1s should fail validation")
	}
}
