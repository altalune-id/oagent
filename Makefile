GOTOOLCHAIN ?= auto
GO := env -u GOROOT GOTOOLCHAIN=$(GOTOOLCHAIN) go

BIN     := bin/auth-server
VERSION := $(shell cat version/VERSION 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD   := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X altalune.id/auth-server/version.Version=$(VERSION) \
	-X altalune.id/auth-server/version.Commit=$(COMMIT) \
	-X altalune.id/auth-server/version.BuildTime=$(BUILD)

.DEFAULT_GOAL := help

.PHONY: help
help: ## list targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  make %-8s %s\n", $$1, $$2}'

.PHONY: build
build: ## build the auth-server binary into bin/
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/auth-server

.PHONY: install
install: ## install auth-server into GOBIN/GOPATH/bin
	CGO_ENABLED=0 $(GO) install -trimpath -ldflags "$(LDFLAGS)" ./cmd/auth-server

.PHONY: run
run: ## run the auth-server serve command
	$(GO) run ./cmd/auth-server serve

.PHONY: config-examples
config-examples: ## regenerate .env.reference and config.reference.yml from struct tags
	$(GO) tool gen-config-example
	$(GO) tool gen-config-example --format=yaml

.PHONY: generate
generate: ## regenerate protos (connect, openapi, mcp tools)
	env -u GOROOT GOTOOLCHAIN=$(GOTOOLCHAIN) buf generate

.PHONY: fmt
fmt: ## format all Go files
	$(GO) fmt ./...

.PHONY: vet
vet: ## run go vet
	$(GO) vet ./...

.PHONY: lint
lint: ## run golangci-lint
	env -u GOROOT GOTOOLCHAIN=$(GOTOOLCHAIN) golangci-lint run --timeout=3m ./...

.PHONY: test
test: ## run tests with the race detector
	$(GO) test -race ./...

.PHONY: cover
cover: ## run tests with coverage
	$(GO) test ./... -cover

.PHONY: tidy
tidy: ## run go mod tidy
	$(GO) mod tidy

.PHONY: check
check: vet lint test ## vet + lint + test
