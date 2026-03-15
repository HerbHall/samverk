package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/herbhall/samverk/internal/synapset"
)

// Project type constants for CI checklist selection.
const (
	ProjectTypeGo       = "go"
	ProjectTypeFrontend = "frontend"
	ProjectTypeFullstack = "fullstack"
)

// corePrinciples is the unconditional rules block included in every agent CLAUDE.md.
const corePrinciples = `## Core Principles -- UNCONDITIONAL

These rules cannot be overridden by any learning, optimization, or time pressure:
1. Once found, always fix, never leave. Never classify errors as "pre-existing."
2. Build, test, and lint must pass before any commit. No exceptions.
3. Never force-push main, skip hooks, commit secrets, or use --no-verify.
4. Never mark work as complete when it is not. Never hide errors.
5. You own every error you find, regardless of who introduced it.`

// goCIChecklist is the Go agent CI checklist extracted from DevKit subagent-ci-checklist.md.
const goCIChecklist = `## Pre-Commit CI Checklist (MUST verify before finishing)

Run these checks and fix any errors:

1. ` + "`go build ./...`" + ` -- Compilation
2. ` + "`go test ./...`" + ` -- Tests (skip -race on Windows MSYS)
3. ` + "`GOOS=linux GOARCH=amd64 go build ./...`" + ` -- Cross-compile check
4. Self-check your code for these MANDATORY lint patterns before finishing:
   - ` + "`for _, v := range slice`" + ` where v is a struct > 64 bytes -> use ` + "`for i := range slice`" + ` with ` + "`slice[i]`" + `
   - ` + "`var result []T`" + ` inside a loop -> use ` + "`make([]T, 0, len(source))`" + ` to preallocate
   - Two consecutive ` + "`append()`" + ` to same slice -> combine into one call
   - Functions returning multiple values -> use named returns, change ` + "`:=`" + ` to ` + "`=`" + `
5. If you added/modified HTTP handlers with swagger annotations:
   ` + "`go run github.com/swaggo/swag/cmd/swag@v1.16.4 init -g cmd/<app>/main.go -o api/swagger --parseDependency --parseInternal`" + `

Common Go CI failures to watch for:
- gosec G101: Constants near credential code get flagged. Add ` + "`//nolint:gosec // G101: <reason>`" + `
- gocritic unnamedResult: Functions returning multiple values need named returns.
- gocritic appendCombine: Two consecutive ` + "`append()`" + ` must be combined.
- gocritic rangeValCopy: Use ` + "`for i := range slice`" + ` with ` + "`slice[i]`" + ` for large structs.
- bodyclose: Always close ` + "`*http.Response`" + ` body.
- exhaustive: Switch on enum types MUST list ALL cases, even with a default return.
- prealloc: ` + "`var slice []T`" + ` in a loop body should be ` + "`make([]T, 0, len(source))`" + `.`

// frontendCIChecklist is the frontend agent CI checklist.
const frontendCIChecklist = `## Pre-Commit CI Checklist (MUST verify before finishing)

Run these checks and fix any errors:

1. If you added/removed dependencies in package.json: ` + "`cd web && pnpm install`" + ` to sync pnpm-lock.yaml
2. ` + "`npx tsc --noEmit`" + ` -- TypeScript compilation
3. ` + "`npx eslint src/<your-files>`" + ` -- Lint check

Common frontend CI failures to watch for:
- pnpm-lock.yaml drift: CI uses ` + "`--frozen-lockfile`" + `. If you added deps, ` + "`pnpm install`" + ` MUST run.
- Recharts Tooltip: Use ` + "`Partial<TooltipContentProps<number, string>>`" + ` for props.
- JSX short-circuit with unknown type: Use ` + "`unknownVar != null &&`" + ` instead of bare ` + "`&&`" + `.
- Unused imports: ESLint catches imports TypeScript does not flag. Verify every named import is referenced.
- shadcn/ui imports: Verify component names match what is actually exported.`

// fullstackCIChecklist is the combined backend+frontend CI checklist.
const fullstackCIChecklist = `## Pre-Commit CI Checklist (MUST verify before finishing)

Backend:
1. ` + "`go build ./...`" + ` && ` + "`go test ./...`" + `
2. If HTTP handlers added: ` + "`go run github.com/swaggo/swag/cmd/swag@v1.16.4 init -g cmd/<app>/main.go -o api/swagger --parseDependency --parseInternal`" + `
3. Self-check: ` + "`for _, v := range`" + ` on large structs -> index-based; ` + "`var []T`" + ` -> ` + "`make([]T, 0, cap)`" + `; consecutive appends -> combine; unnamed multi-returns -> name them
4. Watch for: gosec G101, gocritic unnamedResult, gocritic appendCombine, bodyclose, exhaustive

Frontend:
1. If deps changed: ` + "`cd web && pnpm install`" + ` to sync lockfile
2. ` + "`cd web && npx tsc --noEmit`" + ` && ` + "`npx eslint src/<your-files>`" + `
3. Recharts Tooltip: use ` + "`Partial<TooltipContentProps<...>>`" + ` with JSX element form
4. JSX unknown type: use ` + "`!= null`" + ` check, not bare ` + "`&&`" + `
5. Verify all imports are used (ESLint catches what tsc misses)`

// gitSafetyRules is the git workflow block for CLI agents operating in worktrees.
const gitSafetyRules = `## Git Workflow

You are running in an isolated git worktree on a dedicated branch.

- Do NOT commit directly to main. Your worktree is already on an agent branch.
- Make your changes, then let the runner handle commit and push.
- If you need to run git commands, stay on the current branch.
- Never force-push, skip hooks, or use --no-verify.
- Never commit secrets, credentials, or API keys.`

// knownGotchas is a curated subset of high-frequency gotchas for agents.
const knownGotchas = `## Known Gotchas

- gosec G101: Constants near credential code get flagged as hardcoded credentials. Add nolint annotation.
- gocritic rangeValCopy: Iterating large structs by value copies them. Use index-based access.
- Build-tag files (!windows): Lint errors in platform-specific files only appear in Linux CI.
- go:embed cache: Go build cache does not detect changes to embedded files. Use make build.
- Swagger drift: Any change to Go types in swagger-annotated handlers requires regenerating the spec.
- context.WithTimeout: Cancels ALL downstream operations including cleanup. Use separate cleanup context.`

// DetectProjectType infers the project type from issue labels. Labels are
// checked for frontend/fullstack indicators. Falls back to Go (the primary
// language of Samverk).
func DetectProjectType(labels []string) string {
	hasFrontend := false
	hasBackend := false

	for _, label := range labels {
		lower := strings.ToLower(label)
		switch {
		case strings.Contains(lower, "frontend") || strings.Contains(lower, "web") || strings.Contains(lower, "ui"):
			hasFrontend = true
		case strings.Contains(lower, "backend") || strings.Contains(lower, "api") || strings.Contains(lower, "server"):
			hasBackend = true
		case strings.Contains(lower, "fullstack") || strings.Contains(lower, "full-stack"):
			return ProjectTypeFullstack
		}
	}

	if hasFrontend && hasBackend {
		return ProjectTypeFullstack
	}
	if hasFrontend {
		return ProjectTypeFrontend
	}
	return ProjectTypeGo
}

// mcpConfig is the JSON config for CLI agent MCP servers.
var mcpConfig = map[string]interface{}{
	"mcpServers": map[string]interface{}{
		"samverk": map[string]interface{}{
			"type": "http",
			"url":  "https://samverk.herbhall.net/mcp",
		},
		"synapset": map[string]interface{}{
			"type": "http",
			"url":  "https://synapset.herbhall.net/mcp",
		},
	},
}

// GenerateAgentCLAUDEMD builds a per-worktree CLAUDE.md for agents. It includes
// core principles, git safety rules, a CI checklist selected by project type,
// known gotchas, and the issue body as task context.
func GenerateAgentCLAUDEMD(projectType, issueBody string) string {
	var b strings.Builder

	b.WriteString("# Agent Task\n\n")
	b.WriteString(corePrinciples)
	b.WriteString("\n\n")
	b.WriteString(gitSafetyRules)
	b.WriteString("\n\n")

	switch projectType {
	case ProjectTypeFrontend:
		b.WriteString(frontendCIChecklist)
	case ProjectTypeFullstack:
		b.WriteString(fullstackCIChecklist)
	default: // "go" and any unrecognized type default to Go checklist
		b.WriteString(goCIChecklist)
	}

	b.WriteString("\n\n")
	b.WriteString(knownGotchas)

	if issueBody != "" {
		b.WriteString("\n\n## Task Context\n\n")
		b.WriteString(issueBody)
	}

	b.WriteString("\n")
	return b.String()
}

// WriteMCPConfig writes a .claude/.mcp.json file into the given worktree
// directory, configuring Samverk and Synapset MCP endpoints for CLI agents.
func WriteMCPConfig(worktreeDir string) error {
	dir := filepath.Join(worktreeDir, ".claude")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}

	data, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mcp config: %w", err)
	}

	path := filepath.Join(dir, ".mcp.json")
	if err = os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write mcp config: %w", err)
	}

	return nil
}

// FetchRelevantPatterns queries Synapset for patterns relevant to the given
// query string. Returns up to maxResults pattern descriptions. Returns an
// empty slice (not error) if the client is nil or Synapset is unavailable.
func FetchRelevantPatterns(ctx context.Context, client *synapset.Client, query string, maxResults int) ([]string, error) {
	if client == nil || query == "" {
		return nil, nil
	}

	memories, err := client.SearchMemory(ctx, "devkit", query, maxResults)
	if err != nil {
		// Synapset unavailable -- degrade gracefully.
		return nil, nil //nolint:nilerr // graceful degradation when Synapset is unavailable
	}

	if len(memories) == 0 {
		return nil, nil
	}

	results := make([]string, 0, len(memories))
	for i := range memories {
		if memories[i].Content != "" {
			results = append(results, memories[i].Content)
		}
	}

	return results, nil
}
