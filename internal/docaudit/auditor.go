package docaudit

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Severity indicates the importance of a finding.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Finding represents a single documentation drift issue.
type Finding struct {
	File     string
	Line     int
	Severity Severity
	Message  string
}

// Auditor checks for documentation drift in a repository.
type Auditor struct {
	repoRoot     string
	staleDays    int
	requiredDocs []string
}

// New creates an Auditor rooted at the given repository path.
func New(repoRoot string) *Auditor {
	return &Auditor{
		repoRoot:  repoRoot,
		staleDays: 7,
		requiredDocs: []string{
			"docs/architecture.md",
			"docs/tech-stack.md",
			".samverk/status.md",
		},
	}
}

// Run executes all documentation checks and returns findings.
func (a *Auditor) Run() ([]Finding, error) {
	var findings []Finding

	staleFindings, err := a.checkStaleStatus()
	if err != nil {
		return nil, fmt.Errorf("check stale status: %w", err)
	}
	findings = append(findings, staleFindings...)

	missingRefFindings, err := a.checkMissingReferences()
	if err != nil {
		return nil, fmt.Errorf("check missing references: %w", err)
	}
	findings = append(findings, missingRefFindings...)

	missingDocFindings := a.checkRequiredDocs()
	findings = append(findings, missingDocFindings...)

	return findings, nil
}

// checkStaleStatus checks whether .samverk/status.md has a stale frontmatter
// updated field (older than staleDays).
func (a *Auditor) checkStaleStatus() ([]Finding, error) {
	statusPath := filepath.Join(a.repoRoot, ".samverk", "status.md")

	f, err := os.Open(statusPath) //nolint:gosec // G304: path is constructed from trusted repoRoot
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // handled by checkRequiredDocs
		}
		return nil, fmt.Errorf("open status.md: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if lineNum == 1 && strings.TrimSpace(line) == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter && strings.TrimSpace(line) == "---" {
			break
		}
		if !inFrontmatter {
			break
		}

		if strings.HasPrefix(line, "updated:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "updated:"))
			t, parseErr := time.Parse(time.RFC3339, value)
			if parseErr != nil {
				return []Finding{{
					File:     ".samverk/status.md",
					Line:     lineNum,
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("could not parse updated timestamp: %s", value),
				}}, nil
			}

			age := time.Since(t)
			if age > time.Duration(a.staleDays)*24*time.Hour {
				return []Finding{{
					File:     ".samverk/status.md",
					Line:     lineNum,
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("status.md is %d days old (threshold: %d days)", int(age.Hours()/24), a.staleDays),
				}}, nil
			}
		}
	}

	return nil, scanner.Err()
}

// mdLinkRe matches markdown links like [text](path).
var mdLinkRe = regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)

// checkMissingReferences scans docs/*.md for relative file links and checks
// that each referenced file exists.
func (a *Auditor) checkMissingReferences() ([]Finding, error) {
	docsDir := filepath.Join(a.repoRoot, "docs")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read docs dir: %w", err)
	}

	var findings []Finding

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(docsDir, entry.Name())
		relPath := filepath.Join("docs", entry.Name())

		ff, err := a.checkFileReferences(filePath, relPath)
		if err != nil {
			return nil, fmt.Errorf("check references in %s: %w", relPath, err)
		}
		findings = append(findings, ff...)
	}

	return findings, nil
}

func (a *Auditor) checkFileReferences(absPath, relPath string) ([]Finding, error) {
	f, err := os.Open(absPath) //nolint:gosec // G304: path is constructed from trusted repoRoot + docs dir scan
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var findings []Finding
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		matches := mdLinkRe.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			linkTarget := m[2]

			// Skip external links, anchors, and absolute paths.
			if strings.HasPrefix(linkTarget, "http://") ||
				strings.HasPrefix(linkTarget, "https://") ||
				strings.HasPrefix(linkTarget, "#") ||
				strings.HasPrefix(linkTarget, "/") ||
				strings.HasPrefix(linkTarget, "mailto:") {
				continue
			}

			// Strip anchor fragment from relative links.
			if idx := strings.Index(linkTarget, "#"); idx >= 0 {
				linkTarget = linkTarget[:idx]
			}
			if linkTarget == "" {
				continue
			}

			// Resolve relative to the file's directory.
			dir := filepath.Dir(absPath)
			targetAbs := filepath.Join(dir, linkTarget)

			if _, statErr := os.Stat(targetAbs); os.IsNotExist(statErr) { //nolint:gosec // G703: path is from markdown link resolution within repo
				findings = append(findings, Finding{
					File:     relPath,
					Line:     lineNum,
					Severity: SeverityError,
					Message:  fmt.Sprintf("referenced file does not exist: %s", linkTarget),
				})
			}
		}
	}

	return findings, scanner.Err()
}

// checkRequiredDocs verifies that required documentation files exist.
func (a *Auditor) checkRequiredDocs() []Finding {
	var findings []Finding

	for _, doc := range a.requiredDocs {
		absPath := filepath.Join(a.repoRoot, doc)
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			findings = append(findings, Finding{
				File:     doc,
				Line:     0,
				Severity: SeverityError,
				Message:  fmt.Sprintf("required document is missing: %s", doc),
			})
		}
	}

	return findings
}
