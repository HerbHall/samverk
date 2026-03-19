package api

import "net/http"

// projectDTO is the JSON representation of a registered project.
type projectDTO struct {
	Name   string   `json:"name"`
	Owner  string   `json:"owner"`
	Repo   string   `json:"repo"`
	Phase  string   `json:"phase"`
	Tags   []string `json:"tags"`
	Active bool     `json:"active"`
}

// handleListProjects returns all registered projects from the project registry.
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
		dtos = append(dtos, projectDTO{
			Name:   p.Name,
			Owner:  p.Owner,
			Repo:   p.Repo,
			Phase:  p.Phase,
			Tags:   tags,
			Active: p.Name == activeName,
		})
	}

	writeJSON(w, http.StatusOK, dtos)
}
