# Plan Commit — Handoff Instructions

## What This Is

Six planning files from a Claude Chat session on 2026-03-08 that need to be
committed to the Samverk repo. These define the unified execution plan for
three work streams: Gitea migration (B-track), adaptive worker scaling (W-track),
and PC agent worker (P-track). 62 issues total.

## Files to Commit

Copy these files into the repo at the paths shown:

```text
scripts/gitea-migration-batch.json   ← 28 issues, B01-B28
scripts/adaptive-scaling-batch.json  ← 20 issues, W01-W20
scripts/pc-agent-batch.json          ← 14 issues, P01-P14
docs/unified-execution-plan.md       ← Master plan, cross-stream dependencies
docs/gitea-migration-plan.md         ← B-track dependency graph and execution windows
docs/adaptive-scaling-plan.md        ← W-track phased approach (Observe/Expose/Scale/Tune)
```

## Commit Command

```bash
git add \
  scripts/gitea-migration-batch.json \
  scripts/adaptive-scaling-batch.json \
  scripts/pc-agent-batch.json \
  docs/unified-execution-plan.md \
  docs/gitea-migration-plan.md \
  docs/adaptive-scaling-plan.md

git commit -m "docs: add unified execution plan and issue batches for Q2 2026

Three work streams with 62 issues total:
- B-track: Gitea migration (28 issues) — dual-forge operation
- W-track: Adaptive worker scaling (20 issues) — smart resource use
- P-track: PC agent worker (14 issues) — CC in isolated worktrees

Issue batch files use the existing create-issues.sh format.
depends_on fields use batch refs (B01, W01, P01) — resolve to
actual issue numbers after creation.

Key design decisions:
- PC agents work in D:\bots\worker-N via bare clone + git worktrees
- User's D:\devspace\Samverk is never touched by agents
- Scaling: observe → expose → scale → tune (each phase standalone)
- Gitea adapter: 7 stub methods to implement (RepoWriter + PullRequestManager)"
```

## After Commit — CLAUDE.md Update

Add to the Key Decisions section in CLAUDE.md:

- Unified execution plan — Q2 2026 ([docs/unified-execution-plan.md](docs/unified-execution-plan.md))

Add to the References section:

- [Unified Execution Plan](docs/unified-execution-plan.md)
- [Gitea Migration Plan](docs/gitea-migration-plan.md)
- [Adaptive Scaling Plan](docs/adaptive-scaling-plan.md)

## After Commit — Issue Creation

Once committed, create the issues using the existing script. Start with
B-track since B11 (create Gitea prod repo) is the human task that unblocks
the most downstream work:

```bash
# Dry run first
bash scripts/create-issues.sh --dry-run scripts/gitea-migration-batch.json

# Create issues
bash scripts/create-issues.sh scripts/gitea-migration-batch.json
```

Note: The batch files use string refs in depends_on (e.g., "B01" instead of
integer issue numbers). The create-issues.sh script currently expects integers.
Either:

1. Two-pass: create all issues, build a ref→number mapping, update frontmatter
2. Or: strip depends_on before first pass, add them via a second script

The W-track and P-track can be created after, or in parallel.
