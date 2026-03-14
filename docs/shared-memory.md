# Shared Agent Memory

- **Status**: Moved to Synapset
- **Date**: 2026-03-03 (original), 2026-03-14 (migrated)

## Summary

The shared agent memory concept -- originally designed here as a Samverk concept doc -- has been extracted into **Synapset**, a standalone MCP server providing vector-based semantic memory to any AI tool.

Synapset is the canonical home for all design, research, and implementation of the shared memory layer. This file remains as a reference pointer.

## What Samverk Retains

Samverk is a **consumer** of Synapset, not the owner. Samverk-specific integration concerns:

- **Dispatch feedback loop** -- Samverk's dispatcher will query Synapset for agent performance history, error patterns, and cost data to make evidence-based routing decisions
- **Institutional memory** -- Agent findings deposited into a `samverk` pool, queryable by future agents regardless of model or provider
- **Selective access** -- QC agents evaluate work without inheriting the producing agent's reasoning biases (Synapset supports pool-level access control)

## Where to Find the Full Design

| What | Where |
|---|---|
| Design vision, principles, constraints | `Synapset/docs/design-vision.md` |
| Concept brief and tech stack | `Synapset/docs/concept-brief.md` |
| Feasibility assessment (4.6/5, GO) | `Synapset/docs/feasibility.md` |
| Research documents (7 completed) | `Synapset/docs/research/` |
| Original Open Brain brief | `Synapset/docs/research/openbrain.txt` |
| Proof of concepts (3 passing) | `Synapset/poc/` |

**Repository**: `samverk-admin/synapset` on Gitea (`D:\DevSpace\Synapset\`)

## References

- [Nate Jones "Open Brain" concept](https://gitea.herbhall.net/samverk-admin/synapset) -- Inspiration, now in Synapset
- [ADR-012: Git Issues as Protocol](decisions/ADR-012-git-issues-protocol.md) -- Current communication layer (Samverk-specific)
