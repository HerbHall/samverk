# Summary

<!-- 1-3 bullet points describing what this PR does -->

## Dashboard Evergreen Checklist

- [ ] Does this change add a new background service? → update Metrics page or create page
- [ ] Does this change add a new API resource group? → add page/section
- [ ] Does this change affect MCP tools/servers? → update MCP page
- [ ] Does this change affect routing/providers? → update Providers page
- [ ] Does this change affect data stores? → update Data page
- [ ] Does this change affect project registry? → update Projects page
- [ ] N/A: this change has no user-visible system components

## Test Plan

<!-- How was this change tested? -->

- [ ] `make ci` passes locally
- [ ] Manual verification (describe what was checked)
