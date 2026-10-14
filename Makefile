GOTOOLCHAIN ?= auto
GO := env -u GOROOT -u GOVERSION GOTOOLCHAIN=$(GOTOOLCHAIN) go
BUF ?= pnpm exec buf

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
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  make %-16s %s\n", $$1, $$2}'

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
	env -u GOROOT -u GOVERSION GOTOOLCHAIN=$(GOTOOLCHAIN) $(BUF) generate
	env -u GOROOT -u GOVERSION GOTOOLCHAIN=$(GOTOOLCHAIN) $(BUF) generate --template buf.gen.mcp.yaml
	$(GO) tool gen-scope
	$(GO) tool gen-apperror

.PHONY: generate-check
generate-check: generate ## fail when make generate changes a tracked file (run on a clean tree)
	@if [ -n "$$(git status --porcelain -- . ':!pnpm-lock.yaml')" ]; then git status --short; echo "generated code is stale: run make generate and commit it"; exit 1; fi

.PHONY: config-check
config-check: ## fail when .env.reference or config.reference.yml drifted from the config structs
	$(GO) tool gen-config-example --check
	$(GO) tool gen-config-example --format=yaml --check

GOVULNCHECK_VERSION ?= v1.9.0

.PHONY: vulncheck
vulncheck: ## scan dependencies and the standard library for known vulnerabilities
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

.PHONY: templ
templ: ## generate templ components
	$(GO) tool templ generate -path internal

.PHONY: web-css
web-css: ## compile internal/web/static/app.css
	pnpm --filter @oagent/web build

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
	node ui/scripts/compress.mts internal/web/console/public

.PHONY: ui
ui: web ui-mcp ui-console ## build all embedded web assets

.PHONY: build-all
build-all: ui ## build embedded web assets, then the oagent binary
	@$(MAKE) build

GOLANGCI_VERSION ?= v2.14.0
# the installed golangci-lint when it is GOLANGCI_VERSION, else that exact version through go run
GOLANGCI := $(shell v=$$(golangci-lint version --short 2>/dev/null); \
	if [ "v$$v" = "$(GOLANGCI_VERSION)" ]; then echo "env -u GOROOT -u GOVERSION GOTOOLCHAIN=$(GOTOOLCHAIN) golangci-lint"; \
	else echo "$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)"; fi)

.PHONY: fmt
fmt: ## format all Go files (gofmt + gci import groups, settings from .golangci.yml)
	$(GOLANGCI) fmt ./...

.PHONY: vet
vet: ## run go vet (unit and integration files)
	$(GO) vet -tags integration ./...

.PHONY: lint
lint: lint-go lint-js lint-config ## run every linter

.PHONY: lint-go
lint-go: ## run golangci-lint (GOLANGCI_VERSION)
	$(GOLANGCI) run --timeout=3m ./...

.PHONY: lint-js
lint-js: ## run ESLint over ui/, docs and every JS/TS/Vue/JSON/Markdown/CSS file
	pnpm lint

.PHONY: lint-config
lint-config: ## run yamllint, hadolint, actionlint and shellcheck (binary, else Docker, else skipped)
	scripts/lint/tool.sh yamllint --strict -c .yamllint.yaml .
	@if ls Dockerfile* >/dev/null 2>&1; then scripts/lint/tool.sh hadolint Dockerfile*; fi
	@if ls .github/workflows/*.y*ml >/dev/null 2>&1; then scripts/lint/tool.sh actionlint; fi
	scripts/lint/tool.sh shellcheck $$(git ls-files -co --exclude-standard -- '*.sh' .husky/pre-commit .husky/pre-push ':!:.agents' ':!:.claude')

.PHONY: comment-check
comment-check: ## fail on any comment that breaks the comment discipline
	$(GO) tool comment-lint .

.PHONY: comment-list
comment-list: ## list comment-discipline violations without failing
	@$(GO) tool comment-lint -list .

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
COVER_SKIP ?= /gen(/|$$)|/views$$|/testutil/|/sessiontest$$|/ratelimittest$$|/examples/|/cmd/oagent$$

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
