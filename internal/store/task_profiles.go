package store

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

// UpdateTaskProfile recomputes the profile for (agentType, provider) from
// completed sessions, then upserts the result.
// It uses all available completed sessions for accurate percentile calculation.
// A minimum of 1 sample is required; with fewer than 5 samples the P90 is
// estimated from the same distribution as P50.
func (s *SQLiteStore) UpdateTaskProfile(ctx context.Context, agentType, provider string) error {
	// Fetch durations from completed sessions.
	rows, err := s.db.QueryContext(ctx, `
SELECT started_at, finished_at
FROM sessions
WHERE agent_type = ?
  AND provider   = ?
  AND status     = 'completed'
  AND finished_at IS NOT NULL
ORDER BY finished_at DESC
LIMIT 200`, agentType, provider)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	var durations []time.Duration
	for rows.Next() {
		var startStr, finStr string
		if err := rows.Scan(&startStr, &finStr); err != nil {
			return err
		}
		start, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			continue
		}
		fin, err := time.Parse(time.RFC3339, finStr)
		if err != nil {
			continue
		}
		d := fin.Sub(start)
		if d > 0 {
			durations = append(durations, d)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(durations) == 0 {
		return nil
	}

	// Sort for percentile calculation.
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	avg := avgDuration(durations)
	p50 := percentileDuration(durations, 50)
	p90 := percentileDuration(durations, 90)

	// Compute average token count from cost_records for completed sessions.
	var avgTokens int
	row := s.db.QueryRowContext(ctx, `
SELECT CAST(AVG(CAST(input_tokens + output_tokens AS REAL)) AS INTEGER)
FROM cost_records cr
JOIN sessions s ON cr.session_id = s.id
WHERE s.agent_type = ?
  AND s.provider   = ?
  AND s.status     = 'completed'`, agentType, provider)
	_ = row.Scan(&avgTokens) // ignore error — tokens are best-effort

	_, err = s.db.ExecContext(ctx, `
INSERT INTO task_profiles (agent_type, provider, avg_duration_ms, p50_duration_ms, p90_duration_ms, sample_count, avg_tokens, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(agent_type, provider) DO UPDATE SET
    avg_duration_ms = excluded.avg_duration_ms,
    p50_duration_ms = excluded.p50_duration_ms,
    p90_duration_ms = excluded.p90_duration_ms,
    sample_count    = excluded.sample_count,
    avg_tokens      = excluded.avg_tokens,
    updated_at      = excluded.updated_at`,
		agentType,
		provider,
		avg.Milliseconds(),
		p50.Milliseconds(),
		p90.Milliseconds(),
		len(durations),
		avgTokens,
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// ListTaskProfiles returns all task profiles ordered by agent_type, provider.
func (s *SQLiteStore) ListTaskProfiles(ctx context.Context) ([]*models.TaskProfile, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT agent_type, provider, avg_duration_ms, p50_duration_ms, p90_duration_ms, sample_count, avg_tokens, updated_at
FROM task_profiles
ORDER BY agent_type, provider`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var profiles []*models.TaskProfile
	for rows.Next() {
		var p models.TaskProfile
		var avgMs, p50Ms, p90Ms int64
		var updStr string
		if err := rows.Scan(&p.AgentType, &p.Provider, &avgMs, &p50Ms, &p90Ms, &p.SampleCount, &p.AvgTokens, &updStr); err != nil {
			return nil, err
		}
		p.AvgDuration = time.Duration(avgMs) * time.Millisecond
		p50Dur := time.Duration(p50Ms) * time.Millisecond
		p.P50Duration = p50Dur
		p90Dur := time.Duration(p90Ms) * time.Millisecond
		p.P90Duration = p90Dur
		p.UpdatedAt, _ = time.Parse(time.RFC3339, updStr)
		profiles = append(profiles, &p)
	}
	return profiles, rows.Err()
}

// GetTaskProfile returns the profile for a specific (agentType, provider) pair.
// Returns (nil, nil) when no profile exists yet.
func (s *SQLiteStore) GetTaskProfile(ctx context.Context, agentType, provider string) (*models.TaskProfile, error) {
	var p models.TaskProfile
	var avgMs, p50Ms, p90Ms int64
	var updStr string
	err := s.db.QueryRowContext(ctx, `
SELECT agent_type, provider, avg_duration_ms, p50_duration_ms, p90_duration_ms, sample_count, avg_tokens, updated_at
FROM task_profiles
WHERE agent_type = ? AND provider = ?`, agentType, provider).
		Scan(&p.AgentType, &p.Provider, &avgMs, &p50Ms, &p90Ms, &p.SampleCount, &p.AvgTokens, &updStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.AvgDuration = time.Duration(avgMs) * time.Millisecond
	p.P50Duration = time.Duration(p50Ms) * time.Millisecond
	p.P90Duration = time.Duration(p90Ms) * time.Millisecond
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updStr)
	return &p, nil
}

// avgDuration returns the mean of a slice of durations.
func avgDuration(d []time.Duration) time.Duration {
	if len(d) == 0 {
		return 0
	}
	var sum time.Duration
	for _, v := range d {
		sum += v
	}
	return sum / time.Duration(len(d))
}

// percentileDuration returns the Pth percentile of a sorted duration slice
// using nearest-rank interpolation.
func percentileDuration(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
