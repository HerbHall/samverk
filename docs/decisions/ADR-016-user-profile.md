# ADR-016: User Profile as First-Class Concept

## Status

Accepted

## Context

During Samverk development, Claude Code automatically read the HerbHall/devkit repo to apply consistent project conventions. This revealed that developers already maintain persistent preferences and standards across projects -- Samverk agents need access to these to avoid re-asking resolved questions and to maintain consistency.

Without a user profile, every agent on every task starts from scratch: "What license?" "What branch naming?" "What test framework?" The user repeats themselves endlessly, or agents guess inconsistently.

## Decision

Samverk maintains a persistent user profile that captures preferences, conventions, and standing decisions across all projects. Agents consult the profile rather than asking the user to repeat themselves.

The profile covers:

- **Project conventions:** directory structure, naming, git workflow
- **Technical preferences:** languages, frameworks, testing, CI/CD
- **AI agent configuration:** trust tiers, model routing, cost thresholds
- **Standing decisions:** license, hosting, security requirements

The profile can be bootstrapped from an existing Devkit-style repo, repo analysis, onboarding conversation, or manual configuration. Profile defaults are overridden by explicit project-level config.

## Consequences

- Reduces check-in friction -- agents do not re-ask resolved questions
- Enables consistency across projects without manual repetition
- Requires a profile schema, storage mechanism, and update flow
- Introduces precedence resolution between profile and project config
- Profile must be versioned so old projects use the conventions they were started with

## References

- [User Profile](../user-profile.md)
- [ADR-017: Devkit as Reference Implementation](ADR-017-devkit-reference.md)
