# Architecture

## Overview

Samverk organizes AI agents into a hierarchical structure that mirrors how a real company operates. The hierarchy exists on two parallel tracks: **Production** (gets work done) and **Quality Control** (validates the work).

---

## The Hierarchy

```
┌─────────────────────────────────────────┐
│              USER INPUT                 │
└─────────────────┬───────────────────────┘
                  │
┌─────────────────▼───────────────────────┐
│          ORCHESTRATION LAYER            │
│  - Intake & clarification               │
│  - Research context                     │
│  - Plan approval before execution       │
└──────┬──────────────────────────────────┘
       │  Approved Plan
┌──────▼──────────────────────────────────┐
│            DIVISION LAYER               │
│  Research | Production | Legal | QA     │
│  (Domain decomposition)                 │
└──────┬──────────────────────────────────┘
       │
┌──────▼──────────────────────────────────┐
│           DEPARTMENT LAYER              │
│  Divisions break work into focused      │
│  tasks with clear interfaces            │
└──────┬──────────────────────────────────┘
       │
┌──────▼──────────────────────────────────┐
│             AGENT LAYER                 │
│  Narrow-scope execution agents          │
│  One job, done well                     │
└─────────────────────────────────────────┘

         ║ PARALLEL QC TRACK ║
         Each layer has a corresponding
         QC mirror that validates output
         before it moves upward.
```

---

## Layer Responsibilities

### Orchestration Layer
- Receives raw user input
- Asks clarifying questions to prevent wasted work downstream
- Researches context (existing codebase, prior decisions, constraints)
- Produces an approved plan before any production work begins
- **Nothing goes to Production without an approved plan**

### Division Layer
Divisions represent major domains of work:
- **Research Division** — information gathering, technical spikes, unknowns reduction
- **Production Division** — code, content, and artifact generation
- **Legal/Compliance Division** — license review, IP considerations, policy checks
- **QC Division** — testing, validation, review

### Department Layer
Each Division breaks its work into Departments — smaller, more focused scopes. For example, Production Division might have:
- Backend Department
- Frontend Department
- Documentation Department
- Infrastructure Department

### Agent Layer
Individual agents execute specific, narrow tasks. An agent in the Backend Department might only handle database schema generation. Narrow scope = higher quality output + easier validation.

---

## The QC Mirror

Every Production layer has a parallel QC structure:

```
Production Agent  →  QC Agent (validates output)
Production Dept   →  QC Dept (validates dept deliverables)
Production Div    →  QC Div (validates division output)
```

When QC rejects output, the problem escalates upward through the production hierarchy for re-parameterization, then flows back down.

### Arbitration
When Production and QC disagree and cannot resolve at their level, the conflict escalates to the Orchestration Layer for resolution. This prevents deadlocks.

---

## Escalation Path

```
Agent hits blocker
    → Escalates to Department
        → Department escalates to Division
            → Division escalates to Orchestration
                → Orchestration re-parameterizes or seeks user input
                    → Flows back down with updated parameters
```

---

## External Contractors

Certain specialized tasks call for external APIs or providers rather than in-house agents:
- Legal database lookups
- Trademark searches
- Specialized ML models
- Domain-specific APIs

These are treated as "external contractors" — called when needed, billed per use, not part of the permanent org chart.

---

## Open Design Questions

### Depth Calibration
Who decides how deep the agent tree goes for a given task? A simple bug fix doesn't need the full hierarchy. A new feature might need all layers. This decision logic is not yet designed.

### Cost Management
The parallel QC structure at minimum doubles token consumption. Cost tracking and budget limits need to be first-class concerns, not afterthoughts. An "HR/Finance" equivalent is needed for agent lifecycle and cost management.

### "Good Enough" Threshold
When is output acceptable? Who decides? How many QC cycles before escalating vs. shipping? This needs a configurable policy system.

### Free Tier Optimization
Using free tiers of various AI providers to reduce cost is possible but fragile — policies change constantly. May be a V2 feature with appropriate caveats.

---

## Provider Strategy

**V1: Claude-only**
Start with Anthropic's Claude API exclusively. Clean abstractions from day one to support multi-provider in V2.

**V2: Cross-provider validation**
Use Claude to validate GPT-4 output, or vice versa. This is a genuine differentiator — using provider diversity as a quality mechanism rather than a cost-optimization play. Underexplored in the current market.

---

## Implementation Stack

- **Language:** Go (consistent with Subnetree project)
- **Primary platform:** Windows (developer's primary environment)
- **AI Provider:** Anthropic Claude API (V1)
- **Orchestration:** Custom — not built on LangChain/LangGraph/CrewAI
