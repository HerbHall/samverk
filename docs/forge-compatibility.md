# Forge Compatibility Matrix

Comparison of GitHub and Gitea APIs against Samverk's IssueTracker requirements.
Informs the interface design in `internal/forge/` and adapter implementations.

## Go SDKs

| Forge | SDK | Module Path |
|-------|-----|-------------|
| GitHub | google/go-github v68 | `github.com/google/go-github/v68/github` |
| Gitea | gitea/go-sdk | `code.gitea.io/sdk/gitea` |

## Required Operations

All 7 operations from the communication protocol spec, plus `GetIssue` and `ListComments` added during design review.

| Operation | GitHub API | GitHub SDK | Gitea API | Gitea SDK | Compatible? |
|-----------|-----------|------------|-----------|-----------|-------------|
| **CreateIssue** | `POST /repos/{owner}/{repo}/issues` | `IssuesService.Create()` | `POST /api/v1/repos/{owner}/{repo}/issues` | `CreateIssue()` | Yes |
| **GetIssue** | `GET /repos/{owner}/{repo}/issues/{number}` | `IssuesService.Get()` | `GET /api/v1/repos/{owner}/{repo}/issues/{index}` | `GetIssue()` | Yes |
| **UpdateIssue** | `PATCH /repos/{owner}/{repo}/issues/{number}` | `IssuesService.Edit()` | `PATCH /api/v1/repos/{owner}/{repo}/issues/{index}` | `EditIssue()` | Yes |
| **ListIssues** | `GET /repos/{owner}/{repo}/issues` | `IssuesService.ListByRepo()` | `GET /api/v1/repos/{owner}/{repo}/issues` | `ListRepoIssues()` | Yes |
| **AddComment** | `POST /repos/{owner}/{repo}/issues/{number}/comments` | `IssuesService.CreateComment()` | `POST /api/v1/repos/{owner}/{repo}/issues/{index}/comments` | `CreateIssueComment()` | Yes |
| **ListComments** | `GET /repos/{owner}/{repo}/issues/{number}/comments` | `IssuesService.ListComments()` | `GET /api/v1/repos/{owner}/{repo}/issues/{index}/comments` | `ListIssueComments()` | Yes |
| **SetLabels** | `PUT /repos/{owner}/{repo}/issues/{number}/labels` | `IssuesService.ReplaceLabelsForIssue()` | `PUT /api/v1/repos/{owner}/{repo}/issues/{index}/labels` | `ReplaceIssueLabels()` | Yes |
| **AddLabel** | `POST /repos/{owner}/{repo}/issues/{number}/labels` | `IssuesService.AddLabelsToIssue()` | `POST /api/v1/repos/{owner}/{repo}/issues/{index}/labels` | `AddIssueLabels()` | Yes |
| **RemoveLabel** | `DELETE /repos/{owner}/{repo}/issues/{number}/labels/{label_id}` | `IssuesService.RemoveLabelForIssue()` | `DELETE /api/v1/repos/{owner}/{repo}/issues/{index}/labels/{id}` | `DeleteIssueLabel()` | Yes |
| **Assign** | `POST /repos/{owner}/{repo}/issues/{number}/assignees` | `IssuesService.AddAssignees()` | `PATCH /api/v1/repos/{owner}/{repo}/issues/{index}` (body) | `EditIssue()` with assignees | Partial -- different mechanism |
| **Unassign** | `DELETE /repos/{owner}/{repo}/issues/{number}/assignees` | `IssuesService.RemoveAssignees()` | `PATCH /api/v1/repos/{owner}/{repo}/issues/{index}` (body) | `EditIssue()` with assignees | Partial -- different mechanism |
| **Watch** | Webhooks: `POST /repos/{owner}/{repo}/hooks` | N/A (use `net/http` handler) | Webhooks: `POST /api/v1/repos/{owner}/{repo}/hooks` | `CreateRepoHook()` | Yes -- both support webhook creation |

## Pagination

| Aspect | GitHub | Gitea |
|--------|--------|-------|
| Page parameter | `page` | `page` |
| Size parameter | `per_page` (max 100) | `limit` (max 50 default) |
| Response headers | `Link` header with rel=next/last | `X-Total-Count` + `Link` header |
| SDK handling | `ListOptions{Page, PerPage}` | `ListOptions{Page, PageSize}` |

Both SDKs abstract pagination. The adapter normalizes to a shared `ListOptions` struct.

## Authentication

| Aspect | GitHub | Gitea |
|--------|--------|-------|
| Token auth | `Authorization: Bearer <token>` | `Authorization: token <token>` |
| SDK auth | Pass `oauth2.TokenSource` via `http.Client` | `NewClientWithHTTP()` or `SetToken()` option |
| Scopes needed | `repo` (full repo access) | API token with repo permissions |

## Key Differences

### Assignee Management

GitHub has dedicated endpoints (`POST /assignees`, `DELETE /assignees`) that accept a list of usernames. Gitea handles assignees via the issue edit endpoint -- you PATCH the full assignee list. The adapter must:

- GitHub: call `AddAssignees()` / `RemoveAssignees()`
- Gitea: read current assignees, modify the list, call `EditIssue()`

### Label References

GitHub's label endpoints accept label **names** (strings). Gitea's endpoints accept label **IDs** (integers). The adapter must:

- GitHub: pass label names directly
- Gitea: resolve label names to IDs via `GetRepoLabel()` before add/remove operations

### Webhook Events

| Event | GitHub | Gitea |
|-------|--------|-------|
| Issue opened | `issues` (action: opened) | `issues` (action: opened) |
| Issue labeled | `issues` (action: labeled) | `issue_label` |
| Issue commented | `issue_comment` | `issue_comment` |
| Issue closed | `issues` (action: closed) | `issues` (action: closed) |
| Issue assigned | `issues` (action: assigned) | `issues` (action: assigned) |

Webhook payloads differ in structure but carry equivalent data. The adapter normalizes both into `forge.Event`.

### Rate Limiting

| Aspect | GitHub | Gitea (self-hosted) |
|--------|--------|---------------------|
| Authenticated | 5,000 req/hr | No limit (configurable) |
| Unauthenticated | 60 req/hr | Configurable |
| Headers | `X-RateLimit-Remaining` | None by default |

## Features NOT in Shared Interface

These are GitHub-specific features that Gitea lacks API support for. They are **not** part of the `IssueTracker` interface:

- PR review comments (line-level diff comments)
- Security/Dependabot alerts
- GitHub Actions integration
- Code scanning results
- Branch protection rule management (Gitea has basic support but different API)

## Recommendation

**Separate adapter implementations** behind the shared `IssueTracker` interface. The SDKs are structurally different enough (pointer semantics in go-github, value semantics in gitea-sdk, different auth patterns) that a single "compatible" client would be more complex than two clean adapters.

Build GitHub adapter first (issue #15), then Gitea adapter as a follow-up issue using this matrix as the implementation guide.

## MCP Server Strategy

**Don't wrap an existing MCP server.** GitHub's official MCP server (`github/github-mcp-server`) is comprehensive but GitHub-specific. Instead:

- Build Samverk's MCP server on top of the `IssueTracker` interface
- Use the official MCP Go SDK (`modelcontextprotocol/go-sdk`) as the protocol layer
- MCP tools map to `IssueTracker` methods -- one server works with any forge
- This aligns with ADR-013 (forge abstraction) and avoids coupling the MCP layer to a specific platform
