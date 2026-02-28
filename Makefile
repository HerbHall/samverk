.PHONY: build test test-race test-coverage lint lint-md lint-all ci hooks run clean

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

build:
	go build $(LDFLAGS) -o bin/$(BIN) ./cmd/samverk/

test:
	go test ./...

test-race:
	go test -race ./...

test-coverage:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.10.1 run ./...

lint-md:
	npx markdownlint-cli2 "**/*.md" "#node_modules"

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

clean:
	rm -rf bin/ coverage.out
	go clean
