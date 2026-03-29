package status

import (
	"sync"
	"testing"
	"time"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry() returned nil")
	}
	if r.subsystem == nil {
		t.Error("NewRegistry() subsystem map is nil")
	}
	if r.started.IsZero() {
		t.Error("NewRegistry() started time is zero")
	}
	if len(r.subsystem) != 0 {
		t.Errorf("NewRegistry() subsystem map not empty, got %d entries", len(r.subsystem))
	}
}

func TestRegister(t *testing.T) {
	tests := []struct {
		name        string
		subsystems  []struct{ name string; running bool }
		wantName    string
		wantRunning bool
		wantNil     bool
	}{
		{
			name:        "register running subsystem",
			subsystems:  []struct{ name string; running bool }{{"worker", true}},
			wantName:    "worker",
			wantRunning: true,
		},
		{
			name:        "register stopped subsystem",
			subsystems:  []struct{ name string; running bool }{{"poller", false}},
			wantName:    "poller",
			wantRunning: false,
		},
		{
			name: "overwrite existing registration",
			subsystems: []struct{ name string; running bool }{
				{"svc", true},
				{"svc", false},
			},
			wantName:    "svc",
			wantRunning: false,
		},
		{
			name:        "unknown subsystem returns nil",
			subsystems:  []struct{ name string; running bool }{{"known", true}},
			wantName:    "unknown",
			wantNil:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRegistry()
			for i := range tc.subsystems {
				r.Register(tc.subsystems[i].name, tc.subsystems[i].running)
			}
			got := r.GetStatus(tc.wantName)
			if tc.wantNil {
				if got != nil {
					t.Errorf("GetStatus(%q) = %+v, want nil", tc.wantName, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("GetStatus(%q) returned nil", tc.wantName)
			}
			if got.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tc.wantName)
			}
			if got.Running != tc.wantRunning {
				t.Errorf("Running = %v, want %v", got.Running, tc.wantRunning)
			}
			if got.PollCount != 0 {
				t.Errorf("PollCount = %d, want 0 after Register", got.PollCount)
			}
			if got.Errors != 0 {
				t.Errorf("Errors = %d, want 0 after Register", got.Errors)
			}
		})
	}
}

func TestGetStatus(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantNil bool
	}{
		{
			name:    "existing subsystem",
			query:   "alpha",
			wantNil: false,
		},
		{
			name:    "missing subsystem returns nil",
			query:   "nonexistent",
			wantNil: true,
		},
		{
			name:    "empty name returns nil",
			query:   "",
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRegistry()
			r.Register("alpha", true)

			got := r.GetStatus(tc.query)
			if tc.wantNil && got != nil {
				t.Errorf("GetStatus(%q) = %+v, want nil", tc.query, got)
			}
			if !tc.wantNil && got == nil {
				t.Errorf("GetStatus(%q) = nil, want non-nil", tc.query)
			}
		})
	}

	t.Run("returns a copy not a pointer to internal state", func(t *testing.T) {
		r := NewRegistry()
		r.Register("svc", true)
		s := r.GetStatus("svc")
		if s == nil {
			t.Fatal("expected non-nil status")
		}
		// Mutate the copy; internal state must be unchanged.
		s.PollCount = 9999
		s2 := r.GetStatus("svc")
		if s2 == nil {
			t.Fatal("expected non-nil status on second get")
		}
		if s2.PollCount != 0 {
			t.Errorf("mutation of returned copy affected internal state: PollCount = %d", s2.PollCount)
		}
	})
}

func TestRecordActivity(t *testing.T) {
	tests := []struct {
		name          string
		registerFirst bool
		subsystem     string
		callCount     int
		wantPollCount int64
		wantRunning   bool
	}{
		{
			name:          "single activity increments poll count",
			registerFirst: true,
			subsystem:     "worker",
			callCount:     1,
			wantPollCount: 1,
			wantRunning:   true,
		},
		{
			name:          "multiple activities accumulate",
			registerFirst: true,
			subsystem:     "worker",
			callCount:     5,
			wantPollCount: 5,
			wantRunning:   true,
		},
		{
			name:          "activity on stopped subsystem marks it running",
			registerFirst: true,
			subsystem:     "stopped",
			callCount:     1,
			wantPollCount: 1,
			wantRunning:   true,
		},
		{
			name:          "activity on unregistered subsystem is a no-op",
			registerFirst: false,
			subsystem:     "ghost",
			callCount:     1,
			wantPollCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRegistry()
			if tc.registerFirst {
				running := tc.name != "activity on stopped subsystem marks it running"
				r.Register(tc.subsystem, running)
			}

			before := time.Now()
			for i := 0; i < tc.callCount; i++ {
				r.RecordActivity(tc.subsystem)
			}
			after := time.Now()

			if !tc.registerFirst {
				if r.GetStatus(tc.subsystem) != nil {
					t.Error("unregistered subsystem should not be created by RecordActivity")
				}
				return
			}

			got := r.GetStatus(tc.subsystem)
			if got == nil {
				t.Fatal("GetStatus returned nil after RecordActivity")
			}
			if got.PollCount != tc.wantPollCount {
				t.Errorf("PollCount = %d, want %d", got.PollCount, tc.wantPollCount)
			}
			if got.Running != tc.wantRunning {
				t.Errorf("Running = %v, want %v", got.Running, tc.wantRunning)
			}
			if tc.callCount > 0 && (got.LastActive.Before(before) || got.LastActive.After(after)) {
				t.Errorf("LastActive %v not in expected range [%v, %v]", got.LastActive, before, after)
			}
		})
	}
}

func TestRecordError(t *testing.T) {
	tests := []struct {
		name          string
		registerFirst bool
		subsystem     string
		callCount     int
		wantErrors    int64
	}{
		{
			name:          "single error increments count",
			registerFirst: true,
			subsystem:     "worker",
			callCount:     1,
			wantErrors:    1,
		},
		{
			name:          "multiple errors accumulate",
			registerFirst: true,
			subsystem:     "worker",
			callCount:     3,
			wantErrors:    3,
		},
		{
			name:          "error on unregistered subsystem is a no-op",
			registerFirst: false,
			subsystem:     "ghost",
			callCount:     1,
			wantErrors:    0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRegistry()
			if tc.registerFirst {
				r.Register(tc.subsystem, true)
			}
			for i := 0; i < tc.callCount; i++ {
				r.RecordError(tc.subsystem)
			}
			if !tc.registerFirst {
				if r.GetStatus(tc.subsystem) != nil {
					t.Error("unregistered subsystem should not be created by RecordError")
				}
				return
			}
			got := r.GetStatus(tc.subsystem)
			if got == nil {
				t.Fatal("GetStatus returned nil")
			}
			if got.Errors != tc.wantErrors {
				t.Errorf("Errors = %d, want %d", got.Errors, tc.wantErrors)
			}
		})
	}
}

func TestAll(t *testing.T) {
	tests := []struct {
		name       string
		subsystems []string
		wantCount  int
	}{
		{
			name:      "empty registry returns empty slice",
			wantCount: 0,
		},
		{
			name:       "single subsystem",
			subsystems: []string{"a"},
			wantCount:  1,
		},
		{
			name:       "multiple subsystems",
			subsystems: []string{"a", "b", "c"},
			wantCount:  3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRegistry()
			for _, s := range tc.subsystems {
				r.Register(s, true)
			}
			got := r.All()
			if len(got) != tc.wantCount {
				t.Errorf("All() len = %d, want %d", len(got), tc.wantCount)
			}
			// Verify names match registered set.
			nameSet := make(map[string]bool, len(got))
			for i := range got {
				nameSet[got[i].Name] = true
			}
			for _, s := range tc.subsystems {
				if !nameSet[s] {
					t.Errorf("All() missing subsystem %q", s)
				}
			}
		})
	}

	t.Run("returns snapshot not live references", func(t *testing.T) {
		r := NewRegistry()
		r.Register("svc", true)
		snap := r.All()
		if len(snap) != 1 {
			t.Fatalf("expected 1, got %d", len(snap))
		}
		snap[0].PollCount = 9999
		live := r.GetStatus("svc")
		if live == nil {
			t.Fatal("GetStatus returned nil")
		}
		if live.PollCount != 0 {
			t.Errorf("snapshot mutation affected live state: PollCount = %d", live.PollCount)
		}
	})
}

func TestUptime(t *testing.T) {
	before := time.Now()
	r := NewRegistry()
	after := time.Now()

	uptime := r.Uptime()
	maxUptime := time.Since(before)

	if uptime < 0 {
		t.Errorf("Uptime() = %v, want >= 0", uptime)
	}
	if uptime > maxUptime {
		t.Errorf("Uptime() = %v, exceeds elapsed time since before NewRegistry %v", uptime, maxUptime)
	}
	_ = after // bounds check via maxUptime is sufficient
}

func TestRegistryConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	r.Register("concurrent", true)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines * 3)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			r.RecordActivity("concurrent")
		}()
		go func() {
			defer wg.Done()
			r.RecordError("concurrent")
		}()
		go func() {
			defer wg.Done()
			_ = r.GetStatus("concurrent")
		}()
	}

	wg.Wait()

	got := r.GetStatus("concurrent")
	if got == nil {
		t.Fatal("GetStatus returned nil after concurrent access")
	}
	if got.PollCount != goroutines {
		t.Errorf("PollCount = %d, want %d", got.PollCount, goroutines)
	}
	if got.Errors != goroutines {
		t.Errorf("Errors = %d, want %d", got.Errors, goroutines)
	}
}