package api

import (
	"net/http"
	"time"

	"github.com/herbhall/samverk/internal/hostmetrics"
)

// SetHostMetrics attaches the host metrics collector to the API handler.
func (a *API) SetHostMetrics(hm *hostmetrics.Collector) {
	a.hostMetrics = hm
}

// hostMetricsResponse is the JSON body returned by GET /api/v1/metrics/host.
type hostMetricsResponse struct {
	Current         hostMetricsDTO          `json:"current"`
	History         []hostMetricsDTO        `json:"history"`
	Recommendations []recommendationDTO     `json:"recommendations,omitempty"`
}

// hostMetricsDTO is the JSON-serializable form of hostmetrics.Snapshot.
type hostMetricsDTO struct {
	CollectedAt    time.Time  `json:"collected_at"`
	DiskTotalBytes uint64     `json:"disk_total_bytes"`
	DiskUsedBytes  uint64     `json:"disk_used_bytes"`
	DiskPercent    float64    `json:"disk_percent"`
	RAMTotalBytes  uint64     `json:"ram_total_bytes"`
	RAMUsedBytes   uint64     `json:"ram_used_bytes"`
	RAMAvailBytes  uint64     `json:"ram_avail_bytes"`
	RAMPercent     float64    `json:"ram_percent"`
	SwapTotalBytes uint64     `json:"swap_total_bytes"`
	SwapUsedBytes  uint64     `json:"swap_used_bytes"`
	SwapPercent    float64    `json:"swap_percent"`
	LoadAvg1       float64    `json:"load_avg_1"`
	LoadAvg5       float64    `json:"load_avg_5"`
	LoadAvg15      float64    `json:"load_avg_15"`
	NumCPU         int        `json:"num_cpu"`
	CPUPercent     float64    `json:"cpu_percent"`
	CPUSource      string     `json:"cpu_source,omitempty"`
	InLXC          bool       `json:"in_lxc"`
	Alerts         []alertDTO `json:"alerts,omitempty"`
}

// alertDTO is the JSON-serializable form of hostmetrics.Alert.
type alertDTO struct {
	Resource  string  `json:"resource"`
	Level     string  `json:"level"`
	Message   string  `json:"message"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
}

// recommendationDTO is the JSON-serializable form of hostmetrics.Recommendation.
type recommendationDTO struct {
	Resource string `json:"resource"`
	Level    string `json:"level"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
}

// handleHostMetrics serves GET /api/v1/metrics/host.
func (a *API) handleHostMetrics(w http.ResponseWriter, r *http.Request) {
	if a.hostMetrics == nil {
		writeError(w, http.StatusServiceUnavailable, "host metrics not configured")
		return
	}

	// Parse ?since= query parameter; default to 24h.
	sinceDur := 24 * time.Hour
	if raw := r.URL.Query().Get("since"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			sinceDur = parsed
		}
	}

	current := a.hostMetrics.Snapshot()
	history := a.hostMetrics.History(time.Now().Add(-sinceDur))

	resp := hostMetricsResponse{
		Current: snapshotToDTO(current),
		History: make([]hostMetricsDTO, 0, len(history)),
	}
	for i := range history {
		resp.History = append(resp.History, snapshotToDTO(history[i]))
	}

	// Run recommendation engine against the full 24h window regardless of ?since=.
	analysisWindow := a.hostMetrics.History(time.Now().Add(-24 * time.Hour))
	if recs := hostmetrics.AnalyzeHistory(analysisWindow); len(recs) > 0 {
		resp.Recommendations = make([]recommendationDTO, 0, len(recs))
		for i := range recs {
			resp.Recommendations = append(resp.Recommendations, recommendationDTO{
				Resource: recs[i].Resource,
				Level:    string(recs[i].Level),
				Title:    recs[i].Title,
				Detail:   recs[i].Detail,
			})
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// snapshotToDTO converts a hostmetrics.Snapshot to the API DTO.
func snapshotToDTO(s hostmetrics.Snapshot) hostMetricsDTO {
	dto := hostMetricsDTO{
		CollectedAt:    s.CollectedAt,
		DiskTotalBytes: s.DiskTotalBytes,
		DiskUsedBytes:  s.DiskUsedBytes,
		DiskPercent:    s.DiskPercent,
		RAMTotalBytes:  s.RAMTotalBytes,
		RAMUsedBytes:   s.RAMUsedBytes,
		RAMAvailBytes:  s.RAMAvailBytes,
		RAMPercent:     s.RAMPercent,
		SwapTotalBytes: s.SwapTotalBytes,
		SwapUsedBytes:  s.SwapUsedBytes,
		SwapPercent:    s.SwapPercent,
		LoadAvg1:       s.LoadAvg1,
		LoadAvg5:       s.LoadAvg5,
		LoadAvg15:      s.LoadAvg15,
		NumCPU:         s.NumCPU,
		CPUPercent:     s.CPUPercent,
		CPUSource:      string(s.CPUSource),
		InLXC:          s.InLXC,
	}
	if len(s.Alerts) > 0 {
		dto.Alerts = make([]alertDTO, 0, len(s.Alerts))
		for i := range s.Alerts {
			dto.Alerts = append(dto.Alerts, alertDTO{
				Resource:  s.Alerts[i].Resource,
				Level:     string(s.Alerts[i].Level),
				Message:   s.Alerts[i].Message,
				Value:     s.Alerts[i].Value,
				Threshold: s.Alerts[i].Threshold,
			})
		}
	}
	return dto
}
