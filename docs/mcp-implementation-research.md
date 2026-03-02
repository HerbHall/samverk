# MCP Implementation Research

Research into Go MCP (Model Context Protocol) libraries for Samverk's MCP server implementation. The server must expose 22 MCP tools via Streamable HTTP transport at `POST /mcp`, integrated into the existing `net/http` server alongside REST API and embedded SPA routes.

## Requirements Summary

From [mcp-server.md](mcp-server.md):

- **Transport:** Streamable HTTP (stateless JSON-RPC 2.0 over HTTP POST)
- **Endpoint:** `POST /mcp` on the same port as REST API and SPA
- **Auth:** Bearer token in `Authorization` header (API keys in `.samverk/auth.yaml`)
- **Tools:** 22 tools across project management, issue ops, digest, and repo operations
- **Autonomy:** Server-side tier enforcement (Tier 1/2 execute, Tier 3 returns confirmation prompt)
- **Session:** Stateless (no persistent session state between requests)
- **Deployment:** Single binary (no sidecar process)

## Library Comparison

Two significant Go MCP libraries exist. All others on GitHub have fewer than 10 stars and are not production-viable.

### 1. `github.com/modelcontextprotocol/go-sdk` (Official SDK)

| Attribute | Value |
|-----------|-------|
| Maintainer | Anthropic / Google (official MCP project) |
| Stars | ~3,985 |
| License | Apache 2.0 (new) / MIT (legacy) |
| Latest | v1.4.0 (2026-02-27) |
| Go version | Go 1.24+ |
| First stable | v1.0.0 (2025-09-30) |
| Contributors | Google engineers (findleyr, jba), Anthropic (maciej-kisiel) |
| Direct deps | `golang-jwt/jwt`, `google/go-cmp`, `google/jsonschema-go`, `segmentio/encoding`, `yosida95/uritemplate`, `golang.org/x/oauth2`, `golang.org/x/tools` |
| MCP spec versions | 2024-11-05, 2025-03-26, 2025-06-18, 2025-11-25 |

**Key characteristics:**

- Official SDK maintained under the `modelcontextprotocol` GitHub org
- Type-safe tool registration via generics (`AddTool[In, Out]`)
- Input schemas inferred from Go struct tags (`jsonschema:"..."`)
- Input validation against schema before handler is called
- Structured output support with output schemas
- Built-in `StreamableHTTPHandler` that implements `http.Handler`
- Built-in `auth.RequireBearerToken` middleware returning `func(http.Handler) http.Handler`
- `TokenInfo` in context (scopes, expiration, user ID) for session hijacking prevention
- MCP-level middleware via `Server.AddReceivingMiddleware` / `Server.AddSendingMiddleware`
- Stateless mode via `StreamableHTTPOptions{Stateless: true}`
- JSON response mode via `StreamableHTTPOptions{JSONResponse: true}`
- DNS rebinding protection for localhost servers
- Session timeout management
- Conformance test suite run against the MCP spec
- OpenSSF Scorecard integrated (8.7 score)

### 2. `github.com/mark3labs/mcp-go` (Community)

| Attribute | Value |
|-----------|-------|
| Maintainer | Ed Zynda (mark3labs) + community |
| Stars | ~8,258 |
| License | MIT |
| Latest | v0.44.1 (2026-02-27) |
| Go version | Go 1.23+ |
| First release | 2024-11-27 |
| Contributors | ezynda3 (166 commits), pottekkat (23), cryo-zd (22) |
| Direct deps | `google/uuid`, `invopop/jsonschema`, `spf13/cast`, `stretchr/testify`, `yosida95/uritemplate` |
| MCP spec versions | Not versioned against spec |

**Key characteristics:**

- Community-built, predates the official SDK by ~5 months
- Higher star count due to first-mover advantage
- Tool registration via builder pattern (`mcp.NewTool("name", mcp.WithString(...)...)`)
- `StreamableHTTPServer` implements `http.Handler` for mux integration
- Auth via `WithHTTPContextFunc` callback (extract token from request, inject into context)
- Hook-based extensibility (BeforeAny, OnSuccess, OnError, per-method hooks)
- Tool handler middleware via `ToolHandlerMiddleware` type
- Session management (stateless, stateful, custom session ID managers)
- Per-session tools and tool filtering
- No built-in auth middleware (must implement manually)
- No built-in input schema validation against typed structs
- Still on v0.x (no v1.0 release)
- Acknowledged by the official SDK README as inspiration

## Feature-by-Feature Comparison

| Feature | go-sdk (official) | mcp-go (community) |
|---------|-------------------|---------------------|
| Streamable HTTP transport | Yes (`StreamableHTTPHandler`) | Yes (`StreamableHTTPServer`) |
| Implements `http.Handler` | Yes | Yes |
| Stateless mode | Yes (`Stateless: true`) | Yes (`WithStateLess(true)`) |
| JSON response (no SSE) | Yes (`JSONResponse: true`) | Not documented |
| Type-safe tool handlers | Yes (generics: `AddTool[In, Out]`) | No (untyped `map[string]any`) |
| Input schema from structs | Yes (`jsonschema` struct tags) | No (manual builder) |
| Input validation | Yes (automatic) | No (manual) |
| Output schema support | Yes (structured output) | No |
| Built-in Bearer auth | Yes (`auth.RequireBearerToken`) | No (manual via context func) |
| Token info in context | Yes (`auth.TokenInfoFromContext`) | No (custom context values) |
| MCP-level middleware | Yes (`AddReceivingMiddleware`) | Yes (hooks + `ToolHandlerMiddleware`) |
| HTTP-level middleware | Standard `http.Handler` wrapping | Via `WithHTTPContextFunc` |
| DNS rebinding protection | Yes (default on) | No |
| Session timeout | Yes (`SessionTimeout` option) | Yes (`WithSessionIdleTTL`) |
| Conformance tests | Yes (full MCP spec suite) | No |
| Go version | 1.24+ | 1.23+ |
| Semantic versioning | v1.x (stable API) | v0.x (unstable) |

## Recommendation

**Use `github.com/modelcontextprotocol/go-sdk` (official SDK).**

Rationale:

1. **Official and maintained by spec authors.** The SDK is developed by Google and Anthropic engineers who write the MCP specification itself. When the spec evolves, this SDK will be updated first and correctly. Community libraries lag or diverge.

2. **Type-safe tool registration eliminates boilerplate.** Samverk has 22 tools. With the official SDK, each tool's input struct doubles as the JSON schema definition via `jsonschema` struct tags. Input validation is automatic. With mcp-go, each tool requires manual builder calls for every parameter and manual type assertions in handlers.

3. **Built-in auth middleware matches our needs exactly.** `auth.RequireBearerToken` is a standard `func(http.Handler) http.Handler` that extracts Bearer tokens, validates them via a custom `TokenVerifier`, and injects `TokenInfo` into context. Samverk's API key auth maps directly to this pattern. With mcp-go, we would need to build this middleware from scratch.

4. **Stable API (v1.x).** The official SDK reached v1.0.0 in September 2025 and is now at v1.4.0. The API surface is stable with backward compatibility guarantees. mcp-go is still at v0.44.x with no v1.0 commitment, meaning breaking changes are possible.

5. **Stateless JSON response mode.** Samverk's MCP server is designed for stateless request/response (no SSE streams needed). The official SDK supports both `Stateless: true` and `JSONResponse: true`, which maps exactly to our "JSON-RPC 2.0 over HTTP POST" requirement.

6. **Go 1.24 requirement is not a concern.** Samverk is on Go 1.25.0 (`go.mod`), which satisfies the go-sdk's Go 1.24+ requirement.

7. **Fewer dependencies with better provenance.** The go-sdk's dependencies are from Google (`google/go-cmp`, `google/jsonschema-go`) and well-known projects (`golang-jwt`, `segmentio/encoding`). mcp-go pulls in `spf13/cast`, `invopop/jsonschema`, and `stretchr/testify` as direct dependencies.

**Trade-off acknowledged:** mcp-go has 2x the stars and a lower Go version requirement. But stars reflect first-mover adoption, not quality. The official SDK's conformance test suite, stable versioning, and spec-author maintenance outweigh community popularity.

## Implementation Sketch

The following shows how Samverk would integrate the official go-sdk into `internal/mcp/` and wire it into the existing server.

### Tool Registration

Each MCP tool gets a typed input struct and a handler function:

```go
package mcp

import (
    "context"

    gosdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListIssuesInput defines the parameters for the list_issues tool.
type ListIssuesInput struct {
    State    string   `json:"state,omitempty"    jsonschema:"Issue state filter (open, closed, all)"`
    Labels   []string `json:"labels,omitempty"   jsonschema:"Filter by label names"`
    Assignee string   `json:"assignee,omitempty" jsonschema:"Filter by assignee username"`
    Limit    int      `json:"limit,omitempty"    jsonschema:"Maximum number of issues to return"`
}

// ListIssuesOutput is the structured output for list_issues.
type ListIssuesOutput struct {
    Issues []IssueSummary `json:"issues"`
    Total  int            `json:"total"`
}

// IssueSummary is a compact issue representation for tool output.
type IssueSummary struct {
    Number int      `json:"number"`
    Title  string   `json:"title"`
    State  string   `json:"state"`
    Labels []string `json:"labels"`
}

// handleListIssues handles the list_issues MCP tool call.
func (h *Handler) handleListIssues(
    ctx context.Context,
    req *gosdk.CallToolRequest,
    input ListIssuesInput,
) (*gosdk.CallToolResult, ListIssuesOutput, error) {
    tracker, err := h.trackerForContext(ctx)
    if err != nil {
        return nil, ListIssuesOutput{}, err
    }

    issues, err := tracker.ListIssues(ctx, input.State, input.Labels)
    if err != nil {
        return nil, ListIssuesOutput{}, err
    }

    out := ListIssuesOutput{Total: len(issues)}
    for _, iss := range issues {
        out.Issues = append(out.Issues, IssueSummary{
            Number: iss.Number,
            Title:  iss.Title,
            State:  iss.State,
            Labels: iss.Labels,
        })
    }
    return nil, out, nil
}
```

### Server Setup and Tool Wiring

```go
package mcp

import (
    "net/http"

    gosdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Handler holds dependencies for MCP tool handlers.
type Handler struct {
    forgeRegistry ForgeRegistry  // resolves project -> IssueTracker
    store         Store          // SQLite persistence
    autonomy      TierEvaluator  // autonomy tier checks
}

// NewServer creates the MCP server with all tools registered.
func NewServer(h *Handler) *gosdk.Server {
    server := gosdk.NewServer(
        &gosdk.Implementation{
            Name:    "samverk",
            Version: "0.0.1",
        },
        &gosdk.ServerOptions{
            Instructions: "Samverk project management tools for check-in conversations.",
        },
    )

    // Project management tools
    gosdk.AddTool(server, &gosdk.Tool{
        Name:        "list_projects",
        Description: "List all registered projects with status summary",
    }, h.handleListProjects)

    gosdk.AddTool(server, &gosdk.Tool{
        Name:        "set_project",
        Description: "Set active project context for this conversation",
    }, h.handleSetProject)

    // Issue operations
    gosdk.AddTool(server, &gosdk.Tool{
        Name:        "list_issues",
        Description: "List issues with filters (state, labels, assignee)",
    }, h.handleListIssues)

    gosdk.AddTool(server, &gosdk.Tool{
        Name:        "get_issue",
        Description: "Get full issue details including comments",
    }, h.handleGetIssue)

    gosdk.AddTool(server, &gosdk.Tool{
        Name:        "create_issue",
        Description: "Create a new issue with title, body, and labels",
    }, h.handleCreateIssue)

    // ... remaining 17 tools follow the same pattern

    return server
}
```

### Wiring into Existing `http.ServeMux`

The `StreamableHTTPHandler` implements `http.Handler`, so it mounts directly onto the mux alongside REST API and SPA routes:

```go
package server

import (
    "context"
    "net/http"

    gosdk "github.com/modelcontextprotocol/go-sdk/mcp"
    "github.com/modelcontextprotocol/go-sdk/auth"
    internalmcp "github.com/herbhall/samverk/internal/mcp"
)

// Setup creates the HTTP server with MCP, REST API, and SPA routes.
func Setup(mcpHandler *internalmcp.Handler, apiHandler http.Handler) *http.ServeMux {
    // Create the MCP server with all tools.
    mcpServer := internalmcp.NewServer(mcpHandler)

    // Create the Streamable HTTP handler (stateless, JSON responses).
    mcpHTTPHandler := gosdk.NewStreamableHTTPHandler(
        func(r *http.Request) *gosdk.Server {
            return mcpServer
        },
        &gosdk.StreamableHTTPOptions{
            Stateless:    true,
            JSONResponse: true,
        },
    )

    mux := http.NewServeMux()

    // MCP endpoint with Bearer token auth middleware.
    mux.Handle("POST /mcp", authMiddleware(mcpHTTPHandler))

    // REST API for dashboard.
    mux.Handle("/api/v1/", apiHandler)

    // Embedded SPA (static files).
    mux.Handle("/", spaHandler())

    return mux
}
```

### Auth Middleware

The go-sdk provides `auth.RequireBearerToken` that wraps an `http.Handler`. Samverk implements a custom `TokenVerifier` that validates API keys against `.samverk/auth.yaml`:

```go
package server

import (
    "context"
    "net/http"

    "github.com/modelcontextprotocol/go-sdk/auth"
)

// authMiddleware wraps an http.Handler with Bearer token validation.
func authMiddleware(next http.Handler) http.Handler {
    verifier := func(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
        // Look up the API key hash in the auth store.
        keyInfo, err := lookupAPIKey(token)
        if err != nil {
            return nil, auth.ErrInvalidToken
        }

        return &auth.TokenInfo{
            UserID: keyInfo.Name,
            Scopes: keyInfo.Projects,
        }, nil
    }

    middleware := auth.RequireBearerToken(verifier, nil)
    return middleware(next)
}
```

### JSON-RPC Request/Response Flow

With `Stateless: true` and `JSONResponse: true`, the request/response cycle is standard HTTP:

```text
Client (Claude)                          Samverk MCP Server
     |                                         |
     |  POST /mcp                              |
     |  Authorization: Bearer <api-key>        |
     |  Content-Type: application/json         |
     |  Accept: application/json               |
     |                                         |
     |  {                                      |
     |    "jsonrpc": "2.0",                    |
     |    "id": 1,                             |
     |    "method": "tools/call",              |
     |    "params": {                          |
     |      "name": "list_issues",             |
     |      "arguments": {                     |
     |        "state": "open",                 |
     |        "labels": ["status:blocked"]     |
     |      }                                  |
     |    }                                    |
     |  }                                      |
     |  -------------------------------------> |
     |                                         |  1. Auth middleware validates Bearer token
     |                                         |  2. StreamableHTTPHandler parses JSON-RPC
     |                                         |  3. SDK validates input against schema
     |                                         |  4. handleListIssues called with typed input
     |                                         |  5. Handler queries IssueTracker
     |                                         |  6. SDK marshals typed output to JSON-RPC
     |  <------------------------------------- |
     |                                         |
     |  HTTP 200                               |
     |  Content-Type: application/json         |
     |                                         |
     |  {                                      |
     |    "jsonrpc": "2.0",                    |
     |    "id": 1,                             |
     |    "result": {                          |
     |      "content": [{                      |
     |        "type": "text",                  |
     |        "text": "{\"issues\":[...],      |
     |                  \"total\":5}"           |
     |      }],                                |
     |      "structuredContent": {             |
     |        "issues": [...],                 |
     |        "total": 5                       |
     |      }                                  |
     |    }                                    |
     |  }                                      |
```

### Autonomy Tier Enforcement

Tier enforcement wraps tool handlers via MCP-level middleware. Tier 3 actions return a confirmation prompt instead of executing:

```go
package mcp

import (
    "context"
    "encoding/json"
    "fmt"

    gosdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// autonomyMiddleware checks the action tier before executing tool handlers.
func (h *Handler) autonomyMiddleware() gosdk.Middleware {
    return func(next gosdk.MethodHandler) gosdk.MethodHandler {
        return func(ctx context.Context, method string, req gosdk.Request) (gosdk.Result, error) {
            if method != "tools/call" {
                return next(ctx, method, req)
            }

            // Extract tool name from the request.
            toolName := extractToolName(req)
            tier := h.autonomy.EvaluateTier(toolName)

            // Tier 1 and 2: execute immediately.
            if tier <= 2 {
                return next(ctx, method, req)
            }

            // Tier 3: return confirmation prompt.
            confirmation := map[string]any{
                "status":       "confirmation_required",
                "action":       toolName,
                "tier":         tier,
                "confirm_tool": "approve_action",
                "reason":       fmt.Sprintf("%s requires user confirmation", toolName),
            }
            data, _ := json.Marshal(confirmation)
            return &gosdk.CallToolResult{
                Content: []gosdk.Content{
                    &gosdk.TextContent{Text: string(data)},
                },
            }, nil
        }
    }
}
```

## Migration Path

If the official SDK ever introduces a breaking change or proves insufficient, migration to mcp-go is straightforward because:

1. Both libraries use the same MCP protocol (JSON-RPC 2.0)
2. Both implement `http.Handler` for mux integration
3. Tool handler signatures differ but the business logic (forge queries, store calls) is identical
4. Auth middleware pattern is the same (extract token from header, validate, inject context)

The main refactoring cost would be converting typed input structs to manual builder patterns and adding explicit type assertions in handlers.

## References

- [Official go-sdk repository](https://github.com/modelcontextprotocol/go-sdk)
- [mcp-go repository](https://github.com/mark3labs/mcp-go)
- [MCP Streamable HTTP spec](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports#streamable-http)
- [Samverk MCP Server Requirements](mcp-server.md)
- [Samverk Tech Stack](tech-stack.md)

## Data Collected

Research conducted 2026-03-02. Repository statistics and API surfaces verified via `gh api` against live GitHub data.
