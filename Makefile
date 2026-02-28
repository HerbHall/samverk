.PHONY: build test test-race test-coverage lint lint-md run clean

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
	@which golangci-lint > /dev/null 2>&1 || (echo "golangci-lint not found. Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

lint-md:
	npx markdownlint-cli2 "**/*.md" "#node_modules"

run: build
	./bin/$(BIN) serve

clean:
	rm -rf bin/ coverage.out
	go clean
