package api

import (
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

// agentDTO is the JSON body for a single agent (provider) in the agents endpoint.
type agentDTO struct {
	Name           string           `json:"name"`
	Type           string           `json:"type"`
	Model          string           `json:"model"`
	Healthy        bool             `json:"healthy"`
	AccountURL     string           `json:"account_url,omitempty"`
	RoutingChains  []string         `json:"routing_chains"`
	Stats          agentStatsDTO    `json:"stats"`
	RecentSessions []agentSessionDTO `json:"recent_sessions"`
	// Throughput is the number of completed sessions per hour bucket (last 24h).
	Throughput []throughputPoint `json:"throughput"`
}

// agentStatsDTO contains aggregated session statistics for a provider.
type agentStatsDTO struct {
	TotalSessions   int     `json:"total_sessions"`
	Completed       int     `json:"completed"`
	Failed          int     `json:"failed"`
	Active          int     `json:"active"`
	SuccessRate     float64 `json:"success_rate"`
	AvgDurationSecs float64 `json:"avg_duration_secs"`
}

// agentSessionDTO is a lightweight session summary for the recent tasks list.
type agentSessionDTO struct {
	ID          string  `json:"id"`
	IssueNumber int     `json:"issue_number"`
	Status      string  `json:"status"`
	StartedAt   string  `json:"started_at"`
	DurationSecs *float64 `json:"duration_secs,omitempty"`
}

// throughputPoint represents a single hourly throughput data point.
type throughputPoint struct {
	Hour  string `json:"hour"`
	Count int    `json:"count"`
}

// agentsResponse is the JSON body returned by GET /api/v1/agents.
type agentsResponse struct {
	Agents []agentDTO `json:"agents"`
}

// handleAgents serves GET /api/v1/agents.
// It aggregates provider info, session stats, and recent sessions per provider.
func (a *API) handleAgents(w http.ResponseWriter, r *http.Request) {
	// Build provider map from capacity config.
	type providerMeta struct {
		dto        ProviderDTO
		chains     []string
	}
	providerMap := make(map[string]*providerMeta)

	if a.capacity != nil {
		for i := range a.capacity.Providers {
			p := a.capacity.Providers[i]
			pm := &providerMeta{dto: p}
			providerMap[p.Name] = pm
		}
		// Compute routing chains per provider.
		for chain, providers := range a.capacity.RoutingChains {
			for _, pName := range providers {
				if pm, ok := providerMap[pName]; ok {
					pm.chains = append(pm.chains, chain)
				}
			}
		}
	}

	// Override health from the health monitor if available.
	if a.healthMonitor != nil {
		for _, h := range a.healthMonitor.AllHealth() {
			if pm, ok := providerMap[h.Name]; ok {
				pm.dto.Healthy = h.Healthy
			}
		}
	}

	// Fetch all sessions from the store and group by provider.
	type providerSessions struct {
		all []*models.Session
	}
	sessionsByProvider := make(map[string]*providerSessions)

	if a.store != nil {
		sessions, err := a.store.ListSessions(r.Context(), "") // all statuses
		if err == nil {
			for _, s := range sessions {
				ps, ok := sessionsByProvider[s.Provider]
				if !ok {
					ps = &providerSessions{}
					sessionsByProvider[s.Provider] = ps
				}
				ps.all = append(ps.all, s)
			}
		}
	}

	// Build response for each known provider.
	agents := make([]agentDTO, 0, len(providerMap))
	for name, pm := range providerMap {
		agent := agentDTO{
			Name:          name,
			Type:          pm.dto.Type,
			Model:         pm.dto.Model,
			Healthy:       pm.dto.Healthy,
			AccountURL:    pm.dto.AccountURL,
			RoutingChains: pm.chains,
		}
		if agent.RoutingChains == nil {
			agent.RoutingChains = []string{}
		}

		ps := sessionsByProvider[name]
		if ps != nil {
			agent.Stats = computeAgentStats(ps.all)
			agent.RecentSessions = recentSessions(ps.all, 10)
			agent.Throughput = computeThroughput(ps.all)
		} else {
			agent.Stats = agentStatsDTO{}
			agent.RecentSessions = []agentSessionDTO{}
			agent.Throughput = []throughputPoint{}
		}

		agents = append(agents, agent)
	}

	// Sort agents alphabetically by name for stable output.
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})

	writeJSON(w, http.StatusOK, agentsResponse{Agents: agents})
}

// computeAgentStats aggregates session statistics for a provider.
func computeAgentStats(sessions []*models.Session) agentStatsDTO {
	stats := agentStatsDTO{TotalSessions: len(sessions)}
	var totalDuration time.Duration
	completedWithDuration := 0

	for _, s := range sessions {
		switch s.Status {
		case models.SessionStatusCompleted:
			stats.Completed++
		case models.SessionStatusFailed:
			stats.Failed++
		case models.SessionStatusActive:
			stats.Active++
		case models.SessionStatusPending, models.SessionStatusCancelled:
			// Pending and cancelled sessions are counted in TotalSessions
			// but not tracked separately in the stats breakdown.
		}
		if s.FinishedAt != nil {
			d := s.FinishedAt.Sub(s.StartedAt)
			if d > 0 {
				totalDuration += d
				completedWithDuration++
			}
		}
	}

	finished := stats.Completed + stats.Failed
	if finished > 0 {
		stats.SuccessRate = math.Round(float64(stats.Completed)/float64(finished)*10000) / 100
	}
	if completedWithDuration > 0 {
		stats.AvgDurationSecs = math.Round(totalDuration.Seconds()/float64(completedWithDuration)*10) / 10
	}

	return stats
}

// recentSessions returns the most recent n sessions, sorted newest first.
func recentSessions(sessions []*models.Session, n int) []agentSessionDTO {
	// sessions are already ordered by created_at DESC from ListSessions
	limit := n
	if len(sessions) < limit {
		limit = len(sessions)
	}

	result := make([]agentSessionDTO, 0, limit)
	for i := 0; i < limit; i++ {
		s := sessions[i]
		dto := agentSessionDTO{
			ID:          s.ID,
			IssueNumber: s.IssueNumber,
			Status:      string(s.Status),
			StartedAt:   s.StartedAt.Format(time.RFC3339),
		}
		if s.FinishedAt != nil {
			d := s.FinishedAt.Sub(s.StartedAt).Seconds()
			if d > 0 {
				rounded := math.Round(d*10) / 10
				dto.DurationSecs = &rounded
			}
		}
		result = append(result, dto)
	}
	return result
}

// computeThroughput calculates hourly completed-session counts for the last 24 hours.
func computeThroughput(sessions []*models.Session) []throughputPoint {
	now := time.Now().UTC()
	cutoff := now.Add(-24 * time.Hour)

	// Initialize 24 hourly buckets.
	buckets := make(map[string]int, 24)
	hours := make([]string, 0, 24)
	for i := 23; i >= 0; i-- {
		h := now.Add(-time.Duration(i) * time.Hour).Truncate(time.Hour).Format("2006-01-02T15:00:00Z")
		buckets[h] = 0
		hours = append(hours, h)
	}

	for _, s := range sessions {
		if s.Status != models.SessionStatusCompleted {
			continue
		}
		if s.FinishedAt == nil || s.FinishedAt.Before(cutoff) {
			continue
		}
		key := s.FinishedAt.UTC().Truncate(time.Hour).Format("2006-01-02T15:00:00Z")
		if _, ok := buckets[key]; ok {
			buckets[key]++
		}
	}

	result := make([]throughputPoint, 0, len(hours))
	for _, h := range hours {
		result = append(result, throughputPoint{Hour: h, Count: buckets[h]})
	}
	return result
}
