package loganalyst

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/logstore"
)

// mockLogQuerier records calls and returns preset entries.
type mockLogQuerier struct {
	entries []logstore.LogEntry
	err     error
	calls   int
}

func (m *mockLogQuerier) Query(_ context.Context, _ logstore.QueryFilter) ([]logstore.LogEntry, error) {
	m.calls++
	return m.entries, m.err
}

// mockOllamaClient returns preset text or an error.
type mockOllamaClient struct {
	text string
	err  error
}

func (m *mockOllamaClient) Generate(_ context.Context, _ string) (string, error) {
	return m.text, m.err
}

func sampleEntries() []logstore.LogEntry {
	now := time.Now()
	return []logstore.LogEntry{
		{ID: 1, Timestamp: now, Level: "info", Message: "session started", Component: "agent"},
		{ID: 2, Timestamp: now.Add(time.Second), Level: "warn", Message: "slow response", Component: "provider.claude"},
		{ID: 3, Timestamp: now.Add(2 * time.Second), Level: "error", Message: "CLI process timed out after 180s", Component: "agent"},
		{ID: 4, Timestamp: now.Add(3 * time.Second), Level: "info", Message: "session completed", Component: "dispatcher"},
	}
}

func TestSummarizeWithOllama(t *testing.T) {
	logs := &mockLogQuerier{entries: sampleEntries()}
	ollama := &mockOllamaClient{text: "The session started an agent task that experienced a timeout after 180s."}
	a := New(logs, ollama, zap.NewNop())

	scope := ScopeFilter{Type: ScopeSession, ID: "sess_123"}
	summary, err := a.Summarize(context.Background(), scope)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !summary.AIGenerated {
		t.Error("expected AIGenerated=true")
	}
	if summary.TotalEntries != 4 {
		t.Errorf("expected 4 entries, got %d", summary.TotalEntries)
	}
	if summary.ErrorCount != 1 {
		t.Errorf("expected 1 error, got %d", summary.ErrorCount)
	}
	if summary.WarnCount != 1 {
		t.Errorf("expected 1 warning, got %d", summary.WarnCount)
	}
	if len(summary.Components) != 3 {
		t.Errorf("expected 3 components, got %d: %v", len(summary.Components), summary.Components)
	}
	if summary.Text == "" {
		t.Error("expected non-empty text")
	}
}

func TestSummarizeFallback(t *testing.T) {
	logs := &mockLogQuerier{entries: sampleEntries()}
	a := New(logs, nil, zap.NewNop())

	scope := ScopeFilter{Type: ScopeSession, ID: "sess_456"}
	summary, err := a.Summarize(context.Background(), scope)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.AIGenerated {
		t.Error("expected AIGenerated=false")
	}
	if summary.Text == "" {
		t.Error("expected non-empty fallback text")
	}
	if summary.TotalEntries != 4 {
		t.Errorf("expected 4 entries, got %d", summary.TotalEntries)
	}
}

func TestSummarizeOllamaError(t *testing.T) {
	logs := &mockLogQuerier{entries: sampleEntries()}
	ollama := &mockOllamaClient{err: errors.New("connection refused")}
	a := New(logs, ollama, zap.NewNop())

	scope := ScopeFilter{Type: ScopeFailures, Since: time.Now().Add(-24 * time.Hour)}
	summary, err := a.Summarize(context.Background(), scope)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.AIGenerated {
		t.Error("expected AIGenerated=false after ollama error")
	}
	if summary.Text == "" {
		t.Error("expected non-empty fallback text")
	}
}

func TestCacheHit(t *testing.T) {
	logs := &mockLogQuerier{entries: sampleEntries()}
	a := New(logs, nil, zap.NewNop())

	scope := ScopeFilter{Type: ScopeIssue, ID: "507"}

	s1, err := a.Summarize(context.Background(), scope)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	s2, err := a.Summarize(context.Background(), scope)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if logs.calls != 1 {
		t.Errorf("expected logstore queried once, got %d", logs.calls)
	}
	if s1.Text != s2.Text {
		t.Error("expected identical cached summaries")
	}
}

func TestCacheMiss(t *testing.T) {
	logs := &mockLogQuerier{entries: sampleEntries()}
	a := New(logs, nil, zap.NewNop())

	scope1 := ScopeFilter{Type: ScopeSession, ID: "sess_a"}
	scope2 := ScopeFilter{Type: ScopeSession, ID: "sess_b"}

	if _, err := a.Summarize(context.Background(), scope1); err != nil {
		t.Fatalf("scope1: %v", err)
	}
	if _, err := a.Summarize(context.Background(), scope2); err != nil {
		t.Fatalf("scope2: %v", err)
	}

	if logs.calls != 2 {
		t.Errorf("expected logstore queried twice, got %d", logs.calls)
	}
}

func TestBuildFilterValidation(t *testing.T) {
	tests := []struct {
		name    string
		scope   ScopeFilter
		wantErr bool
	}{
		{"session without id", ScopeFilter{Type: ScopeSession}, true},
		{"issue without id", ScopeFilter{Type: ScopeIssue}, true},
		{"issue with bad number", ScopeFilter{Type: ScopeIssue, ID: "abc"}, true},
		{"timerange without since", ScopeFilter{Type: ScopeTimeRange}, true},
		{"failures without since", ScopeFilter{Type: ScopeFailures}, true},
		{"unknown scope", ScopeFilter{Type: "unknown"}, true},
		{"valid session", ScopeFilter{Type: ScopeSession, ID: "s1"}, false},
		{"valid issue", ScopeFilter{Type: ScopeIssue, ID: "42"}, false},
		{"valid timerange", ScopeFilter{Type: ScopeTimeRange, Since: time.Now()}, false},
		{"valid failures", ScopeFilter{Type: ScopeFailures, Since: time.Now()}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildFilter(tt.scope)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildFilter() error=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}
