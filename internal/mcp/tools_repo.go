package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/herbhall/samverk/internal/forge"
)

// listFilesInput is the typed input for the list_files tool.
type listFilesInput struct {
	Path string `json:"path,omitempty" jsonschema:"repository path to list (default: root)"`
	Ref  string `json:"ref,omitempty" jsonschema:"git ref: branch, tag, or commit SHA (default: HEAD)"`
}

// readFileInput is the typed input for the read_file tool.
type readFileInput struct {
	Path      string `json:"path" jsonschema:"required,file path to read"`
	Ref       string `json:"ref,omitempty" jsonschema:"git ref: branch, tag, or commit SHA (default: HEAD)"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"first line to return (1-based, inclusive)"`
	EndLine   int    `json:"end_line,omitempty" jsonschema:"last line to return (1-based, inclusive)"`
}

// getDiffInput is the typed input for the get_diff tool.
type getDiffInput struct {
	Base     string `json:"base" jsonschema:"required,base ref (branch, tag, or commit SHA)"`
	Head     string `json:"head" jsonschema:"required,head ref (branch, tag, or commit SHA)"`
	MaxBytes int    `json:"max_bytes,omitempty" jsonschema:"max response bytes before truncation (default 32768)"`
	Paths    string `json:"paths,omitempty" jsonschema:"comma-separated file glob filters (e.g. internal/provider/**)"`
	StatOnly bool   `json:"stat_only,omitempty" jsonschema:"return only file change summary without line content"`
}

const defaultMaxDiffBytes = 32768

// listBranchesInput is the typed input for the list_branches tool.
type listBranchesInput struct{}

// getCommitLogInput is the typed input for the get_commit_log tool.
type getCommitLogInput struct {
	Branch string `json:"branch,omitempty" jsonschema:"branch name (default: main)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max number of commits to return (default: 20)"`
}

// searchCodeInput is the typed input for the search_code tool.
type searchCodeInput struct {
	Query string `json:"query" jsonschema:"required,code search query"`
}

// registerRepoTools adds read-only repository browsing tools to the MCP server.
func registerRepoTools(srv *gosdk.Server, h *Handler) {
	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "list_files",
		Description: "List files and directories at a path in the repository",
	}, h.handleListFiles)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "read_file",
		Description: "Read the contents of a file from the repository",
	}, h.handleReadFile)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "get_diff",
		Description: "Get the diff between two refs (branches, tags, or commits). Supports max_bytes truncation, paths filtering, and stat_only mode.",
	}, h.handleGetDiff)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "list_branches",
		Description: "List all branches in the repository",
	}, h.handleListBranches)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "get_commit_log",
		Description: "Get recent commits on a branch",
	}, h.handleGetCommitLog)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "search_code",
		Description: "Search for code in the repository",
	}, h.handleSearchCode)
}

// handleListFiles lists files at a given path in the repository.
func (h *Handler) handleListFiles(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input listFilesInput,
) (*gosdk.CallToolResult, any, error) {
	if h.activeReader() == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "repo operations not available: no RepoReader configured"},
			},
		}, nil, nil
	}

	entries, err := h.activeReader().ListFiles(ctx, input.Path, input.Ref)
	if err != nil {
		return nil, nil, fmt.Errorf("listing files: %w", err)
	}

	result, err := json.Marshal(entries)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling file entries: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// handleReadFile reads a file from the repository with optional line range.
func (h *Handler) handleReadFile(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input readFileInput,
) (*gosdk.CallToolResult, any, error) {
	if h.activeReader() == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "repo operations not available: no RepoReader configured"},
			},
		}, nil, nil
	}

	if input.Path == "" {
		return nil, nil, fmt.Errorf("path must not be empty")
	}

	data, err := h.activeReader().ReadFile(ctx, input.Path, input.Ref)
	if err != nil {
		return nil, nil, fmt.Errorf("reading file: %w", err)
	}

	content := string(data)

	// Apply line range if specified.
	if input.StartLine > 0 || input.EndLine > 0 {
		lines := strings.Split(content, "\n")
		start := input.StartLine
		end := input.EndLine

		if start <= 0 {
			start = 1
		}
		if end <= 0 || end > len(lines) {
			end = len(lines)
		}
		if start > len(lines) {
			start = len(lines)
		}
		if start > end {
			start = end
		}

		// Convert to 0-based indexing.
		content = strings.Join(lines[start-1:end], "\n")
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: content},
		},
	}, nil, nil
}

// handleGetDiff returns the diff between two refs with optional filtering and truncation.
func (h *Handler) handleGetDiff(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input getDiffInput,
) (*gosdk.CallToolResult, any, error) {
	if h.activeReader() == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "repo operations not available: no RepoReader configured"},
			},
		}, nil, nil
	}

	if input.Base == "" {
		return nil, nil, fmt.Errorf("base must not be empty")
	}
	if input.Head == "" {
		return nil, nil, fmt.Errorf("head must not be empty")
	}

	diff, err := h.activeReader().GetDiff(ctx, input.Base, input.Head)
	if err != nil {
		return nil, nil, fmt.Errorf("getting diff: %w", err)
	}

	// Apply path filtering if requested.
	if input.Paths != "" {
		diff = filterDiffByPaths(diff, input.Paths)
	}

	// Apply stat-only mode if requested.
	if input.StatOnly {
		diff = extractDiffStats(diff)
	}

	// Apply max_bytes truncation.
	maxBytes := input.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxDiffBytes
	}
	diff = truncateDiff(diff, maxBytes)

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: diff},
		},
	}, nil, nil
}

// filterDiffByPaths filters a unified diff to only include files matching
// the comma-separated glob patterns. Each pattern is matched against the
// file path using filepath.Match semantics.
func filterDiffByPaths(diff, paths string) string {
	patterns := strings.Split(paths, ",")
	for i := range patterns {
		patterns[i] = strings.TrimSpace(patterns[i])
	}

	sections := splitDiffSections(diff)
	var b strings.Builder

	// Keep header lines (before first diff section).
	if len(sections) > 0 && sections[0].file == "" {
		b.WriteString(sections[0].content)
		sections = sections[1:]
	}

	for _, sec := range sections {
		if matchesAnyPattern(sec.file, patterns) {
			b.WriteString(sec.content)
		}
	}

	result := b.String()
	if result == "" {
		return fmt.Sprintf("No files matched path filter: %s", paths)
	}
	return result
}

// diffSection represents a file section within a unified diff.
type diffSection struct {
	file    string
	content string
}

// splitDiffSections splits a unified diff into per-file sections.
// Each section starts with a line beginning with "diff " or "--- ".
func splitDiffSections(diff string) []diffSection {
	lines := strings.Split(diff, "\n")
	var sections []diffSection
	var current strings.Builder
	currentFile := ""

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			// Save previous section.
			if current.Len() > 0 {
				sections = append(sections, diffSection{file: currentFile, content: current.String()})
				current.Reset()
			}
			currentFile = extractFilePath(line)
		} else if strings.HasPrefix(line, "--- ") && currentFile == "" {
			// Alternate diff format without "diff --git" header.
			if current.Len() > 0 {
				sections = append(sections, diffSection{file: currentFile, content: current.String()})
				current.Reset()
			}
			currentFile = extractFileFromDashLine(line)
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}

	if current.Len() > 0 {
		sections = append(sections, diffSection{file: currentFile, content: current.String()})
	}

	return sections
}

// extractFilePath extracts the file path from a "diff --git a/path b/path" line.
func extractFilePath(line string) string {
	// Format: "diff --git a/path/to/file b/path/to/file"
	parts := strings.SplitN(line, " b/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return line
}

// extractFileFromDashLine extracts the file path from a "--- a/path" line.
func extractFileFromDashLine(line string) string {
	trimmed := strings.TrimPrefix(line, "--- ")
	trimmed = strings.TrimPrefix(trimmed, "a/")
	return trimmed
}

// matchesAnyPattern checks if the file path matches any of the glob patterns.
// Supports ** for recursive matching by splitting on path separators.
func matchesAnyPattern(file string, patterns []string) bool {
	for _, pat := range patterns {
		if pat == "" {
			continue
		}
		// Handle ** patterns by converting to simple prefix match.
		if strings.Contains(pat, "**") {
			prefix := strings.Split(pat, "**")[0]
			if strings.HasPrefix(file, prefix) {
				return true
			}
			continue
		}
		if matched, _ := filepath.Match(pat, file); matched {
			return true
		}
		// Also try matching just the filename.
		if matched, _ := filepath.Match(pat, filepath.Base(file)); matched {
			return true
		}
	}
	return false
}

// extractDiffStats parses a unified diff and returns only file-level statistics.
func extractDiffStats(diff string) string {
	sections := splitDiffSections(diff)
	var b strings.Builder
	totalAdded, totalRemoved, fileCount := 0, 0, 0

	// Keep header lines.
	if len(sections) > 0 && sections[0].file == "" {
		b.WriteString(sections[0].content)
		sections = sections[1:]
	}

	for _, sec := range sections {
		if sec.file == "" {
			continue
		}
		fileCount++
		added, removed := countChanges(sec.content)
		totalAdded += added
		totalRemoved += removed
		fmt.Fprintf(&b, "%s | +%d -%d\n", sec.file, added, removed)
	}

	fmt.Fprintf(&b, "\n%d files changed, %d insertions(+), %d deletions(-)\n",
		fileCount, totalAdded, totalRemoved)

	return b.String()
}

// countChanges counts added and removed lines in a diff section.
func countChanges(section string) (added, removed int) {
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	return added, removed
}

// truncateDiff truncates diff content to maxBytes, appending a marker if truncated.
func truncateDiff(diff string, maxBytes int) string {
	if len(diff) <= maxBytes {
		return diff
	}
	totalBytes := len(diff)
	truncated := diff[:maxBytes]
	// Try to truncate at a line boundary.
	if idx := strings.LastIndex(truncated, "\n"); idx > 0 {
		truncated = truncated[:idx+1]
	}
	return fmt.Sprintf("%s\n... [truncated, %d bytes total]", truncated, totalBytes)
}

// handleListBranches lists all branches in the repository.
func (h *Handler) handleListBranches(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	_ listBranchesInput,
) (*gosdk.CallToolResult, any, error) {
	if h.activeReader() == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "repo operations not available: no RepoReader configured"},
			},
		}, nil, nil
	}

	branches, err := h.activeReader().ListBranches(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("listing branches: %w", err)
	}

	result, err := json.Marshal(branches)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling branches: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// handleGetCommitLog returns recent commits on a branch.
func (h *Handler) handleGetCommitLog(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input getCommitLogInput,
) (*gosdk.CallToolResult, any, error) {
	if h.activeReader() == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "repo operations not available: no RepoReader configured"},
			},
		}, nil, nil
	}

	branch := input.Branch
	if branch == "" {
		branch = "main"
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}

	commits, err := h.activeReader().GetCommitLog(ctx, branch, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("getting commit log: %w", err)
	}

	result, err := json.Marshal(commits)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling commits: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// handleSearchCode searches for code in the repository.
func (h *Handler) handleSearchCode(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input searchCodeInput,
) (*gosdk.CallToolResult, any, error) {
	if h.activeReader() == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "repo operations not available: no RepoReader configured"},
			},
		}, nil, nil
	}

	if input.Query == "" {
		return nil, nil, fmt.Errorf("query must not be empty")
	}

	results, err := h.activeReader().SearchCode(ctx, input.Query)
	if err != nil {
		if errors.Is(err, forge.ErrNotSupported) {
			return &gosdk.CallToolResult{
				Content: []gosdk.Content{
					&gosdk.TextContent{Text: "search_code is not available on this forge platform (Gitea does not expose repo-scoped code search via its SDK)"},
				},
			}, nil, nil
		}
		return nil, nil, fmt.Errorf("searching code: %w", err)
	}

	result, err := json.Marshal(results)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling search results: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}
