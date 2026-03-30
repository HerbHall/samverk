.PHONY: build test test-race test-coverage test-integration test-all lint lint-md lint-all ci hooks run clean web dev-web \
       cross-build cross-build-full deploy deploy-config deploy-force deploy-staging redeploy ssh ssh-staging version-check \
       check-sync

# Binary
BIN=samverk

# Tool versions from tools.json (single source of truth)
GOLANGCI_LINT_VERSION ?= $(shell grep '"golangci-lint"' tools.json 2>/dev/null | sed 's/.*: *"\(.*\)".*/\1/' || echo "v2.10.1")
GOVULNCHECK_VERSION   ?= $(shell grep '"govulncheck"' tools.json 2>/dev/null | sed 's/.*: *"\(.*\)".*/\1/' || echo "latest")
SWAG_VERSION          ?= $(shell grep '"swag"' tools.json 2>/dev/null | sed 's/.*: *"\(.*\)".*/\1/' || echo "v1.16.4")

# Version injection
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "unknown")
VERSION_PKG = github.com/herbhall/samverk/internal/version

LDFLAGS=-ldflags "-s -w \
	-X $(VERSION_PKG).Version=$(VERSION) \
	-X $(VERSION_PKG).GitCommit=$(COMMIT) \
	-X $(VERSION_PKG).BuildDate=$(DATE)"

web:
	cd web && npx pnpm install --no-frozen-lockfile && npx pnpm build
	cp -r web/dist/* internal/server/static/

dev-web:
	cd web && npx pnpm dev

build: web
	go build $(LDFLAGS) -o bin/$(BIN) ./cmd/samverk/

test:
	go test ./...

test-race:
	go test -race ./...

test-coverage:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

test-integration:
	go test -tags=integration -v -timeout 60s ./internal/integration/...

test-all: test test-integration

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

lint-md:
	npx markdownlint-cli2 "**/*.md" "#node_modules" "#web/node_modules" "#CHANGELOG.md" "#.samverk"

# Run all lint checks (matches CI)
lint-all: lint lint-md

# Local CI simulation: build + test + lint (run before pushing)
ci: build test lint-all

# Install git hooks (pre-push runs CI checks automatically)
hooks:
	cp scripts/pre-push .git/hooks/pre-push
	chmod +x .git/hooks/pre-push
	@echo "pre-push hook installed"

run: build
	SAMVERK_ENV=development ./bin/$(BIN) serve

# Cross-compile for Linux (deploy target)
DEPLOY_HOST ?= 192.168.1.162
DEPLOY_USER ?= root

cross-build:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BIN)-linux-amd64 ./cmd/samverk/
	@echo "Built bin/$(BIN)-linux-amd64"

# Cross-compile with fresh SPA build -- the standard build target for deploy.
# Always rebuilds web first to prevent stale embedded SPA (see issue #458).
cross-build-full: web cross-build

# Pre-deploy: verify local tree matches Gitea main (prevents stale deploys).
# Skip with: DEPLOY_SKIP_SYNC_CHECK=1 make redeploy
check-sync:
	bash scripts/check-gitea-sync.sh

# Deploy: rebuild SPA + binary, wait for idle dispatcher, then swap.
# This is the ONLY deploy target that should be used.
# Enforces Gitea sync check to prevent deploying stale local code.
deploy: check-sync cross-build-full
	bash scripts/safe-deploy.sh $(DEPLOY_HOST)

# One-step redeploy to production with safety gate.
redeploy:
	$(MAKE) deploy DEPLOY_HOST=192.168.1.162

# Config-only deploy: sync providers.yaml and project.yaml without rebuilding the binary.
deploy-config:
	bash scripts/deploy-config.sh $(DEPLOY_HOST)

# Unsafe deploy (skips idle wait). Use only for emergency hotfixes.
deploy-force: cross-build-full
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'systemctl stop samverk-dispatch samverk-serve 2>/dev/null || true'
	scp bin/$(BIN)-linux-amd64 $(DEPLOY_USER)@$(DEPLOY_HOST):/usr/local/bin/$(BIN)
	scp deploy/samverk-serve.service deploy/samverk-dispatch.service $(DEPLOY_USER)@$(DEPLOY_HOST):/tmp/
	scp deploy/install.sh $(DEPLOY_USER)@$(DEPLOY_HOST):/tmp/install.sh
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'sed -i "s/\r$$//" /tmp/install.sh && bash /tmp/install.sh && \
		systemctl start samverk-serve samverk-dispatch'
	@echo "Force deployment complete. Services restarted."

# Deploy to staging (CT 203)
STAGING_HOST ?= 192.168.1.199

deploy-staging: cross-build-full
	bash scripts/safe-deploy.sh $(STAGING_HOST)

# Quick SSH access to production server
ssh:
	ssh root@192.168.1.162

# Quick SSH access to staging server
ssh-staging:
	ssh root@$(STAGING_HOST)

clean:
	rm -rf bin/ coverage.out
	go clean

# Check local tool versions against tools.json (advisory, does not fail)
version-check:
	@echo "=== Tool Version Check (tools.json) ==="
	@EXPECTED_GO=$$(cat .go-version 2>/dev/null || echo "?"); \
	ACTUAL_GO=$$(go version 2>/dev/null | sed 's/.*go\([0-9.]*\).*/\1/' || echo "not found"); \
	if [ "$$EXPECTED_GO" = "$$ACTUAL_GO" ]; then echo "  Go:             $$ACTUAL_GO (ok)"; \
	else echo "  Go:             $$ACTUAL_GO (expected $$EXPECTED_GO)"; fi
	@EXPECTED_NODE=$$(cat .nvmrc 2>/dev/null || echo "?"); \
	ACTUAL_NODE=$$(node --version 2>/dev/null | sed 's/v\([0-9]*\).*/\1/' || echo "not found"); \
	if [ "$$EXPECTED_NODE" = "$$ACTUAL_NODE" ]; then echo "  Node:           $$ACTUAL_NODE (ok)"; \
	else echo "  Node:           $$ACTUAL_NODE (expected $$EXPECTED_NODE)"; fi
	@ACTUAL_PNPM=$$(pnpm --version 2>/dev/null | sed 's/\([0-9]*\).*/\1/' || echo "not found"); \
	echo "  pnpm:           $$ACTUAL_PNPM (major)"
	@echo "  golangci-lint:  $(GOLANGCI_LINT_VERSION)"
	@echo "  govulncheck:    $(GOVULNCHECK_VERSION)"
	@echo "  swag:           $(SWAG_VERSION)"
	@echo "=== Done ==="
