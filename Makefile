GOTOOLCHAIN ?= auto
GO := env -u GOROOT GOTOOLCHAIN=$(GOTOOLCHAIN) go

BIN     := bin/oagent
VERSION := $(shell cat version/VERSION 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD   := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X altalune.id/oagent/version.Version=$(VERSION) \
	-X altalune.id/oagent/version.Commit=$(COMMIT) \
	-X altalune.id/oagent/version.BuildTime=$(BUILD)

.DEFAULT_GOAL := help

.PHONY: help
help: ## list targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  make %-8s %s\n", $$1, $$2}'

.PHONY: build
build: ## build the oagent binary into bin/
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/oagent

.PHONY: webhook-pong
webhook-pong: ## build the webhook-pong example into bin/
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/webhook-pong ./examples/webhook-pong

.PHONY: install
install: ## install oagent into GOBIN/GOPATH/bin
	CGO_ENABLED=0 $(GO) install -trimpath -ldflags "$(LDFLAGS)" ./cmd/oagent

.PHONY: run
run: ## run the oagent serve command
	$(GO) run ./cmd/oagent serve

.PHONY: config-examples
config-examples: ## regenerate .env.reference and config.reference.yml from struct tags
	$(GO) tool gen-config-example
	$(GO) tool gen-config-example --format=yaml

.PHONY: generate
generate: templ ## regenerate protos (connect, openapi, mcp tools), scope names, error codes and templ components
	env -u GOROOT GOTOOLCHAIN=$(GOTOOLCHAIN) buf generate
	$(GO) tool gen-scope
	$(GO) tool gen-apperror

.PHONY: templ
templ: ## generate templ components
	$(GO) tool templ generate -path internal

.PHONY: web-css
web-css: ## compile internal/web/static/app.css
	pnpm --filter @oagent/web exec tailwindcss -i app.css -o ../../internal/web/static/app.css --minify

.PHONY: web-js
web-js: ## build internal/web/static/password.js
	pnpm --filter @oagent/web build:js

.PHONY: web
web: web-css web-js ## build the templ pages' CSS and JS

.PHONY: ui-mcp
ui-mcp: ## build the MCP App (Vue) and embed it
	pnpm --filter @oagent/mcp-ui build
	cp ui/mcp/dist/index.html internal/mcp/ui/assets/app.html

.PHONY: ui-test
ui-test: ## run every ui/* package's tests
	pnpm -r --if-present test

.PHONY: ui-console
ui-console: ## build the console SPA and embed it
	pnpm --filter @oagent/console generate
	@mkdir -p internal/web/console/public
	@find internal/web/console/public -mindepth 1 ! -name .gitkeep -exec rm -rf {} +
	@cp -R ui/console/.output/public/. internal/web/console/public/

.PHONY: ui
ui: ui-mcp ui-console ## build all embedded web assets

.PHONY: build-all
build-all: ui ## build embedded web assets, then the oagent binary
	@$(MAKE) build

.PHONY: fmt
fmt: ## format all Go files
	$(GO) fmt ./...

.PHONY: vet
vet: ## run go vet (unit and integration files)
	$(GO) vet -tags integration ./...

.PHONY: lint
lint: ## run golangci-lint
	env -u GOROOT GOTOOLCHAIN=$(GOTOOLCHAIN) golangci-lint run --timeout=3m ./...

.PHONY: test
test: ## run tests with the race detector
	$(GO) test -race ./...

INTEGRATION_TIMEOUT ?= 10m

.PHONY: test-integration
test-integration: ## run integration tests (TEST_PG_DSN if set, else one Postgres container per package)
	$(GO) test -race -shuffle=on -tags integration -timeout $(INTEGRATION_TIMEOUT) ./...

.PHONY: test-all
test-all: test test-integration ## unit + integration tests

COVER_MIN ?= 85
# generated code, templ views, test helpers and main wiring
COVER_SKIP ?= /gen(/|$$)|/views$$|/testutil/|/sessiontest$$|/examples/|/cmd/oagent$$

.PHONY: cover
cover: ## unit + integration coverage; fails when a package is under COVER_MIN percent
	PATH="$$($(GO) env GOROOT)/bin:$$PATH" $(GO) test -tags integration -count=1 -timeout $(INTEGRATION_TIMEOUT) -cover ./... \
		| tee /dev/stderr \
		| awk -v min=$(COVER_MIN) -v skip='$(COVER_SKIP)' ' \
			$$1 == "FAIL" { bad = 1 } \
			{ pkg = ""; pct = -1 } \
			$$1 == "ok" { pkg = $$2; for (i = 1; i <= NF; i++) if ($$i == "coverage:") pct = $$(i + 1) + 0 } \
			$$1 == "?" { pkg = $$2; pct = 0 } \
			$$2 == "coverage:" { pkg = $$1; pct = $$3 + 0 } \
			pkg != "" && pkg !~ skip && pct < min { printf "cover: %s is at %.1f%% (min %d%%)\n", pkg, pct, min > "/dev/stderr"; bad = 1 } \
			END { exit bad }'

.PHONY: tidy
tidy: ## run go mod tidy
	$(GO) mod tidy

.PHONY: check
check: vet lint test ## vet + lint + test
