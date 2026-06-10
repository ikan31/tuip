.PHONY: help build run status providers dashboard-create dashboard-add dashboard-list test lint fmt check release-snapshot clean install

APP := tuip
MAIN := ./cmd/tuip
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP)
PROVIDERS ?= slack github cloudflare
DASHBOARD ?= work
CONFIG ?=
CONFIG_FLAG := $(if $(CONFIG),--config $(CONFIG),)

help: ## Show available make targets
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the tuip binary into ./bin/tuip
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN) $(MAIN)

run: ## Run tuip with ARGS="...", for example: make run ARGS="status slack github"
	go run $(MAIN) $(ARGS)

status: ## Run status for default providers; override with PROVIDERS="slack github"
	go run $(MAIN) status $(PROVIDERS)

providers: ## List built-in providers
	go run $(MAIN) providers list

dashboard-create: ## Create a dashboard; override with DASHBOARD=personal CONFIG=./tuip.yaml
	go run $(MAIN) $(CONFIG_FLAG) dashboard create $(DASHBOARD) $(PROVIDERS)

dashboard-add: ## Add default providers to a dashboard; override DASHBOARD/PROVIDERS/CONFIG
	go run $(MAIN) $(CONFIG_FLAG) dashboard add $(DASHBOARD) $(PROVIDERS)

dashboard-list: ## List dashboards; override with CONFIG=./tuip.yaml
	go run $(MAIN) $(CONFIG_FLAG) dashboard list

install: ## Install tuip with go install
	go install $(MAIN)

test: ## Run tests
	go test ./...

lint: ## Run golangci-lint
	golangci-lint run --fix

fmt: ## Format Go files
	gofmt -w cmd internal

check: fmt lint test ## Format, lint, and test final code

release-snapshot: ## Build a local GoReleaser snapshot into ./dist
	goreleaser release --snapshot --clean

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) $(APP) dist
