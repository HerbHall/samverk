# Dashboard Evergreen Process

Defines when the Samverk dashboard must be updated, what triggers a required
update, and how to detect drift before it accumulates.

## When Dashboard Updates Are Required

The following changes require a corresponding dashboard update before the PR merges:

- **Adding a new background service** (collector, watcher, probe, goroutine) -- add a
  card to the Metrics page or create a dedicated page
- **Adding a new API subsystem** (new `/api/v1/X` resource group) -- add a page or
  section surfacing the new data
- **Adding or removing an MCP tool or server** -- update the MCP page server list and
  tool count
- **Changing routing chain membership** (adding/removing providers or re-assigning
  chains) -- update the Providers page
- **Adding a new data store table or Synapset pool** -- update the Data page source list
- **Changing the project registry** (`server.yaml` projects) -- update the Projects page

## What Does Not Require a Dashboard Update

- Refactors with no user-visible change
- Bug fixes to existing dashboard pages (the page already exists)
- Internal implementation changes with no new user-facing data
- Dependency upgrades with no API or behavior change
- Documentation-only changes
- Test additions

## Dashboard Update Checklist

Include this checklist in PR descriptions when any trigger above applies.
Check all boxes before merging.

```markdown
## Dashboard Evergreen Checklist

- [ ] Does this change add a new background service? → update Metrics page or create page
- [ ] Does this change add a new API resource group? → add page/section
- [ ] Does this change affect MCP tools/servers? → update MCP page
- [ ] Does this change affect routing/providers? → update Providers page
- [ ] Does this change affect data stores? → update Data page
- [ ] Does this change affect project registry? → update Projects page
- [ ] N/A: this change has no user-visible system components
```

## Drift Detection

To detect drift at session start, compare the routes registered in `web/src/App.tsx`
against the page inventory in `docs/dashboard-ia.md`.

Steps:

1. List all routes from `web/src/App.tsx` (the `<Route path="...">` entries)
2. Open `docs/dashboard-ia.md` and check that each route has a corresponding entry
3. Any route without a page inventory entry is drift -- either add the page to the
   inventory or investigate whether the route was added without a dashboard update

Current routes (as of 2026-03-21):

| Route | Page | Inventory Entry |
|-------|------|-----------------|
| `/` | Dashboard | yes |
| `/issues` | Issues | yes |
| `/my-queue` | My Queue | yes |
| `/agents` | Agents | yes |
| `/providers` | Providers | yes |
| `/metrics` | Metrics | yes |
| `/logs` | Logs | yes |
| `/quality` | Quality | yes |
| `/mcp` | MCP | yes |
| `/projects` | Projects | yes |
| `/data` | Data | yes |
| `/synapset` | Synapset | yes |
| `/devkit` | DevKit | yes |

## Dashboard Audit Process

Run a quarterly review to catch slow drift:

1. List all routes in `web/src/App.tsx`
2. Compare against the page inventory table in `docs/dashboard-ia.md`
3. For each page, verify it loads data correctly against a running instance
4. If new pages were added since the last audit, update `docs/dashboard-ia.md`
5. Update the route table in this file to reflect current state
6. Note the audit date in `docs/dashboard-ia.md`
