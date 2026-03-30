# Contributing to Samverk

Samverk is the pipeline orchestration engine for the Toolkit ecosystem. This guide documents the development workflow for consistency and agent compliance.

## Getting Started

1. Clone the repository from Gitea: `git clone https://gitea.herbhall.net/samverk/samverk.git`
2. Install dependencies:
   - Go 1.23+ for the backend
   - Node.js 20+ and pnpm for the React dashboard (`cd web && pnpm install`)
3. Create a feature branch: `git checkout -b feature/issue-NNN-desc`
4. Make your changes
5. Run CI checks: `make ci`
6. Push and open a pull request on Gitea

## Development Workflow

Follow the DevKit **Explore -> Plan -> Code -> Verify -> Commit** flow:

1. **Explore**: Read relevant source files and understand existing patterns before coding
2. **Plan**: Get plan approval before implementing multi-file changes
3. **Code**: Implement methodically, following existing conventions
4. **Verify**: Run the full CI suite locally before pushing

```bash
make build       # Go compilation
make test        # Go tests
make lint        # golangci-lint
make ci          # All of the above

# Frontend
cd web
pnpm install     # Sync dependencies
npx tsc --noEmit # TypeScript check
pnpm lint        # ESLint
```

After verification, commit with clear messages using conventional prefixes.

### Cross-Compile Check

Samverk deploys to Linux. Always verify cross-compilation:

```bash
GOOS=linux GOARCH=amd64 go build ./...
```

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` new feature (dispatcher capability, API endpoint, dashboard component)
- `fix:` bug fix
- `docs:` documentation only
- `test:` adding or updating tests
- `chore:` maintenance tasks (dependency updates, CI config)
- `refactor:` code restructuring without behavior change

Include the co-author tag for agent-generated commits:

```text
Co-Authored-By: Claude <noreply@anthropic.com>
```

## Pull Requests

- Never commit directly to `main` -- all changes go through feature branches
- Branch naming: `feature/issue-NNN-desc`, `fix/issue-NNN-desc`
- Keep PRs focused on a single change
- Ensure CI passes on both Gitea Actions and GitHub Actions before merging
- Reference related issues with `Closes #N`
- For significant features, verify containerized behavior with a Docker build before merging

## Reporting Issues

- File issues in the [Samverk Gitea repository](https://gitea.herbhall.net/samverk/samverk/issues)
- Use conventional commit prefixes in issue titles: `feat:`, `fix:`, `docs:`, etc.
- Include steps to reproduce for bug reports
- Note affected components (dispatcher, API, dashboard, agent execution)
- Check existing issues before creating a new one

## Code of Conduct

Contributors are expected to maintain a professional and respectful environment. All code changes must pass CI, and all errors found during development must be fixed or tracked with an issue.
