# Gitea Actions Compatibility Research

Research for issue #268. Documents what is needed to run existing GitHub Actions workflows
on Gitea Actions, identifying compatibility gaps and required changes.

## Infrastructure Assessment (CT 200)

**Current state:**

| Resource | Available | Notes |
|----------|-----------|-------|
| Gitea version | 1.23.7 | Actions feature supported (introduced in 1.19) |
| Actions enabled | No | `[actions]` section absent from `app.ini` |
| Act runner | Not installed | `act_runner.service` not found |
| Go toolchain | Not installed | Must be provisioned on runner |
| Docker | Not available | LXC container (privileged required for Docker-in-Docker) |
| Memory | 1 GB (757 MB free) | Marginal for concurrent jobs; Go builds peak ~400 MB |
| Disk | 7.8 GB (6.6 GB free) | Adequate for Go module cache + build artifacts |

**Verdict:** Gitea Actions is not yet enabled on the server. Enabling it requires:

1. Add `[actions]` section to `/etc/gitea/app.ini` (`ENABLED = true`)
2. Install and register `act_runner` on CT 200 (or a dedicated runner LXC)
3. Provision Go 1.24+, Node 22, pnpm on the runner host

## Workflow-by-Workflow Compatibility

### ci.yml — Build, test, lint, markdown

**Actions used:**

| Action | Gitea compatible? | Notes |
|--------|------------------|-------|
| `actions/checkout@v4` | Yes | Gitea ships a mirror at `https://gitea.com/actions/checkout` |
| `actions/setup-go@v5` | Yes | Available via Gitea actions mirror |
| `actions/setup-node@v4` | Yes | Available via Gitea actions mirror |
| `pnpm/action-setup@v4` | Yes | Pure shell; works with act |
| `golangci/golangci-lint-action@v7` | Partial | Uses GitHub-specific env vars for caching; simpler to replace with `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6 run ./...` |
| `actions/upload-artifact@v4` | Yes | Gitea 1.21+ supports artifact storage |
| `actions/download-artifact@v4` | Yes | Gitea 1.21+ |
| `DavidAnson/markdownlint-cli2-action@v19` | Partial | May not be mirrored; replace with `npx markdownlint-cli2 "**/*.md" ...` |

**Required changes:**

- Replace `golangci/golangci-lint-action@v7` with a `run:` step using `go run`
- Replace `DavidAnson/markdownlint-cli2-action@v19` with `npx markdownlint-cli2`
- Add `uses: actions/cache@v4` for Go module cache (golangci-lint-action provided this automatically)
- Use `runs-on: ubuntu-latest` — Gitea maps this to the registered runner label

**Estimated effort:** Low (2-3 step replacements)

### nightly.yml — Nightly cross-platform binaries

**Actions used:** `checkout`, `setup-go`, `setup-node`, `pnpm/action-setup`, `upload-artifact` — all Gitea compatible.

**Required changes:**

- `pnpm/action-setup@v4` pinned version — verify version tag exists in Gitea mirror, or use `npm install -g pnpm` fallback
- `github.ref` → same variable name works in Gitea Actions
- `origin/main` git reference works identically

**Estimated effort:** Very low (verify pnpm action tag)

### release-gate.yml — Pre-release evaluation

**Actions used:** `actions/checkout@v4` — compatible.

**GitHub-specific dependencies:**

- `github.event.pull_request.user.login == 'github-actions[bot]'` — Gitea equivalent is `gitea.event.pull_request.user.login == 'gitea-actions[bot]'` (context variable names differ)
- Label detection: `contains(github.event.pull_request.labels.*.name, 'autorelease: pending')` — Gitea uses same expression syntax but `github.event` → `gitea.event`

**Required changes:**

- Replace all `github.` context references with `gitea.` equivalents
- The release gate logic (semver comparison, commit counting) is pure shell — fully compatible

**Estimated effort:** Low (search-and-replace context variable prefix)

### release-please.yml — Automated release PRs

**Actions used:** `googleapis/release-please-action@v4`

**Compatibility:** Not available on Gitea. `release-please` is GitHub-native and relies on the GitHub API for PR creation and label management.

**Alternatives for Gitea:**

1. **Manual releases only** — Tag + draft release notes from changelog (simplest)
2. **Gitea webhook + script** — On push to main, run `git cliff` or `conventional-changelog` to generate notes, create release via Gitea API
3. **`release-please` Gitea mode** — `release-please` v16+ has limited Gitea support via `--host` flag but is alpha quality and untested

**Recommendation:** For the dual-forge setup, keep `release-please` on GitHub as the source of truth. Mirror tags to Gitea automatically after GitHub creates the release (B-track already plans this in B21-B22).

**Estimated effort:** Deferred — not needed until B21 (release mirroring)

### codeql.yml — Security scanning

**Compatibility:** Not available on Gitea. CodeQL is GitHub-proprietary and requires GitHub's analysis infrastructure.

**Alternatives for Gitea:**

| Tool | Coverage | Integration |
|------|----------|-------------|
| `govulncheck` | Go module vulnerability DB | `run: go run golang.org/x/vuln/cmd/govulncheck@latest ./...` |
| `gosec` | Go security linter (already in golangci-lint) | Covered by `golangci-lint run` |
| `trivy` | Container + dependency scanning | Docker action or `run: trivy fs .` |
| `nancy` | Go dependency CVE check | `go list -json -deps ./...` piped to `nancy sleuth` |

**Recommendation:** Add a `security.yml` workflow for Gitea that runs `govulncheck + trivy fs`. Keep CodeQL on GitHub (it scans PRs to GitHub). Security coverage is complementary, not duplicated.

**Estimated effort:** Medium — new workflow file, not a port

### copilot-setup-steps.yml — Copilot agent environment

**Compatibility:** Not applicable to Gitea. This workflow configures the GitHub Copilot coding agent environment and has no Gitea equivalent.

**Action:** No Gitea port needed.

## Summary Table

| Workflow | Gitea Status | Required Changes | Effort |
|----------|-------------|-----------------|--------|
| `ci.yml` | Portable with minor edits | Replace 2 marketplace actions with `run:` steps; add cache step | Low |
| `nightly.yml` | Portable | Verify pnpm action tag | Very low |
| `release-gate.yml` | Portable | Replace `github.` → `gitea.` context prefix | Low |
| `release-please.yml` | Not portable | Keep on GitHub; mirror tags to Gitea (B21) | Deferred |
| `codeql.yml` | Not portable | New `security.yml` with govulncheck + trivy | Medium |
| `copilot-setup-steps.yml` | Not applicable | No Gitea port | None |

## Enabling Gitea Actions (Required First Step)

Before any workflow can run on Gitea, actions must be enabled on CT 200:

```bash
# On CT 200 — add to /etc/gitea/app.ini
cat >> /etc/gitea/app.ini << 'EOF'

[actions]
ENABLED = true
DEFAULT_ACTIONS_URL = https://gitea.com
EOF
systemctl restart gitea
```

Then register an `act_runner`. Options:

1. **On CT 200 itself** (host runner, no Docker) — lightweight, no isolation
2. **Dedicated runner LXC** (CT 201) — isolated, can provision Go/Node without affecting Gitea
3. **Docker runner in CT 200** — requires privileged LXC or nested virtualization

**Recommendation:** Create a dedicated runner LXC (CT 201) with:

- 2 GB RAM (Go race detector + pnpm need headroom)
- 20 GB disk (Go module cache + node_modules)
- Go 1.24, Node 22, pnpm, golangci-lint pre-installed
- `act_runner` registered with Gitea using a registration token

This is captured as B14 (#269) in the execution plan.

## Implementation Order

Based on this research, the recommended order for B-track CI work:

1. **B14** (#269): Enable Gitea Actions + provision runner LXC
2. **B15** (#270): Port `ci.yml` → `gitea-ci.yml` (portable workflows)
3. **B16** (#271): Add `security.yml` (govulncheck + trivy) as CodeQL replacement
4. **B21** (#276): Tag mirroring (release-please stays on GitHub)

The dual-forge model means workflows can differ between GitHub and Gitea — no need for identical CI. GitHub runs full CI + CodeQL + release-please. Gitea runs a lighter CI (build, test, lint) as a development sanity check.
