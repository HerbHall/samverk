package api

import (
	"context"
	"net/http"
	"time"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// projectDTO is the JSON representation of a registered project.
type projectDTO struct {
	Name       string   `json:"name"`
	Owner      string   `json:"owner"`
	Repo       string   `json:"repo"`
	Phase      string   `json:"phase"`
	Tags       []string `json:"tags"`
	Active     bool     `json:"active"`
	Forge      string   `json:"forge,omitempty"`
	ForgeURL   string   `json:"forge_url,omitempty"`
	OpenIssues int      `json:"open_issues"`
	NeedsHuman int      `json:"needs_human"`
	InProgress int      `json:"in_progress"`
}

// handleListProjects returns all registered projects from the project registry,
// enriched with per-project issue counts.
// Returns an empty array when no registry is configured (single-project mode).
func (a *API) handleListProjects(w http.ResponseWriter, _ *http.Request) {
	if a.projectRegistry == nil {
		writeJSON(w, http.StatusOK, []projectDTO{})
		return
	}

	projects := a.projectRegistry.List()
	activeName := a.projectRegistry.ActiveName()

	dtos := make([]projectDTO, 0, len(projects))
	for _, p := range projects {
		tags := p.Tags
		if tags == nil {
			tags = []string{}
		}
		dto := projectDTO{
			Name:     p.Name,
			Owner:    p.Owner,
			Repo:     p.Repo,
			Phase:    p.Phase,
			Tags:     tags,
			Active:   p.Name == activeName,
			Forge:    p.ForgeType,
			ForgeURL: p.ForgeURL,
		}

		// Fetch issue counts if the project has a tracker.
		if p.Tracker != nil {
			dto.OpenIssues, dto.NeedsHuman, dto.InProgress = fetchIssueCounts(p.Tracker)
		}

		dtos = append(dtos, dto)
	}

	writeJSON(w, http.StatusOK, dtos)
}

// fetchIssueCounts queries the tracker for open issue counts broken down by status label.
// Returns (openIssues, needsHuman, inProgress). Uses a 5-second timeout.
func fetchIssueCounts(tracker forge.IssueTracker) (openIssues, needsHuman, inProgress int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issues, err := tracker.ListIssues(ctx, &forge.ListOptions{
		State:   forge.StateOpen,
		PerPage: 200,
	})
	if err != nil {
		return 0, 0, 0
	}

	openIssues = len(issues)
	for _, iss := range issues {
		for _, label := range iss.Labels {
			switch label {
			case models.LabelStatusNeedsHuman:
				needsHuman++
			case models.LabelStatusClaimed:
				inProgress++
			}
		}
	}
	return openIssues, needsHuman, inProgress
}
