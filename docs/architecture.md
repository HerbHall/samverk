# Architecture

## Overview

Samverk organizes AI agents into a hierarchical structure operating on two parallel tracks: **Production** (gets work done) and **Quality Control** (validates the work). The system runs asynchronously -- agents work continuously regardless of whether the user is present.

## Hybrid Local/Cloud Model

The original architecture assumed cloud-only. This has been updated to a tiered hybrid model where task complexity determines where work runs:

```text
Cloud (paid API)              Orchestration layer, complex reasoning,
e.g. Claude, GPT-4           architectural decisions, QC arbitration,
                              resolving ambiguity

Mid-tier model                Division/Department layer
(API or large local)          task decomposition, planning

Local agents                  Agent layer, narrow execution tasks:
(containerized,               code generation, formatting, testing,
GPU-accelerated)              schema validation, boilerplate, docs
```

### Why Containers for Local Agents

- Each agent type runs in its own container with appropriate model pre-loaded
- Containers scale horizontally -- run multiple agents in parallel
- Clean resource boundaries
- Reproducible environments
- Ollama runs cleanly inside Docker -- tooling exists today

### Container Spin-up Latency Is Not a Problem

Cold start latency (30-90 seconds to spin up a container and load a model) is NOT a user experience problem for this audience. The user is not watching it work. Do not optimize for cold start performance -- optimize for throughput and quality instead.

## The Hierarchy

```text
                    USER INPUT
                        |
              ORCHESTRATION LAYER
              Intake, clarification, planning
              (Cloud: complex reasoning)
                        |
                  DIVISION LAYER
          Research | Production | Legal | QC
          (Mid-tier: task decomposition)
                        |
                DEPARTMENT LAYER
          Focused scope per domain
          (Mid-tier or local)
                        |
                  AGENT LAYER
          Narrow-scope execution
          (Local: containerized agents)

              PARALLEL QC TRACK
          Each layer has a corresponding
          QC mirror that validates output
          before it moves upward.
```

## Layer Responsibilities

### Orchestration Layer

- Receives raw user input (from async check-in sessions)
- Asks clarifying questions (queued for next check-in if user is absent)
- Researches context (existing codebase, prior decisions, constraints)
- Produces an approved plan before any production work begins
- **Nothing goes to Production without an approved plan**
- Runs on cloud models -- requires strong reasoning capability

### Division Layer

Divisions represent major domains of work:

- **Research Division** -- information gathering, technical spikes, unknowns reduction
- **Production Division** -- code, content, and artifact generation
- **Legal/Compliance Division** -- license review, IP considerations, policy checks
- **QC Division** -- testing, validation, review

### Department Layer

Each Division breaks its work into Departments -- smaller, more focused scopes. For example, Production Division might have:

- Backend Department
- Frontend Department
- Documentation Department
- Infrastructure Department

### Agent Layer

Individual agents execute specific, narrow tasks. An agent in the Backend Department might only handle database schema generation. Narrow scope = higher quality output + easier validation. Runs on local containerized models for cost efficiency and throughput.

## The QC Mirror

Every Production layer has a parallel QC structure:

```text
Production Agent  -->  QC Agent (validates output)
Production Dept   -->  QC Dept (validates dept deliverables)
Production Div    -->  QC Div (validates division output)
```

When QC rejects output, the problem escalates upward through the production hierarchy for re-parameterization, then flows back down.

### Arbitration

When Production and QC disagree and cannot resolve at their level, the conflict escalates to the Orchestration Layer for resolution. This prevents deadlocks. If the Orchestration Layer cannot resolve it, the decision is queued for the user's next check-in.

## Multi-Model Failover

Samverk is model-agnostic. Provider failover on credit exhaustion is a core feature:

- If Claude API credits run out, fall over to GPT-4 or Gemini
- If all cloud credits are exhausted, fall back to local models
- Resume cloud when credits reset
- User configures priority order and API keys

This serves double duty: **cost management** (never blocked by a single provider's billing) and **quality diversity** (different models have different blind spots -- rotating providers improves overall output quality).

## Escalation Path

```text
Agent hits blocker
    --> Escalates to Department
        --> Department escalates to Division
            --> Division escalates to Orchestration
                --> Orchestration re-parameterizes or queues for user
                    --> Flows back down with updated parameters
```

## External Contractors

Certain specialized tasks call for external APIs or providers rather than in-house agents:

- Legal database lookups
- Trademark searches
- Specialized ML models
- Domain-specific APIs

These are treated as "external contractors" -- called when needed, billed per use, not part of the permanent org chart.

## Implementation Stack

- **Language:** Go (consistent with Subnetree project)
- **Primary platform:** Windows (developer's primary environment)
- **AI Providers:** Anthropic Claude API (primary), multi-provider failover
- **Local models:** Ollama in Docker containers
- **Orchestration:** Custom -- not built on LangChain/LangGraph/CrewAI
