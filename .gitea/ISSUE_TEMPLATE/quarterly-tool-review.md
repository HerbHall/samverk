---
name: Quarterly Tool Version Review
about: Recurring checklist for reviewing pinned tool versions and CVEs
labels: ["maintenance", "security"]
---

## Quarterly Tool Version Review

**Quarter:** <!-- e.g., Q2 2026 -->
**Reviewer:** <!-- @username -->
**`tools.json` last reviewed:** <!-- YYYY-MM-DD -->

## Version Checklist

Compare each pinned version in `tools.json` against current LTS/stable releases.
Update `tools.json` and any CI workflows as needed.

### Go

- [ ] Check [go.dev/dl](https://go.dev/dl/) for current LTS/stable version
  - Pinned: `<!-- paste current value -->`
  - Latest LTS: `<!-- paste latest -->`
- [ ] Updated `tools.json` if changed

### Node.js

- [ ] Check [Node.js releases](https://nodejs.org/en/download/releases/) for current LTS major
  - Pinned major: `<!-- paste current value -->`
  - Current LTS major: `<!-- paste latest -->`
- [ ] Updated `tools.json` if changed

### pnpm

- [ ] Check [pnpm releases](https://github.com/pnpm/pnpm/releases) for current stable major
  - Pinned major: `<!-- paste current value -->`
  - Latest major: `<!-- paste latest -->`
- [ ] Updated `tools.json` if changed

### golangci-lint

- [ ] Check [golangci-lint releases](https://github.com/golangci/golangci-lint/releases) for latest stable
  - Pinned: `<!-- paste current value -->`
  - Latest: `<!-- paste latest -->`
- [ ] Updated `tools.json` if changed

### govulncheck

- [ ] Check [golang/vuln releases](https://github.com/golang/vuln/releases) for latest stable
  - Pinned: `<!-- paste current value -->`
  - Latest: `<!-- paste latest -->`
- [ ] Updated `tools.json` if changed

### trivy

- [ ] Check [trivy releases](https://github.com/aquasecurity/trivy/releases) for latest stable
  - Used in: `.gitea/workflows/security.yml` (installed via install script, not pinned)
  - Latest: `<!-- paste latest -->`
- [ ] Consider pinning version in `tools.json` if needed

## CVE / Advisory Check

- [ ] Query [OSV database](https://osv.dev) for each pinned tool version
- [ ] Run `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` locally
- [ ] Check GitHub Security Advisories for golangci-lint and trivy
- [ ] Review nightly freshness check issues (label: `security`) for open advisories

## Findings

| Tool | Pinned | Latest | CVEs Found | Action Taken |
|------|--------|--------|------------|--------------|
| go | | | | |
| node | | | | |
| pnpm | | | | |
| golangci-lint | | | | |
| govulncheck | | | | |
| trivy | | | | |

## Actions

- [ ] Updated `tools.json` with new versions
- [ ] CI workflows updated if required
- [ ] Follow-up issues created for any unresolved CVEs
- [ ] Next quarterly review scheduled
