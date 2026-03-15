package logstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Syncer is a zap-compatible WriteSyncer that tees log output to both
// stdout and a SQLite LogStore. SQLite errors are swallowed (logged to
// stderr) to ensure logging never crashes the application.
type Syncer struct {
	stdout io.Writer
	store  *LogStore
}

// NewSyncer creates a WriteSyncer that writes to stdout and persists
// structured JSON log lines to the given LogStore.
func NewSyncer(stdout io.Writer, store *LogStore) *Syncer {
	return &Syncer{
		stdout: stdout,
		store:  store,
	}
}

// zapLogLine represents the JSON structure produced by zap's JSON encoder.
type zapLogLine struct {
	Level       string          `json:"level"`
	Timestamp   string          `json:"ts"`
	Message     string          `json:"msg"`
	Logger      string          `json:"logger"`
	SessionID   string          `json:"session_id"`
	IssueNumber int             `json:"issue_number"`
	Extra       json.RawMessage `json:"-"`
}

// knownKeys lists the JSON keys that are extracted into dedicated columns.
var knownKeys = map[string]bool{
	"level":        true,
	"ts":           true,
	"msg":          true,
	"logger":       true,
	"session_id":   true,
	"issue_number": true,
}

// Write implements io.Writer. It always writes p to stdout. If p is a
// valid JSON log line, it also inserts the parsed entry into SQLite.
// SQLite errors are never returned; only stdout write errors propagate.
func (s *Syncer) Write(p []byte) (n int, err error) {
	// Always write to stdout first.
	n, err = s.stdout.Write(p)
	if err != nil {
		return n, err
	}

	// Try to parse as JSON and persist to SQLite.
	s.persistJSON(p)

	return n, nil
}

// Sync implements zapcore.WriteSyncer. It syncs stdout if it supports
// the Sync method (e.g., os.File). Errors from syncing stdout/stderr
// are ignored because /dev/stdout cannot be fsynced on Linux.
func (s *Syncer) Sync() error {
	if f, ok := s.stdout.(*os.File); ok {
		_ = f.Sync() // best-effort; /dev/stdout returns EINVAL on Linux
	}
	return nil
}

// persistJSON attempts to parse p as a JSON log line and insert it into
// SQLite. Any error is printed to stderr but never propagated.
func (s *Syncer) persistJSON(p []byte) {
	// Quick check: must start with '{'.
	if len(p) == 0 || p[0] != '{' {
		return
	}

	var line zapLogLine
	if err := json.Unmarshal(p, &line); err != nil {
		return
	}

	// Must have at least level and msg to be a valid log line.
	if line.Level == "" || line.Message == "" {
		return
	}

	// Parse timestamp. Zap ISO8601 format.
	ts, tsErr := time.Parse("2006-01-02T15:04:05.000Z0700", line.Timestamp)
	if tsErr != nil {
		// Try RFC3339.
		ts, tsErr = time.Parse(time.RFC3339, line.Timestamp)
		if tsErr != nil {
			ts = time.Now().UTC()
		}
	}

	// Extract extra fields (everything not in knownKeys).
	fields := extractExtraFields(p)

	entry := &LogEntry{
		Timestamp:   ts,
		Level:       line.Level,
		Message:     line.Message,
		Component:   line.Logger,
		SessionID:   line.SessionID,
		IssueNumber: line.IssueNumber,
		Fields:      fields,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.store.Insert(ctx, entry); err != nil {
		fmt.Fprintf(os.Stderr, "logstore: insert failed: %v\n", err)
	}
}

// extractExtraFields parses the JSON blob and returns a JSON string
// containing only the fields not in knownKeys.
func extractExtraFields(p []byte) string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(p, &raw); err != nil {
		return "{}"
	}

	extra := make(map[string]json.RawMessage)
	for k, v := range raw {
		if !knownKeys[k] {
			extra[k] = v
		}
	}

	if len(extra) == 0 {
		return "{}"
	}

	b, err := json.Marshal(extra)
	if err != nil {
		return "{}"
	}
	return string(b)
}
