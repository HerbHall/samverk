.PHONY: build test test-race test-coverage test-integration test-all lint lint-md lint-all ci hooks run clean web dev-web \
       cross-build deploy deploy-binary deploy-config redeploy

# Binary
BIN=samverk

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
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.10.1 run ./...

lint-md:
	npx markdownlint-cli2 "**/*.md" "#node_modules" "#web/node_modules" "#CHANGELOG.md"

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
	./bin/$(BIN) serve

# Cross-compile for Linux (deploy target)
DEPLOY_HOST ?= 192.168.1.161
DEPLOY_USER ?= root

cross-build: web
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BIN)-linux-amd64 ./cmd/samverk/
	@echo "Built bin/$(BIN)-linux-amd64"

# Deploy binary to the remote host
deploy-binary: cross-build
	scp bin/$(BIN)-linux-amd64 $(DEPLOY_USER)@$(DEPLOY_HOST):/usr/local/bin/$(BIN)
	@echo "Binary deployed to $(DEPLOY_HOST)"

# Deploy config templates (only copies if files don't already exist on target)
deploy-config:
	scp deploy/samverk-serve.service deploy/samverk-dispatch.service $(DEPLOY_USER)@$(DEPLOY_HOST):/tmp/
	scp deploy/install.sh $(DEPLOY_USER)@$(DEPLOY_HOST):/tmp/install.sh
	@echo "Service files and installer copied to $(DEPLOY_HOST)"

# Full deploy: build, copy binary + configs, run installer, restart services
deploy: deploy-binary deploy-config
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'systemctl stop samverk-dispatch samverk-serve 2>/dev/null; \
		sed -i "s/\r$$//" /tmp/install.sh && bash /tmp/install.sh && \
		systemctl start samverk-serve samverk-dispatch'
	@echo "Deployment complete. Services restarted."

# One-step redeploy with health verification
redeploy:
	$(MAKE) deploy DEPLOY_HOST=192.168.1.162
	@echo "Verifying health..."
	@sleep 3
	@ssh root@192.168.1.162 'curl -sf http://localhost:8080/healthz' && echo " OK" || (echo " FAIL"; exit 1)

clean:
	rm -rf bin/ coverage.out
	go clean
