# ADR-017: Devkit as Reference Implementation

## Status

Accepted

## Context

The user profile concept (ADR-016) needs validation against real-world data before the schema is finalized. HerbHall/devkit is an existing repo that already functions as a cross-project organizational standard, containing project templates, editor config, linting standards, git templates, and methodology documentation.

## Decision

HerbHall/devkit serves as the reference implementation and first test case for the user profile concept. Samverk's profile ingestion will be validated against the existing Devkit structure.

This means:

- The profile schema must be able to represent everything useful in Devkit
- The ingestion flow will be designed and tested against Devkit first
- Gaps between Devkit's content and the profile schema inform schema revisions

## Consequences

- Dogfooding with real material validates the concept before generalizing
- Ties initial development to a specific repo structure (acceptable for V1)
- Devkit may contain content that does not map cleanly to a profile -- these gaps are valuable findings
- Success here proves the concept works for the founder's actual workflow

## References

- [User Profile](../user-profile.md)
- [ADR-016: User Profile as First-Class Concept](ADR-016-user-profile.md)
