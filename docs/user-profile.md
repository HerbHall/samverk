# User Profile

## The Observation

During development of Samverk, Claude Code automatically read the HerbHall/devkit GitHub repo to apply consistent project conventions. Devkit contains:

- Project templates
- Claude/AI agent configuration templates
- Editor config and linting standards
- Git templates
- Methodology and workflow documentation

This is already functioning as a **user organizational standard** -- a persistent set of preferences and conventions that follow the developer across all their projects.

## The Samverk User Profile

Samverk needs a formal equivalent of Devkit built into its architecture. When agents work on a project, they need to know:

- How does this user structure their projects?
- What coding style and conventions do they follow?
- What is their preferred tech stack?
- What decisions have they made on previous projects?
- What are their standing preferences (license type, testing approach, etc.)?

Without this, agents make inconsistent decisions on every task. With it, consistency is automatic -- agents inherit user standards without being told every time.

## Profile Components

### Project Conventions

- Directory structure standards
- File naming conventions
- Git branch and commit message standards
- PR and review workflow

### Technical Preferences

- Primary language(s) and versions
- Preferred frameworks and libraries
- Testing approach and coverage requirements
- Build and CI/CD tooling

### AI Agent Configuration

- Trust tier preferences (autonomy model)
- Preferred model routing (which tasks go to which models)
- Cost thresholds and budget limits
- Communication style preferences

### Standing Decisions

- Default license type
- Open source vs private default
- Preferred hosting and infrastructure
- Security and compliance requirements

## Profile Sources

The user profile can be bootstrapped from:

1. An existing Devkit-style repo (like HerbHall/devkit)
2. Analysis of the user's existing GitHub repos
3. An onboarding conversation with the front-end agent
4. Manual configuration

The profile lives in a dedicated repo or a `.samverk/profile/` directory and is referenced by all projects. Updates to the profile propagate to new work automatically.

## Profile Evolution

The profile should learn over time. When the user makes a decision in a check-in conversation, Samverk should offer to add it to the profile so agents do not re-ask the same question on future projects.

Example:

> Agent: "You chose MIT license for this project. Should I add MIT as your
> default license preference to your profile so I don't need to ask again?"

## Profile Precedence

When project-level config and profile-level config conflict, the resolution order is:

1. Explicit project-level override (highest priority)
2. User profile defaults
3. Samverk system defaults (lowest priority)

This matches how `.editorconfig` cascading works -- project-specific settings override workspace-level settings.

## Reference Implementation

HerbHall/devkit serves as the reference implementation and first test case for the user profile concept. Samverk's profile ingestion will be validated against the existing Devkit structure.

## Related Decisions

- [ADR-016: User Profile as First-Class Concept](decisions/ADR-016-user-profile.md)
- [ADR-017: Devkit as Reference Implementation](decisions/ADR-017-devkit-reference.md)
