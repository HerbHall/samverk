package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/herbhall/samverk/internal/forge"
)

// searchIssuesInput is the typed input for the search_issues tool.
type searchIssuesInput struct {
	Query  string   `json:"query" jsonschema:"required,search string to match against issue titles and bodies"`
	State  string   `json:"state,omitempty" jsonschema:"filter by state: open, closed, or all (default: open)"`
	Labels []string `json:"labels,omitempty" jsonschema:"optional additional label filter"`
}

// searchPRsInput is the typed input for the search_prs tool.
type searchPRsInput struct {
	Query string `json:"query" jsonschema:"required,search string to match against pull request titles and bodies"`
	State string `json:"state,omitempty" jsonschema:"filter by state: open, closed, or all (default: open)"`
}

// searchResultEntry is a single entry in the search results.
type searchResultEntry struct {
	Number  int      `json:"number"`
	Title   string   `json:"title"`
	State   string   `json:"state"`
	Labels  []string `json:"labels,omitempty"`
	Snippet string   `json:"snippet"`
}

// registerSearchTools adds search MCP tools to the server.
func registerSearchTools(srv *gosdk.Server, h *Handler) {
	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "search_issues",
		Description: "Full-text search across issue titles and bodies. Returns matching issues with snippets showing where the query matched.",
	}, h.handleSearchIssues)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "search_prs",
		Description: "Full-text search across pull request titles and bodies. Returns matching PRs with snippets showing where the query matched.",
	}, h.handleSearchPRs)
}

// handleSearchIssues searches issues by text query.
func (h *Handler) handleSearchIssues(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input searchIssuesInput,
) (*gosdk.CallToolResult, any, error) {
	if strings.TrimSpace(input.Query) == "" {
		return nil, nil, fmt.Errorf("query must not be empty")
	}

	state := forge.StateOpen
	switch forge.State(input.State) {
	case forge.StateClosed:
		state = forge.StateClosed
	case "all":
		state = ""
	case forge.StateOpen, "":
		state = forge.StateOpen
	}

	isPR := false
	opts := &forge.SearchOptions{
		Query:         input.Query,
		State:         state,
		Labels:        input.Labels,
		IsPullRequest: &isPR,
	}

	issues, err := h.activeTracker().SearchIssues(ctx, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("searching issues: %w", err)
	}

	entries := buildSearchEntries(issues, input.Query)

	result, err := json.Marshal(entries)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling search results: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// handleSearchPRs searches pull requests by text query.
func (h *Handler) handleSearchPRs(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input searchPRsInput,
) (*gosdk.CallToolResult, any, error) {
	if strings.TrimSpace(input.Query) == "" {
		return nil, nil, fmt.Errorf("query must not be empty")
	}

	state := forge.StateOpen
	switch forge.State(input.State) {
	case forge.StateClosed:
		state = forge.StateClosed
	case "all":
		state = ""
	case forge.StateOpen, "":
		state = forge.StateOpen
	}

	isPR := true
	opts := &forge.SearchOptions{
		Query:         input.Query,
		State:         state,
		IsPullRequest: &isPR,
	}

	issues, err := h.activeTracker().SearchIssues(ctx, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("searching pull requests: %w", err)
	}

	entries := buildSearchEntries(issues, input.Query)

	result, err := json.Marshal(entries)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling search results: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// buildSearchEntries converts forge issues into search result entries with
// snippets showing where the query matched.
func buildSearchEntries(issues []*forge.Issue, query string) []searchResultEntry {
	entries := make([]searchResultEntry, 0, len(issues))
	lowerQuery := strings.ToLower(query)

	for _, iss := range issues {
		entry := searchResultEntry{
			Number: iss.Number,
			Title:  iss.Title,
			State:  string(iss.State),
			Labels: iss.Labels,
		}

		entry.Snippet = extractSnippet(iss.Title, iss.Body, lowerQuery)
		entries = append(entries, entry)
	}

	return entries
}

// extractSnippet returns a short text snippet showing where the query matched.
// It prefers a body match with surrounding context; falls back to the title.
func extractSnippet(title, body, lowerQuery string) string {
	if idx := strings.Index(strings.ToLower(body), lowerQuery); idx >= 0 {
		return snippetAround(body, idx, len(lowerQuery))
	}
	if idx := strings.Index(strings.ToLower(title), lowerQuery); idx >= 0 {
		return snippetAround(title, idx, len(lowerQuery))
	}
	// Forge returned this as a match but our simple search didn't find it
	// (e.g. the forge tokenizes differently). Return the title as the snippet.
	if len(title) > 120 {
		return title[:120] + "..."
	}
	return title
}

// snippetAround extracts a snippet of approximately 120 characters centered
// on the match at position idx with length matchLen.
func snippetAround(text string, idx, matchLen int) string {
	const contextLen = 60
	start := idx - contextLen
	if start < 0 {
		start = 0
	}
	end := idx + matchLen + contextLen
	if end > len(text) {
		end = len(text)
	}

	snippet := text[start:end]
	// Clean up newlines for readability.
	snippet = strings.ReplaceAll(snippet, "\r\n", " ")
	snippet = strings.ReplaceAll(snippet, "\n", " ")

	prefix := ""
	suffix := ""
	if start > 0 {
		prefix = "..."
	}
	if end < len(text) {
		suffix = "..."
	}

	return prefix + snippet + suffix
}
