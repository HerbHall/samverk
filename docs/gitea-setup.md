# Gitea Instance Configuration

Self-hosted Gitea instance for Samverk forge adapter development and testing.

## Infrastructure

| Component | Value |
|-----------|-------|
| Proxmox Host | 192.168.1.203 (Tailscale: 100.124.44.112) |
| Container ID | 200 (LXC, Debian 12, unprivileged) |
| Container IP | 192.168.1.160 |
| Gitea URL | `http://192.168.1.160:3000` |
| Gitea Version | 1.25.5 |
| Database | SQLite3 |
| Auto-start | Yes (onboot=1) |

## Access

| Setting | Value |
|---------|-------|
| Admin User | samverk-admin |
| API Token Name | samverk-agent |
| Auth Header | `Authorization: token <token>` |
| Organization | samverk |
| Repository | samverk/samverk-test |
| Webhook Secret | samverk-webhook-secret |

**Note:** The API token value is stored outside this repo. Retrieve from the Gitea web UI at Settings > Applications if needed.

## Label ID Map

Labels use integer IDs in the Gitea API (not string names like GitHub).

| ID | Name | Category |
|----|------|----------|
| 1 | agent:orchestrator | Agent type |
| 2 | agent:dispatcher | Agent type |
| 3 | agent:code-gen | Agent type |
| 4 | agent:test | Agent type |
| 5 | agent:docs | Agent type |
| 6 | agent:research | Agent type |
| 7 | agent:qc | Agent type |
| 8 | agent:human | Agent type |
| 9 | status:queued | Status |
| 10 | status:claimed | Status |
| 11 | status:in-progress | Status |
| 12 | status:blocked | Status |
| 13 | status:needs-qc | Status |
| 14 | status:needs-human | Status |
| 15 | status:done | Status |
| 16 | priority:critical | Priority |
| 17 | priority:high | Priority |
| 18 | priority:normal | Priority |
| 19 | priority:low | Priority |
| 20 | complexity:local | Complexity |
| 21 | complexity:cloud | Complexity |
| 22 | complexity:ambiguous | Complexity |

## Gitea API Differences from GitHub

These affect the Gitea adapter implementation at `internal/forge/gitea/`:

1. **Labels use integer IDs** -- adapter must cache name-to-ID mapping via `GET /api/v1/repos/{owner}/{repo}/labels`
2. **Assignees via EditIssue** -- no dedicated assign/unassign endpoints; read-modify-write pattern required
3. **Auth header format** -- `Authorization: token <token>` not `Authorization: Bearer <token>`
4. **Pagination** -- `limit` param (max 50 default) not `per_page` (max 100)
5. **Webhook events** -- `issue_label` is a separate event (GitHub uses `issues` with action `labeled`)

## Verified Operations

All IssueTracker interface methods confirmed working (2026-02-28):

- CreateIssue -- `POST /api/v1/repos/{owner}/{repo}/issues`
- GetIssue -- `GET /api/v1/repos/{owner}/{repo}/issues/{index}`
- UpdateIssue -- `PATCH /api/v1/repos/{owner}/{repo}/issues/{index}`
- ListIssues -- `GET /api/v1/repos/{owner}/{repo}/issues?state=open&type=issues&limit=50`
- AddComment -- `POST /api/v1/repos/{owner}/{repo}/issues/{index}/comments`
- ListComments -- `GET /api/v1/repos/{owner}/{repo}/issues/{index}/comments`
- SetLabels -- `PUT /api/v1/repos/{owner}/{repo}/issues/{index}/labels` (replace all)
- AddLabel -- `POST /api/v1/repos/{owner}/{repo}/issues/{index}/labels`
- RemoveLabel -- `DELETE /api/v1/repos/{owner}/{repo}/issues/{index}/labels/{id}` (returns 204)
- Assign -- `PATCH /api/v1/repos/{owner}/{repo}/issues/{index}` with `{"assignees": ["username"]}`
- Unassign -- `PATCH /api/v1/repos/{owner}/{repo}/issues/{index}` with `{"assignees": []}`

## Webhook Configuration

| Setting | Value |
|---------|-------|
| Type | gitea |
| Target | `http://192.168.1.220:9999/webhook` |
| Events | issues, issue_assign, issue_label, issue_milestone, issue_comment |
| Content Type | JSON |
| Secret | samverk-webhook-secret |

Webhook delivery to private IPs enabled via `ALLOWED_HOST_LIST = *` in `app.ini`.

## Container Management

```bash
# Start/stop from Proxmox host
pct start 200
pct stop 200

# Enter container shell
pct enter 200

# Check Gitea service
pct exec 200 -- systemctl status gitea

# View Gitea logs
pct exec 200 -- journalctl -u gitea -f
```

## References

- [ADR-013: Forge Abstraction](decisions/ADR-013-forge-abstraction.md)
- [ADR-019: Self-Hosted-First](decisions/ADR-019-self-hosted-first.md)
- [Forge Compatibility Matrix](forge-compatibility.md)
- [Communication Protocol](communication-protocol.md)
