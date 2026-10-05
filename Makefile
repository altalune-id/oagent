GOTOOLCHAIN ?= auto
GO := env -u GOROOT GOTOOLCHAIN=$(GOTOOLCHAIN) go

.DEFAULT_GOAL := help

.PHONY: help
help: ## list targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  make %-8s %s\n", $$1, $$2}'

.PHONY: fmt
fmt: ## format all Go files
	$(GO) fmt ./...

.PHONY: vet
vet: ## run go vet
	$(GO) vet ./...

.PHONY: tidy
tidy: ## run go mod tidy
	$(GO) mod tidy
