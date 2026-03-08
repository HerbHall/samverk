# Gitea Migration — Execution Plan

## Track Overview

28 issues across 8 parallel tracks. Tracks A–D launch immediately with no dependencies.
Tracks E–J depend on earlier work. Track K is the final validation gate.

```
PARALLEL START (no dependencies)
─────────────────────────────────────────────────────────────────────

Track A: RepoWriter          Track B: PullRequestManager
  B01 CreateBranch             B04 CreatePR
  B02 CreateOrUpdateFile       B05 GetPR + ListPRs
  │                            B06 MergePR
  └──► B03 Verify RepoWriter   B07 GetPRChecks + ListReviewComments
                               │
                               └──► B08 Verify PR Lifecycle ◄── B01,B02

Track C: RepoReader          Track D: Gitea Instance Setup
  B09 All 6 methods            B11 Create prod repo [HUMAN]
  │                            │
  └──► B10 Verify RepoReader   └──► B12 Verify repo config [QC]

Track E: CI/CD (after research)
  B13 Research Gitea Actions compatibility
  │
  ├──► B14 Port ci.yml
  │    │
  │    └──► B15 Port release/security workflows
  │         │
  │         └──► B16 Verify CI on test PR [QC]

DEPENDS ON ADAPTER + INSTANCE WORK
─────────────────────────────────────────────────────────────────────

Track F: Dual-Forge Config (after B11)
  B17 Update server.yaml for dual-forge ◄── B11
  B18 Update create-issues.sh for Gitea ◄── B11
  │
  └──► B19 Verify dispatcher on Gitea [CRITICAL] ◄── B08, B12, B17

Track G: Issue Migration (after B11)
  B20 Build migration script ◄── B11
  │
  └──► B21 Verify migrated issues [QC] ◄── B20

Track H: MCP Dual-Forge (after adapter + config)
  B22 Update MCP tools ◄── B09, B17
  │
  └──► B23 Verify from Claude Desktop [HUMAN] ◄── B22

Track I: Git Mirror (after B11)
  B24 Configure dual-push remote [HUMAN] ◄── B11

Track J: Documentation (after B17)
  B25 ADR-031 Dual-Forge Model ◄── B17
  │
  └──► B26 Update CLAUDE.md + status.md ◄── B25

FINAL GATE
─────────────────────────────────────────────────────────────────────

Track K: Validation
  B27 Full agent loop on Gitea [CRITICAL] ◄── B08,B10,B16,B19,B21,B23
  │
  └──► B28 Conversational check-in dual-forge [HUMAN, CRITICAL] ◄── B27
```

## Agent Assignment Summary

| Agent Type  | Issues | Can Run In Parallel |
|-------------|--------|---------------------|
| code-gen    | B01, B02, B04, B05, B06, B07, B09, B14, B15, B17, B18, B20, B22 | Yes — 13 issues across 6 tracks |
| test        | B03, B08, B19 | After dependencies met |
| qc          | B12, B16, B21, B27 | After dependencies met |
| research    | B13 | Immediate start |
| docs        | B25, B26 | After B17 |
| human       | B11, B23, B24, B28 | B11 immediate; others after deps |

## Parallel Execution Windows

### Window 1 (Immediate — no blockers)
Up to 6 agents simultaneously:

| Agent | Issue | Track |
|-------|-------|-------|
| code-gen #1 | B01 CreateBranch | A |
| code-gen #2 | B02 CreateOrUpdateFile | A |
| code-gen #3 | B04 CreatePR | B |
| code-gen #4 | B05 GetPR+ListPRs | B |
| code-gen #5 | B06 MergePR | B |
| code-gen #6 | B09 RepoReader (all 6) | C |
| code-gen #7 | B07 GetPRChecks+ReviewComments | B |
| research    | B13 CI/CD research | E |
| **human**   | **B11 Create Gitea prod repo** | D |

### Window 2 (After Window 1 completes)
| Agent | Issue | Unlocked By |
|-------|-------|-------------|
| test  | B03 Verify RepoWriter | B01, B02 |
| test  | B08 Verify PR lifecycle | B01,B02,B04-B07 |
| test  | B10 Verify RepoReader | B09 |
| qc    | B12 Verify repo config | B11 |
| code-gen | B14 Port ci.yml | B13 |
| code-gen | B17 Dual-forge config | B11 |
| code-gen | B18 create-issues.sh | B11 |
| code-gen | B20 Migration script | B11 |
| human | B24 Dual-push remote | B11 |

### Window 3 (After Window 2)
| Agent | Issue | Unlocked By |
|-------|-------|-------------|
| code-gen | B15 Port remaining workflows | B13, B14 |
| code-gen | B22 MCP dual-forge | B09, B17 |
| qc    | B21 Verify migration | B20 |
| docs  | B25 ADR-031 | B17 |
| test  | B19 Dispatcher on Gitea **[CRITICAL]** | B08, B12, B17 |

### Window 4 (Final verification)
| Agent | Issue | Unlocked By |
|-------|-------|-------------|
| qc    | B16 CI test PR | B14, B15 |
| human | B23 MCP from Claude Desktop | B22 |
| docs  | B26 Update CLAUDE.md | B25 |

### Window 5 (Gate)
| Agent | Issue | Unlocked By |
|-------|-------|-------------|
| qc    | **B27 Full agent loop [CRITICAL]** | B08,B10,B16,B19,B21,B23 |

### Window 6 (Ship it)
| Agent | Issue | Unlocked By |
|-------|-------|-------------|
| human | **B28 Dual-forge check-in [CRITICAL]** | B27 |

## Critical Path

The longest dependency chain determines minimum time to completion:

```
B04/B05/B06/B07 → B08 → B19 → B27 → B28
     (PR adapter)  (verify) (dispatcher) (full loop) (ship)
```

**Bottleneck:** B11 (human task — create Gitea prod repo) blocks 8 downstream issues.
Do this first.

## Risk Items

1. **Gitea Actions compatibility** — B13 research may reveal that release-please or
   CodeQL have no Gitea equivalent. Mitigation: keep those on GitHub Actions only.
2. **Issue number divergence** — GitHub and Gitea will have different issue numbers.
   depends_on references won't match across forges. Mitigation: B20 migration script
   handles re-mapping.
3. **Gitea SDK gaps** — SearchCode may not have a Gitea equivalent. Mitigation: B09
   documents limitations and returns ErrNotImplemented with clear comment.
4. **Agent PR loop on Gitea** — First time the full agent loop runs against Gitea
   in production. Mitigation: B08 integration test catches issues early.
