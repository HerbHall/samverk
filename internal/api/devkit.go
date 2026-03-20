package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type devkitRuleInfo struct {
	Name       string `json:"name"`
	SizeBytes  int64  `json:"size_bytes"`
	EntryCount int    `json:"entry_count,omitempty"`
}

type devkitSkillInfo struct {
	Name         string `json:"name"`
	HasWorkflows bool   `json:"has_workflows"`
}

type devkitSummary struct {
	Configured bool              `json:"configured"`
	Path       string            `json:"path,omitempty"`
	Rules      []devkitRuleInfo  `json:"rules,omitempty"`
	Skills     []devkitSkillInfo `json:"skills,omitempty"`
	Agents     []string          `json:"agents,omitempty"`
	SkillCount int               `json:"skill_count,omitempty"`
	AgentCount int               `json:"agent_count,omitempty"`
}

// handleDevkitSummary handles GET /api/v1/devkit/summary.
// Reads the DevKit directory at SAMVERK_DEVKIT_PATH and returns a summary
// of rules, skills, and agents found there.
func (a *API) handleDevkitSummary(w http.ResponseWriter, _ *http.Request) {
	devkitPath := os.Getenv("SAMVERK_DEVKIT_PATH")
	if devkitPath == "" {
		writeJSON(w, http.StatusOK, devkitSummary{Configured: false})
		return
	}

	claudePath := filepath.Join(devkitPath, "claude")

	summary := devkitSummary{
		Configured: true,
		Path:       devkitPath,
	}

	summary.Rules = readDevkitRules(filepath.Join(claudePath, "rules"))
	summary.Skills = readDevkitSkills(filepath.Join(claudePath, "skills"))
	summary.Agents = readDevkitAgents(filepath.Join(claudePath, "agents"))
	summary.SkillCount = len(summary.Skills)
	summary.AgentCount = len(summary.Agents)

	writeJSON(w, http.StatusOK, summary)
}

// readDevkitRules reads all markdown files in the rules directory and
// parses their entry_count frontmatter field.
func readDevkitRules(rulesPath string) []devkitRuleInfo {
	entries, err := os.ReadDir(rulesPath)
	if err != nil {
		return nil
	}

	var rules []devkitRuleInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		rule := devkitRuleInfo{
			Name:      entry.Name(),
			SizeBytes: info.Size(),
		}
		rule.EntryCount = parseEntryCount(filepath.Join(rulesPath, entry.Name()))
		rules = append(rules, rule)
	}
	return rules
}

// parseEntryCount scans a file for "entry_count: N" in its YAML frontmatter.
func parseEntryCount(path string) int {
	data, err := os.ReadFile(path) //nolint:gosec // path is constructed from trusted env var
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "entry_count:") {
			var count int
			if _, err := fmt.Sscanf(strings.TrimPrefix(line, "entry_count:"), " %d", &count); err == nil {
				return count
			}
		}
	}
	return 0
}

// readDevkitSkills reads subdirectories from the skills directory and
// checks whether each has a workflows/ subdirectory.
func readDevkitSkills(skillsPath string) []devkitSkillInfo {
	entries, err := os.ReadDir(skillsPath)
	if err != nil {
		return nil
	}

	var skills []devkitSkillInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skill := devkitSkillInfo{
			Name:         entry.Name(),
			HasWorkflows: dirExists(filepath.Join(skillsPath, entry.Name(), "workflows")),
		}
		skills = append(skills, skill)
	}
	return skills
}

// readDevkitAgents reads all markdown files in the agents directory and
// returns their base names (without extension).
func readDevkitAgents(agentsPath string) []string {
	entries, err := os.ReadDir(agentsPath)
	if err != nil {
		return nil
	}

	var agents []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		agents = append(agents, strings.TrimSuffix(entry.Name(), ".md"))
	}
	return agents
}

// dirExists returns true if path is an existing directory.
func dirExists(path string) bool {
	info, err := os.Stat(path) //nolint:gosec // G703: path is constructed from trusted SAMVERK_DEVKIT_PATH env var
	return err == nil && info.IsDir()
}
