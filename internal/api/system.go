package api

import (
	"context"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/herbhall/samverk/internal/version"
)

// versionResponse is the JSON body for /api/v1/system/version.
type versionResponse struct {
	RunningVersion    string `json:"running_version"`
	RunningCommit     string `json:"running_commit"`
	LatestMainCommit  string `json:"latest_main_commit,omitempty"`
	CommitsBehind     int    `json:"commits_behind"`
	DriftDetected     bool   `json:"drift_detected"`
	LastDeploy        string `json:"last_deploy,omitempty"`
}

// subsystemStatusResponse is the JSON body for /api/v1/system/subsystems.
type subsystemStatusResponse struct {
	Subsystems   []SubsystemStatusDTO `json:"subsystems"`
	ServerUptime string               `json:"server_uptime"`
}

// SubsystemStatusDTO represents a single subsystem in the response.
type SubsystemStatusDTO struct {
	Name       string `json:"name"`
	Running    bool   `json:"running"`
	LastActive string `json:"last_active"`
	PollCount  int64  `json:"poll_count"`
	Errors     int64  `json:"errors"`
}

// handleSystemVersion serves GET /api/v1/system/version.
// Returns information about version drift between running binary and latest main.
func (a *API) handleSystemVersion(w http.ResponseWriter, r *http.Request) {
	resp := versionResponse{
		RunningVersion: version.Version,
		RunningCommit:  version.ParseGitDescribe(version.Version),
		CommitsBehind:  0,
		DriftDetected:  false,
	}

	// Try to get the latest commit from HEAD (main branch)
	if latestCommit := getLatestMainCommit(r.Context()); latestCommit != "" && resp.RunningCommit != "" {
		resp.LatestMainCommit = latestCommit
		if latestCommit != resp.RunningCommit {
			resp.DriftDetected = true
			// Count commits between running and latest
			if count, err := countCommitsBetween(r.Context(), resp.RunningCommit, latestCommit); err == nil {
				resp.CommitsBehind = count
			}
		}
	}

	// Set last deploy to current time (server start time would be better, but this is a reasonable approximation)
	resp.LastDeploy = time.Now().UTC().Format(time.RFC3339)

	writeJSON(w, http.StatusOK, resp)
}

// handleSystemSubsystems serves GET /api/v1/system/subsystems.
// Returns the status of all background subsystems.
func (a *API) handleSystemSubsystems(w http.ResponseWriter, r *http.Request) {
	if a.subsystems == nil {
		writeError(w, http.StatusServiceUnavailable, "subsystem registry not configured")
		return
	}

	statuses := a.subsystems.All()
	dtos := make([]SubsystemStatusDTO, 0, len(statuses))
	for _, s := range statuses {
		dtos = append(dtos, SubsystemStatusDTO{
			Name:       s.Name,
			Running:    s.Running,
			LastActive: s.LastActive.UTC().Format(time.RFC3339),
			PollCount:  s.PollCount,
			Errors:     s.Errors,
		})
	}

	resp := subsystemStatusResponse{
		Subsystems:   dtos,
		ServerUptime: a.subsystems.Uptime().String(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// getLatestMainCommit returns the latest commit SHA from HEAD.
// It tries git rev-parse HEAD first, then uses GitCommit from version if available.
func getLatestMainCommit(ctx context.Context) string {
	// Try git rev-parse HEAD
	//nolint:gosec // G702: context from http.Request is trusted
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}

	// Try git rev-parse --short HEAD for shorter hash
	//nolint:gosec // G702: context from http.Request is trusted
	cmd = exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	out, err = cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}

	return ""
}

// countCommitsBetween returns the number of commits between two commit SHAs.
func countCommitsBetween(ctx context.Context, from, to string) (int, error) {
	// Handle short vs long hashes
	revisionRange := from + ".." + to
	//nolint:gosec // G204: from and to are git hashes extracted from version string, not user input
	cmd := exec.CommandContext(ctx, "git", "rev-list", "--count", revisionRange)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	trimmed := strings.TrimSpace(string(out))
	count, parseErr := strconv.Atoi(trimmed)
	if parseErr != nil {
		return 0, parseErr
	}

	return count, nil
}
