# Samverk Handoff — Complete GitHub to Gitea Migration

*Generated: 2026-03-17 | Source: Claude Code session (11 issues closed, 3 PRs)*

## Context

All migration infrastructure is in place. This session completed:

- Gitea upgraded 1.23.7 -> 1.25.5 on CT 200
- All 16 repos transferred to `samverk` org
- Branch protection on samverk/samverk and samverk/synapset
- Self-hosted GitHub Actions runner on CT 202 (auto-deploy working)
- ADR-031 revised to single-forge-per-project model
- Daily backup cron on CT 200 (3 AM UTC, 7-day retention)
- Release-please research: use semantic-release + @saithodev/semantic-release-gitea
- Gitea CI runner restarted and processing tasks

## Tasks

### 1. Migrate open GitHub issues to Gitea (#252)

5 open issues on GitHub need to move to Gitea `samverk/samverk`. Decision: open-only migration, GitHub becomes read-only archive.

Steps:

- List open issues: `gh issue list -R HerbHall/samverk --state open`
- For each: create on Gitea via API, close GitHub issue with "moved" comment
- Preserve labels and cross-references where possible

### 2. Remove dual registration (#633)

After migration:

- Edit `/var/lib/samverk/.samverk/server.yaml` on CT 202
- Remove the `samverk` (GitHub) project entry
- Rename `samverk-gitea` to `samverk`
- `systemctl restart samverk-serve`
- Verify: `set_project("samverk")` points to Gitea, `list_issues` works

### 3. Close epic #250

Summarize all completed migration work across sessions.

### 4. GitHub read-only archive

- `gh repo edit HerbHall/samverk --enable-issues=false --enable-wiki=false`
- Update README with "Development has moved to Gitea" banner
- Keep public for history

### 5. Gitea mirroring research (#620)

Research Gitea's native push mirror to GitHub for selective public visibility. Low priority.

### 6. Phase-aware routing (#624 + #623)

Code features now unblocked. Dispatcher restricts agent types by project phase. Includes `set_project_phase` MCP tool with Tier 3 approval.

## Infrastructure Reference

| Service | IP | Port | Notes |
|---------|-----|------|-------|
| Gitea | 192.168.1.160 | 3000 | CT 200, v1.25.5 |
| Samverk | 192.168.1.162 | 8080 | CT 202, v0.1.20 |
| GitHub Runner | 192.168.1.162 | -- | ct202-samverk, auto-deploy |
| Gitea Runner | 192.168.1.160 | -- | act_runner v0.2.11, active |

Gitea API token: generate fresh via SSH to CT 200:

```bash
ssh root@192.168.1.160 'su -s /bin/bash -c "/usr/local/bin/gitea admin user generate-access-token --username samverk-admin --token-name session-$(date +%s) --scopes all --config /etc/gitea/app.ini" git'
```
