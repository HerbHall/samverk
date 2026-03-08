package agent

import (
	"strings"
)

// FileEdit represents a single file edit extracted from an agent response.
type FileEdit struct {
	Path    string
	Content string
}

// ParseResponse holds the parsed result of an agent's response.
type ParseResponse struct {
	Edits   []FileEdit
	PRTitle string
}

// ParseEditBlocks extracts EDIT/END file blocks and the PR_TITLE line
// from an agent response string. Returns nil edits if no EDIT blocks found.
func ParseEditBlocks(response string) *ParseResponse {
	result := &ParseResponse{}
	lines := strings.Split(response, "\n")

	var currentPath string
	var currentContent strings.Builder
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "PR_TITLE:") {
			result.PRTitle = strings.TrimSpace(strings.TrimPrefix(trimmed, "PR_TITLE:"))
			continue
		}

		if strings.HasPrefix(trimmed, "EDIT ") && !inBlock {
			currentPath = strings.TrimSpace(strings.TrimPrefix(trimmed, "EDIT "))
			currentContent.Reset()
			inBlock = true
			continue
		}

		if trimmed == "END" && inBlock {
			content := currentContent.String()
			// Remove leading newline if present (artifact of first line after EDIT).
			content = strings.TrimPrefix(content, "\n")
			result.Edits = append(result.Edits, FileEdit{
				Path:    currentPath,
				Content: content,
			})
			inBlock = false
			continue
		}

		if inBlock {
			if currentContent.Len() > 0 {
				currentContent.WriteByte('\n')
			}
			currentContent.WriteString(line)
		}
	}

	return result
}

// BranchSlug generates a branch name from an issue number and title.
// Format: agent/issue-{N}-{slug}, truncated to 50 chars.
func BranchSlug(issueNumber int, title string) string {
	words := strings.Fields(strings.ToLower(title))
	if len(words) > 5 {
		words = words[:5]
	}

	// Keep only alphanumeric and hyphens.
	var parts []string
	for _, w := range words {
		var clean strings.Builder
		for _, r := range w {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				clean.WriteRune(r)
			}
		}
		if s := clean.String(); s != "" {
			parts = append(parts, s)
		}
	}

	slug := strings.Join(parts, "-")
	branch := "agent/issue-" + itoa(issueNumber) + "-" + slug

	if len(branch) > 50 {
		branch = branch[:50]
	}
	// Remove trailing hyphen.
	branch = strings.TrimRight(branch, "-")

	return branch
}

// itoa is a simple int-to-string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
