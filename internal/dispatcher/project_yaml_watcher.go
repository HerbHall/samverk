package dispatcher

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// projectYAML is the on-disk structure of .samverk/project.yaml.
type projectYAML struct {
	Name     string `yaml:"name"`
	Forge    string `yaml:"forge"`
	ForgeURL string `yaml:"forge_url"`
	Phase    string `yaml:"phase"`
}

// TrackerFactory creates an IssueTracker for a given forge type and URL.
// The dispatcher calls this when project.yaml changes to build a replacement TrackerEntry.
// Returning an error keeps the existing tracker active.
type TrackerFactory func(forge, forgeURL string) (TrackerEntry, error)

// projectYAMLWatcher watches a project.yaml file for mtime changes and triggers
// tracker reloads via the TrackerFactory. It is started as a goroutine by Run().
//
// The watcher polls every pollInterval (default 5s). On change it:
//  1. Parses the new YAML.
//  2. Validates forge reachability via HTTP GET.
//  3. Calls the factory to build the replacement TrackerEntry.
//  4. Swaps the active tracker atomically inside the Dispatcher.
//  5. Logs at INFO with old -> new forge values.
//
// On any error, it stores the error for ConfigReloadError() and keeps the
// existing tracker. The error is cleared on the next successful reload.
type projectYAMLWatcher struct {
	path         string
	pollInterval time.Duration
	factory      TrackerFactory
	dispatcher   *Dispatcher
	logger       *zap.Logger
}

const (
	defaultProjectYAMLPollInterval = 5 * time.Second
	forgeHealthTimeout             = 5 * time.Second
)

// newProjectYAMLWatcher creates a watcher. factory may be nil to disable automatic
// tracker construction (useful in tests that set trackers directly).
func newProjectYAMLWatcher(path string, factory TrackerFactory, d *Dispatcher, logger *zap.Logger) *projectYAMLWatcher {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &projectYAMLWatcher{
		path:         path,
		pollInterval: defaultProjectYAMLPollInterval,
		factory:      factory,
		dispatcher:   d,
		logger:       logger,
	}
}

// run polls the file until ctx is cancelled.
func (w *projectYAMLWatcher) run(ctx context.Context) {
	info, err := os.Stat(w.path) //nolint:gosec // G304: path is from trusted CLI/config
	if err != nil {
		w.logger.Warn("project.yaml: initial stat failed, hot-reload disabled",
			zap.String("path", w.path), zap.Error(err))
		return
	}
	lastMtime := info.ModTime()

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err = os.Stat(w.path) //nolint:gosec // G304: path is from trusted CLI/config
			if err != nil {
				w.logger.Warn("project.yaml: stat failed", zap.String("path", w.path), zap.Error(err))
				continue
			}
			if !info.ModTime().After(lastMtime) {
				continue
			}
			lastMtime = info.ModTime()
			w.reload(ctx)
		}
	}
}

// reload reads the updated YAML, validates, and swaps the tracker.
func (w *projectYAMLWatcher) reload(ctx context.Context) {
	data, err := os.ReadFile(w.path) //nolint:gosec // G304: path is from trusted CLI/config
	if err != nil {
		w.dispatcher.setConfigReloadError(fmt.Errorf("read project.yaml: %w", err))
		w.logger.Error("project.yaml reload: read failed", zap.Error(err))
		return
	}

	var cfg projectYAML
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		w.dispatcher.setConfigReloadError(fmt.Errorf("parse project.yaml: %w", err))
		w.logger.Error("project.yaml reload: parse failed", zap.Error(err))
		return
	}

	if cfg.ForgeURL == "" {
		w.dispatcher.setConfigReloadError(fmt.Errorf("project.yaml: forge_url is required"))
		w.logger.Error("project.yaml reload: missing forge_url")
		return
	}

	// Validate forge reachability before swapping.
	if err = checkForgeReachable(ctx, cfg.ForgeURL); err != nil {
		w.dispatcher.setConfigReloadError(fmt.Errorf("forge unreachable %s: %w", cfg.ForgeURL, err))
		w.logger.Error("project.yaml reload: forge unreachable",
			zap.String("forge_url", cfg.ForgeURL), zap.Error(err))
		return
	}

	if w.factory == nil {
		// No factory configured. Config is valid; clear any prior error.
		w.dispatcher.setConfigReloadError(nil)
		return
	}

	newEntry, err := w.factory(cfg.Forge, cfg.ForgeURL)
	if err != nil {
		w.dispatcher.setConfigReloadError(fmt.Errorf("build tracker: %w", err))
		w.logger.Error("project.yaml reload: tracker factory failed", zap.Error(err))
		return
	}

	// Perform atomic swap inside the dispatcher lock.
	oldForge, oldURL := w.dispatcher.swapPrimaryTracker(newEntry, cfg.Forge, cfg.ForgeURL)
	w.dispatcher.setConfigReloadError(nil)

	w.logger.Info("project.yaml reloaded: forge changed",
		zap.String("forge_from", oldForge),
		zap.String("forge_to", cfg.Forge),
		zap.String("url_from", oldURL),
		zap.String("url_to", cfg.ForgeURL),
	)
}

// checkForgeReachable attempts an HTTP GET to the forge URL.
// Returns nil if the response is 2xx or 3xx, an error otherwise.
func checkForgeReachable(ctx context.Context, forgeURL string) error {
	hctx, cancel := context.WithTimeout(ctx, forgeHealthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(hctx, http.MethodGet, forgeURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	client := &http.Client{
		Timeout: forgeHealthTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return nil // follow redirects
		},
	}
	resp, err := client.Do(req) //nolint:gosec // G107: forgeURL is from trusted project.yaml config
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}
