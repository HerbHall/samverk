# Licensing Decision

## Status

**Deferred.** All rights reserved during private development. License selection happens before public release on GitHub.

## Current Posture

Samverk is developed privately on self-hosted Gitea. No external contributors, no public repository, no license file needed. The codebase is proprietary until a deliberate decision to open-source.

This is intentional:

- No contributors means no CLA complexity yet
- No public repo means no license enforcement questions
- Private development allows full design freedom without competitive exposure
- The license choice is better informed when the product is closer to release

## Migration Plan

```text
Phase 1 (current):  GitHub (private development, CI, issue tracking)
Phase 2 (soon):     Gitea self-hosted (full migration, private)
Phase 3 (release):  GitHub public repo with chosen license
```

The Gitea instance is already running at `192.168.1.160:3000` with a working Samverk adapter (`internal/forge/gitea/`).

## License Comparison Matrix

Research conducted March 2026 for future reference.

### Candidate Licenses

| Factor | AGPL 3.0 | Apache 2.0 | BSL 1.1 |
|--------|----------|------------|---------|
| Fork protection | Strong (network copyleft) | None | Strong (time-limited) |
| SaaS competitor defense | Yes -- must share modifications | No | Yes -- during protection period |
| Self-hosters | Free, must share mods | Free, no obligations | Free for non-competing use |
| Enterprise adoption | Friction (many corps ban AGPL) | No friction | Confusing ("is this open source?") |
| Contributor attraction | Moderate | Maximum | Low (not true open source) |
| Dual-license compatible | Yes (with CLA) | Yes (less incentive) | Yes |
| awesome-selfhosted listing | Main list | Main list | "Non-free" section |
| Patent protection | Implicit via GPL | Explicit patent grant | Varies |
| Dependency compatibility | Restrictive (copyleft viral) | Compatible with everything | Permissive |
| Converts to open source | Already is | Already is | After change date (3-4 years) |

### Precedent Projects

| Project | License | Model | Revenue |
|---------|---------|-------|---------|
| Grafana | AGPL 3.0 | Open core + commercial cloud | $6B valuation |
| Nextcloud | AGPL 3.0 | Open core + enterprise edition | Profitable |
| Mattermost | AGPL 3.0 (was MIT) | Open core + enterprise | $500M+ valuation |
| ZITADEL | AGPL 3.0 (was Apache) | Open core + cloud | Switched to prevent SaaS strip-mining |
| Elastic | AGPL 3.0 (was Apache) | Dual license + cloud | $10B public company |
| HashiCorp | BSL 1.1 (was MPL) | Source-available + cloud | Acquired by IBM $5.4B |
| GitLab | MIT (EE is proprietary) | Open core + SaaS | $8B public company |

### Key Decision Factors (Owner's Preferences)

Captured March 2026:

- **Product-first**: Commercial protection matters more than community growth
- **Major fork concern**: Primary worry is well-funded competitors (Anthropic, GitHub) shipping "Background Copilot" using Samverk's async engine
- **BSL rejected**: Too confusing, "non-free" classification undesirable
- **CLA accepted**: Willing to require Contributor License Agreement when contributors arrive
- **Dual-license later**: Open to adding commercial license track after establishing open-source version
- **Target user**: Hobbyist self-hosters, not enterprise procurement -- AGPL's enterprise friction is irrelevant

### Leading Candidate

**AGPL 3.0** is the strongest match when the time comes:

- Prevents SaaS competitors from forking without sharing modifications
- Self-hosters (the target user) are unaffected
- CLA enables future commercial license track
- Enterprise adoption concerns are irrelevant for the target market
- Proven model (Grafana, Mattermost, Elastic)

This recommendation should be re-evaluated before public release based on:

- Actual monetization model chosen
- Dependency license audit (any incompatible deps?)
- Competitive landscape at time of release
- Whether enterprise users emerge as a viable segment

## Dependency License Audit

To be completed before public release. Current dependencies:

| Dependency | License | AGPL Compatible |
|------------|---------|-----------------|
| google/go-github/v68 | BSD-3-Clause | Yes |
| spf13/cobra | Apache 2.0 | Yes |
| gopkg.in/yaml.v3 | MIT + Apache 2.0 | Yes |
| code.gitea.io/sdk/gitea | MIT | Yes |
| modernc.org/sqlite | BSD-3-Clause | Yes |

No incompatibilities found in current dependencies. Audit should be repeated when adding new dependencies.

## ADR-028: Defer Licensing Until Public Release

### Decision

Defer license selection. Develop under "all rights reserved" on self-hosted Gitea. Choose license before GitHub public release.

### Context

The licensing decision affects contributors, marketplace listing, enterprise adoption, and monetization. Making this decision now -- before the product is validated, before the monetization model is chosen, and before any external contributors exist -- would be premature.

### Options Considered

1. **Choose AGPL now** -- Strong protection but commits to copyleft before validating the market
2. **Choose Apache now** -- Maximum openness but no fork protection if the async engine proves valuable
3. **Choose BSL now** -- Rejected by owner (confusing, "non-free" classification)
4. **Defer (chosen)** -- Develop privately, decide when the product and business model are clearer

### Consequences

**Positive:**

- Full design freedom during private development
- No premature commitment to a license that may not fit the eventual business model
- Can evaluate competitive landscape at time of release
- No CLA overhead while there are no contributors

**Negative:**

- Cannot accept external contributions until license is chosen
- Cannot list on awesome-selfhosted or other open-source directories
- Must remember to choose before going public (tracked by this document)

### Review Trigger

Re-evaluate this decision when ANY of the following occur:

- Decision to move repository to public GitHub
- First external contributor requests access
- Monetization model is finalized
- Alpha release is ready for public testing
