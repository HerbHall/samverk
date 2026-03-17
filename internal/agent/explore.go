package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// Explore phase constants for context budget management.
const (
	// maxExploreBytes is the total cap for all explored file content (64KB).
	maxExploreBytes = 64 * 1024
	// maxSingleFileBytes caps any individual file at 8KB to prevent one large
	// file from consuming the entire explore budget.
	maxSingleFileBytes = 8 * 1024
)

// ExploreContext reads project context files from the repo to enrich the
// agent's understanding before code generation. It reads CLAUDE.md from the
// repo root, extracts file paths from the issue body, and discovers sibling
// .go files in the same packages as referenced files.
//
// The returned map is keyed by relative path with file content as values.
// Total content is capped at maxExploreBytes (64KB) to avoid prompt bloat.
func ExploreContext(repoDir, issueBody string) map[string]string {
	if repoDir == "" {
		return nil
	}

	result := make(map[string]string)
	var totalBytes int

	// Phase 1: Read CLAUDE.md from repo root (up to maxSingleFileBytes).
	claudeMD := readFileCapped(filepath.Join(repoDir, "CLAUDE.md"), maxSingleFileBytes)
	if claudeMD != "" {
		result["CLAUDE.md"] = claudeMD
		totalBytes += len(claudeMD)
	}

	// Phase 2: Extract referenced file paths from issue body.
	matches := filePathRe.FindAllStringSubmatch(issueBody, -1)
	if len(matches) == 0 {
		return result
	}

	// Deduplicate and collect referenced paths.
	seen := make(map[string]bool, len(matches))
	var referencedPaths []string
	for _, m := range matches {
		p := strings.TrimSpace(m[1])
		if !seen[p] {
			seen[p] = true
			referencedPaths = append(referencedPaths, p)
		}
	}

	// Phase 3: Discover sibling .go files in the same directories.
	siblingDirs := make(map[string]bool, len(referencedPaths))
	for _, p := range referencedPaths {
		dir := filepath.Dir(p)
		if dir != "." && !siblingDirs[dir] {
			siblingDirs[dir] = true
		}
	}

	// Collect all paths: referenced files first, then siblings.
	allPaths := make([]string, 0, len(referencedPaths)*3)
	allPaths = append(allPaths, referencedPaths...)

	for dir := range siblingDirs {
		siblings := discoverGoSiblings(repoDir, dir, seen)
		allPaths = append(allPaths, siblings...)
		for _, s := range siblings {
			seen[s] = true
		}
	}

	// Phase 4: Read files up to the total budget.
	for _, p := range allPaths {
		if _, exists := result[p]; exists {
			continue // already in result (e.g., CLAUDE.md matched by regex)
		}
		if totalBytes >= maxExploreBytes {
			break
		}

		content := readFileCapped(filepath.Join(repoDir, p), maxSingleFileBytes)
		if content == "" {
			continue
		}

		if totalBytes+len(content) > maxExploreBytes {
			// Truncate to fit remaining budget.
			remaining := maxExploreBytes - totalBytes
			if remaining <= 0 {
				break
			}
			content = content[:remaining]
		}

		result[p] = content
		totalBytes += len(content)
	}

	return result
}

// ExploreFileList returns just the list of explored file paths without reading
// content. This is used for CLI-based agents where the agent reads files itself.
func ExploreFileList(repoDir, issueBody string) []string {
	if repoDir == "" {
		return nil
	}

	var paths []string

	// Check for CLAUDE.md.
	if _, err := os.Stat(filepath.Join(repoDir, "CLAUDE.md")); err == nil {
		paths = append(paths, "CLAUDE.md")
	}

	// Extract referenced file paths from issue body.
	matches := filePathRe.FindAllStringSubmatch(issueBody, -1)
	seen := map[string]bool{"CLAUDE.md": true}
	var referencedPaths []string
	for _, m := range matches {
		p := strings.TrimSpace(m[1])
		if !seen[p] {
			seen[p] = true
			referencedPaths = append(referencedPaths, p)
		}
	}
	paths = append(paths, referencedPaths...)

	// Discover sibling .go files.
	siblingDirs := make(map[string]bool, len(referencedPaths))
	for _, p := range referencedPaths {
		dir := filepath.Dir(p)
		if dir != "." && !siblingDirs[dir] {
			siblingDirs[dir] = true
		}
	}
	for dir := range siblingDirs {
		siblings := discoverGoSiblings(repoDir, dir, seen)
		paths = append(paths, siblings...)
		for _, s := range siblings {
			seen[s] = true
		}
	}

	return paths
}

// discoverGoSiblings finds .go files in the given directory (relative to
// repoDir) that are not already in the seen set. Returns relative paths.
func discoverGoSiblings(repoDir, relDir string, seen map[string]bool) []string {
	absDir := filepath.Join(repoDir, relDir)
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil
	}

	var siblings []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		relPath := filepath.Join(relDir, name)
		// Normalize to forward slashes for consistency with issue body paths.
		relPath = filepath.ToSlash(relPath)
		if seen[relPath] {
			continue
		}
		siblings = append(siblings, relPath)
	}
	return siblings
}

// readFileCapped reads a file up to maxBytes. Returns empty string on error
// or if the file does not exist.
func readFileCapped(path string, maxBytes int) string {
	f, err := os.Open(path) //nolint:gosec // G304: paths are constructed from repo-relative paths
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, maxBytes)
	n, _ := f.Read(buf)
	if n == 0 {
		return ""
	}
	return string(buf[:n])
}
