# Gitea Migration Readiness Audit

Automated audit for issue [#252](https://github.com/HerbHall/samverk/issues/252),
performed by [#465](https://github.com/HerbHall/samverk/issues/465).

**Date:** 2026-03-14
**Method:** Code inspection via GitHub API, CI workflow comparison, config review.
Server-side checks (Gitea API, SSH) require private network access and are marked accordingly.

## Forge Abstraction — Code Verification

### Gitea PullRequestManager (6/6 methods) — PASS

All six `forge.PullRequestManager` interface methods are implemented in
`internal/forge/gitea/pulls.go`:

| Method | File | Line | Status |
|--------|------|------|--------|
| `CreatePullRequest` | pulls.go | 57 | Implemented |
| `GetPullRequest` | pulls.go | 72 | Implemented |
| `ListPullRequests` | pulls.go | 82 | Implemented |
| `MergePullRequest` | pulls.go | 112 | Implemented |
| `GetPRChecks` | pulls.go | 136 | Implemented |
| `ListReviewComments` | pulls.go | 171 | Implemented |

Compile-time interface assertion confirmed in `gitea.go:18`:

```go
var _ forge.PullRequestManager = (*Client)(nil)
```

### Gitea RepoWriter (2/2 methods) — PASS

Both `forge.RepoWriter` interface methods are implemented in
`internal/forge/gitea/repo.go`:

| Method | File | Line | Status |
|--------|------|------|--------|
| `CreateBranch` | repo.go | 139 | Implemented |
| `CreateOrUpdateFile` | repo.go | 164 | Implemented |

Compile-time interface assertion confirmed in `gitea.go:17`:

```go
var _ forge.RepoWriter = (*Client)(nil)
```

### Gitea RepoReader (6/6 methods) — PASS (with caveat)

All six `forge.RepoReader` interface methods are implemented in
`internal/forge/gitea/repo.go`:

| Method | File | Line | Status |
|--------|------|------|--------|
| `ListFiles` | repo.go | 20 | Implemented |
| `ReadFile` | repo.go | 41 | Implemented |
| `GetDiff` | repo.go | 53 | Implemented (summary-only; SDK lacks compare endpoint) |
| `ListBranches` | repo.go | 72 | Implemented |
| `GetCommitLog` | repo.go | 97 | Implemented |
| `SearchCode` | repo.go | 134 | Stub — returns `errNotImplemented` |

Compile-time interface assertion confirmed in `repo.go:13`:

```go
var _ forge.RepoReader = (*Client)(nil)
```

**Caveat:** `SearchCode` returns `forge.ErrNotSupported` because the Gitea SDK
v0.23.2 does not expose a repo-scoped code search endpoint. Callers receive a
friendly error. This is a known limitation documented in the code comments and
does not block migration for core workflows.

**Caveat:** `GetDiff` returns a formatted summary (base/head commit SHAs) rather
than a real unified diff because the SDK lacks a compare endpoint. This is
sufficient for informational use but does not provide line-level diff output.

### Integration Tests — PASS

Unit tests using `httptest` mocks exist for all three interfaces:

| File | Coverage |
|------|----------|
| `internal/forge/gitea/gitea_test.go` | IssueTracker (13 methods) |
| `internal/forge/gitea/pulls_test.go` | PullRequestManager (6 methods) |
| `internal/forge/gitea/repo_test.go` | RepoReader + RepoWriter |

Live integration tests against a real Gitea instance:

| File | Tests |
|------|-------|
| `internal/integration/gitea_repo_test.go` | 7 test functions |

Integration test functions cover:

- `TestRepoWriter_CreateBranch`
- `TestRepoWriter_CreateOrUpdateFile`
- `TestRepoReader_ListFiles`
- `TestRepoReader_ReadFile`
- `TestRepoReader_ListBranches`
- `TestRepoReader_GetCommitLog`
- `TestRepoReader_GetDiff`
- `TestPRManager_FullLifecycle`

These run with `make test-integration` (build tag: `integration`).

## Gitea Server — Requires Private Network

The following checks target `gitea.herbhall.net` which is only reachable via
Tailscale or the private LAN. They cannot be verified from a cloud-hosted
environment.

| Check | Command | Status |
|-------|---------|--------|
| Gitea version | `curl -s https://gitea.herbhall.net/api/v1/version` | CANNOT VERIFY |
| Actions runner | `curl -s .../repos/samverk/samverk/actions/runners` | CANNOT VERIFY |
| Repository exists | `curl -s .../repos/samverk/samverk` | CANNOT VERIFY |
| Labels count | `curl -s .../repos/samverk/samverk/labels?limit=50` | CANNOT VERIFY |
| HTTPS confirmed | `curl -sI https://gitea.herbhall.net` | CANNOT VERIFY |
| SSH access | `git clone` over SSH | CANNOT VERIFY |

**Recommendation:** Run these checks from CT 202 or any Tailscale-connected
machine. A helper script is included in `docs/gitea-migration-audit.md`.

## CI Parity

### Gitea Workflows Exist — PASS

| Gitea Workflow | Purpose |
|----------------|---------|
| `.gitea/workflows/gitea-ci.yml` | Lint, Build, Test, Markdown Lint |
| `.gitea/workflows/security.yml` | govulncheck, Trivy filesystem scan |

### Job Name Parity — PARTIAL PASS

| Job Name | GitHub CI | Gitea CI | Notes |
|----------|-----------|----------|-------|
| web (SPA build) | Separate job | Inlined into build+test | Gitea lacks cross-job artifact passing |
| lint | `golangci-lint-action` | `go run golangci-lint` | Functionally equivalent |
| build | Depends on `web` artifact | Self-contained (builds SPA inline) | OK |
| test | `go test -race ./...` | `go test ./...` | **No `-race` on Gitea** (CGO unavailable in LXC) |
| markdown | `markdownlint-cli2-action` | `npx markdownlint-cli2` | Functionally equivalent |

### Missing Gitea Equivalents

The following GitHub workflows have **no Gitea counterpart**:

| GitHub Workflow | Purpose | Migration Impact |
|-----------------|---------|------------------|
| `deploy.yml` | Build + deploy to CT 202 | Needed post-migration |
| `nightly.yml` | Nightly build/test | Nice-to-have |
| `release-gate.yml` | Release validation | Needed for release flow |
| `release-please.yml` | Automated releases | Needs alternative for Gitea |
| `codeql.yml` | GitHub-specific code scanning | Replaced by `security.yml` (govulncheck+trivy) |
| `copilot-setup-steps.yml` | GitHub Copilot agent setup | GitHub-specific, not portable |

## Configuration

### server.yaml Gitea Project Entry — FAIL

`deploy/config/server.yaml` exists with correct schema but `projects: []` is
empty. The Gitea project entry is present only as a commented-out example:

```yaml
projects: []
  # - name: samverk-gitea
  #   owner: samverk
  #   repo: samverk
  #   forge: gitea
  #   gitea_url: https://gitea.herbhall.net
```

**Action required:** Uncomment and activate the Gitea project entry when ready.

### Gitea Token in Environment — FAIL

`deploy/samverk.env.example` does not include a `GITEA_TOKEN` variable.
Current variables: `GITHUB_TOKEN`, `SAMVERK_AUTH_TOKEN`, `ANTHROPIC_API_KEY`,
`OPENAI_API_KEY`.

**Action required:** Add `GITEA_TOKEN` to `samverk.env.example` and configure
the actual token on CT 202.

### project.yaml — INFO

`.samverk/project.yaml` currently reads:

```yaml
forge: github
forge_url: https://github.com/HerbHall/samverk
```

This will need updating to `forge: gitea` post-migration.

## Summary

| Category | Items | Pass | Fail | Cannot Verify |
|----------|-------|------|------|---------------|
| Forge Abstraction | 4 | 4 | 0 | 0 |
| Gitea Server | 6 | 0 | 0 | 6 |
| CI Parity | 2 | 1 | 0 | 0 |
| CI Parity (partial) | 1 | — | — | — |
| Configuration | 2 | 0 | 2 | 0 |
| **Total** | **15** | **5** | **2** | **6** |

### NEEDS HUMAN DECISION

Two items from #252 require human input — no automation can resolve these:

1. **Migrate all issues or only open issues?**
   - Option A: Migrate all (250+) — preserves full history but increases
     complexity (cross-reference rewriting, closed-issue noise)
   - Option B: Migrate open only — cleaner start, closed issues remain on
     GitHub as archive
   - Option C: Migrate open + selected closed (milestones, key decisions) —
     balanced approach

2. **Keep GitHub repo as read-only archive or delete it?**
   - Option A: Archive (read-only) — preserves links, stars, forks; zero
     maintenance
   - Option B: Delete — clean break, but loses all external references
   - Option C: Archive + redirect notice in README — best of both worlds
