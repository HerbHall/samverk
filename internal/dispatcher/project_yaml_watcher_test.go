package dispatcher

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	"samverk.dev/samverk/internal/forge"
)

// writeProjectYAML writes a project.yaml with the given forge and URL to dir.
func writeProjectYAML(t *testing.T, dir, forgeName, forgeURL string) string {
	t.Helper()
	path := filepath.Join(dir, "project.yaml")
	content := "name: test\nforge: " + forgeName + "\nforge_url: " + forgeURL + "\nphase: development\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write project.yaml: %v", err)
	}
	return path
}

// buildTestDispatcher creates a minimal Dispatcher with one mock tracker.
func buildTestDispatcher(t *testing.T) *Dispatcher {
	t.Helper()
	logger := zaptest.NewLogger(t)
	mt := newMockTracker()
	entries := []TrackerEntry{{Owner: "test", Repo: "testrepo", Tracker: mt}}
	return New(entries, nil, nil, nil, nil, logger)
}

// TestProjectYAMLWatcher_SwapsTracker verifies that a project.yaml change triggers
// a tracker swap via the factory.
func TestProjectYAMLWatcher_SwapsTracker(t *testing.T) {
	logger := zaptest.NewLogger(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	yamlPath := writeProjectYAML(t, dir, "github", srv.URL)

	d := buildTestDispatcher(t)
	originalTracker := d.trackerEntries[0].Tracker

	newTracker := newMockTracker()
	var factoryCalled atomic.Int32
	factory := TrackerFactory(func(_, _ string) (TrackerEntry, error) {
		factoryCalled.Add(1)
		return TrackerEntry{Owner: "test", Repo: "testrepo", Tracker: newTracker}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w := newProjectYAMLWatcher(yamlPath, factory, d, logger)
	w.pollInterval = 50 * time.Millisecond
	go w.run(ctx)

	time.Sleep(20 * time.Millisecond)

	if got := d.trackerFor("test", "testrepo"); got != originalTracker {
		t.Fatal("expected original tracker before file change")
	}

	writeProjectYAML(t, dir, "gitea", srv.URL)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if factoryCalled.Load() > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if factoryCalled.Load() == 0 {
		t.Fatal("factory was never called after project.yaml change")
	}

	if got := d.trackerFor("test", "testrepo"); got != newTracker {
		t.Fatal("tracker was not swapped after project.yaml reload")
	}

	if err := d.ConfigReloadError(); err != nil {
		t.Fatalf("expected no config reload error, got: %v", err)
	}
}

// TestProjectYAMLWatcher_UnreachableForgeKeepsOldTracker verifies that when the forge
// URL is unreachable, the old tracker is kept and a reload error is surfaced.
func TestProjectYAMLWatcher_UnreachableForgeKeepsOldTracker(t *testing.T) {
	logger := zaptest.NewLogger(t)

	dir := t.TempDir()
	badURL := "http://127.0.0.1:19999"
	yamlPath := writeProjectYAML(t, dir, "github", badURL)

	d := buildTestDispatcher(t)
	originalTracker := d.trackerEntries[0].Tracker

	var factoryCalled atomic.Int32
	factory := TrackerFactory(func(_, _ string) (TrackerEntry, error) {
		factoryCalled.Add(1)
		return TrackerEntry{Owner: "test", Repo: "testrepo", Tracker: newMockTracker()}, nil
	})

	w := newProjectYAMLWatcher(yamlPath, factory, d, logger)
	w.pollInterval = 50 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go w.run(ctx)
	time.Sleep(20 * time.Millisecond)

	writeProjectYAML(t, dir, "gitea", badURL)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if d.ConfigReloadError() != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if factoryCalled.Load() > 0 {
		t.Fatal("factory was called despite unreachable forge")
	}

	if got := d.trackerFor("test", "testrepo"); got != originalTracker {
		t.Fatal("tracker was swapped despite unreachable forge")
	}

	if err := d.ConfigReloadError(); err == nil {
		t.Fatal("expected a config reload error, got nil")
	}
}

// TestProjectYAMLWatcher_FactoryErrorKeepsOldTracker verifies that when the factory
// returns an error, the old tracker is kept and the error is surfaced.
func TestProjectYAMLWatcher_FactoryErrorKeepsOldTracker(t *testing.T) {
	logger := zaptest.NewLogger(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	yamlPath := writeProjectYAML(t, dir, "github", srv.URL)

	d := buildTestDispatcher(t)
	originalTracker := d.trackerEntries[0].Tracker

	factoryErr := errors.New("simulated factory error")
	factory := TrackerFactory(func(_, _ string) (TrackerEntry, error) {
		return TrackerEntry{}, factoryErr
	})

	w := newProjectYAMLWatcher(yamlPath, factory, d, logger)
	w.pollInterval = 50 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go w.run(ctx)
	time.Sleep(20 * time.Millisecond)

	writeProjectYAML(t, dir, "gitea", srv.URL)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if d.ConfigReloadError() != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if got := d.trackerFor("test", "testrepo"); got != originalTracker {
		t.Fatal("tracker was swapped despite factory error")
	}

	reloadErr := d.ConfigReloadError()
	if reloadErr == nil {
		t.Fatal("expected config reload error, got nil")
	}
	if !errors.Is(reloadErr, factoryErr) {
		t.Fatalf("expected factory error wrapped in reload error, got: %v", reloadErr)
	}
}

// TestProjectYAMLWatcher_NoChangeNoReload verifies that an unchanged file
// does not trigger the factory.
func TestProjectYAMLWatcher_NoChangeNoReload(t *testing.T) {
	logger := zaptest.NewLogger(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	yamlPath := writeProjectYAML(t, dir, "github", srv.URL)

	d := buildTestDispatcher(t)

	var factoryCalled atomic.Int32
	factory := TrackerFactory(func(_, _ string) (TrackerEntry, error) {
		factoryCalled.Add(1)
		return TrackerEntry{}, nil
	})

	w := newProjectYAMLWatcher(yamlPath, factory, d, logger)
	w.pollInterval = 50 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go w.run(ctx)
	time.Sleep(300 * time.Millisecond)

	if factoryCalled.Load() > 0 {
		t.Fatal("factory was called but project.yaml did not change")
	}
}

// TestDispatcher_SetProjectYAMLWatcher verifies the dispatcher stores path/factory
// correctly and that ConfigReloadError is nil initially.
func TestDispatcher_SetProjectYAMLWatcher(t *testing.T) {
	d := buildTestDispatcher(t)

	if d.projectYAMLPath != "" {
		t.Errorf("expected empty projectYAMLPath initially, got %q", d.projectYAMLPath)
	}

	factory := TrackerFactory(func(_, _ string) (TrackerEntry, error) {
		return TrackerEntry{}, nil
	})
	d.SetProjectYAMLWatcher("/tmp/project.yaml", factory)

	if d.projectYAMLPath != "/tmp/project.yaml" {
		t.Errorf("projectYAMLPath = %q, want %q", d.projectYAMLPath, "/tmp/project.yaml")
	}
	if d.trackerFactory == nil {
		t.Error("trackerFactory should not be nil after SetProjectYAMLWatcher")
	}
	if err := d.ConfigReloadError(); err != nil {
		t.Errorf("expected no initial reload error, got: %v", err)
	}
}

// TestProjectYAMLWatcher_RunStartsWatcher verifies that Run() starts the watcher goroutine
// and stops it when the context is cancelled (no goroutine leak).
func TestProjectYAMLWatcher_RunStartsWatcher(t *testing.T) {
	logger := zaptest.NewLogger(t)

	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	yamlPath := writeProjectYAML(t, dir, "github", srv.URL)

	mt := &watchBlocker{mockTracker: newMockTracker()}
	entries := []TrackerEntry{{Owner: "test", Repo: "testrepo", Tracker: mt}}
	d := New(entries, nil, nil, nil, nil, logger)

	d.SetProjectYAMLWatcher(yamlPath, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

	runDone := make(chan error, 1)
	go func() { runDone <- d.Run(ctx) }()

	cancel()
	select {
	case <-runDone:
		// OK - Run returned after context cancel
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}

// watchBlocker is a mock tracker whose Watch blocks until ctx is done.
type watchBlocker struct {
	*mockTracker
}

func (wb *watchBlocker) Watch(ctx context.Context, _ func(forge.Event)) error {
	<-ctx.Done()
	return ctx.Err()
}
