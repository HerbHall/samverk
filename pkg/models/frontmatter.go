package models

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseResult holds the parsed frontmatter and remaining body text.
type ParseResult struct {
	Frontmatter *IssueFrontmatter
	Body        string // markdown after the closing ---
}

const frontmatterDelimiter = "---"

// ParseFrontmatter extracts YAML frontmatter from an issue body.
// The frontmatter must be delimited by --- on its own line at the start of the body.
// Returns error if frontmatter is present but malformed.
// Returns nil Frontmatter (no error) if no frontmatter block exists.
func ParseFrontmatter(body string) (*ParseResult, error) {
	trimmed := strings.TrimLeft(body, " \t\r\n")

	if !strings.HasPrefix(trimmed, frontmatterDelimiter+"\n") {
		return &ParseResult{Frontmatter: nil, Body: body}, nil
	}

	// Skip past the opening delimiter line.
	afterOpen := trimmed[len(frontmatterDelimiter)+1:]

	// Handle empty frontmatter (---\n---) and content frontmatter.
	// Look for closing --- on its own line: either "\n---" or at position 0 (empty block).
	var closeIdx int
	if strings.HasPrefix(afterOpen, frontmatterDelimiter+"\n") || afterOpen == frontmatterDelimiter {
		// Empty frontmatter block: closing --- immediately follows opening.
		closeIdx = 0
	} else {
		idx := strings.Index(afterOpen, "\n"+frontmatterDelimiter)
		if idx == -1 {
			return nil, errors.New("frontmatter: missing closing delimiter")
		}
		closeIdx = idx + 1 // skip the \n so yamlContent excludes trailing newline
	}

	yamlContent := afterOpen[:closeIdx]

	var fm IssueFrontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return nil, fmt.Errorf("frontmatter: invalid YAML: %w", err)
	}

	// Determine the body after the closing delimiter.
	remaining := afterOpen[closeIdx+len(frontmatterDelimiter):]
	// Strip up to one newline ending the delimiter line, then one optional blank line.
	remaining = strings.TrimPrefix(remaining, "\n")
	remaining = strings.TrimPrefix(remaining, "\n")

	return &ParseResult{Frontmatter: &fm, Body: remaining}, nil
}

// RenderFrontmatter produces an issue body with YAML frontmatter followed by markdown.
func RenderFrontmatter(fm *IssueFrontmatter, markdown string) string {
	data, err := yaml.Marshal(fm)
	if err != nil {
		// IssueFrontmatter contains only basic types; marshal should never fail.
		panic(fmt.Sprintf("frontmatter: failed to marshal: %v", err))
	}

	var b strings.Builder
	b.WriteString(frontmatterDelimiter)
	b.WriteByte('\n')
	b.Write(data)
	b.WriteString(frontmatterDelimiter)
	b.WriteByte('\n')
	if markdown != "" {
		b.WriteByte('\n')
		b.WriteString(markdown)
	}
	return b.String()
}
