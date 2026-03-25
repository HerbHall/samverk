# Version Management

## Single Source of Truth

All tool versions are pinned in `tools.json` at the project root. CI workflows,
the Makefile, and pre-push hooks read from this file. To update a tool version,
edit `tools.json` and push -- all consumers pick up the change automatically.

## tools.json

```json
{
  "tools": {
    "go": "1.26.1",
    "node": "22",
    "pnpm": "10",
    "golangci-lint": "v2.10.1",
    "govulncheck": "v1.1.4",
    "markdownlint-cli2": "0.22.0",
    "swag": "v1.16.4"
  }
}
```

## Local Development Files

| File | Purpose | Read by |
|------|---------|---------|
| `.go-version` | Go version for `goenv` and editors | IDEs, goenv |
| `.nvmrc` | Node.js version for `nvm use` | nvm, fnm |
| `web/package.json` `packageManager` | pnpm version for corepack | corepack, pnpm |
| `go.mod` | Go version for CI | `actions/setup-go` |

## How Versions Flow

```text
tools.json (source of truth)
  ├── Makefile (reads via grep/sed)
  ├── scripts/pre-push (reads via grep/sed)
  ├── CI workflows (reads via jq)
  └── docs/version-management.md (reference)

.go-version / .nvmrc (local dev alignment)
  └── Read by editors and version managers

go.mod (Go toolchain)
  └── Read by actions/setup-go in CI

web/package.json packageManager field
  └── Read by corepack for pnpm enforcement
```

## Updating a Tool Version

1. Edit `tools.json` with the new version
2. If updating pnpm: run `cd web && pnpm install` to regenerate lockfile
3. If updating Go: also update `go.mod` (`go mod edit -go=X.Y.Z`) and `.go-version`
4. If updating Node: also update `.nvmrc`
5. Push -- CI picks up the new version automatically

## Verification

Run `make version-check` to compare local tool versions against `tools.json`:

```bash
$ make version-check
=== Tool Version Check (tools.json) ===
  Go:             1.26.1 (ok)
  Node:           22 (ok)
  pnpm:           10 (major)
  golangci-lint:  v2.10.1
  govulncheck:    v1.1.4
  swag:           v1.16.4
=== Done ===
```

## Version Update Cadence

- **Security patches**: Immediate (same day as advisory)
- **Minor updates**: Quarterly review
- **Major updates**: Evaluated per release, tested in a dedicated PR

## Related

- [ADR-033](decisions/ADR-033-multi-machine-free-agent-runtime.md) -- multi-machine agent fleet
- [Unified Execution Plan](unified-execution-plan.md) -- roadmap phases
