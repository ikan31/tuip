.PHONY: help build run test lint fmt install release-snapshot clean

APP := tuip
MAIN := ./cmd/tuip
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP)

help: ## Show available make targets
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the tuip binary into ./bin/tuip
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN) $(MAIN)

run: ## Run tuip from source with ARGS="..."
	go run $(MAIN) $(ARGS)

install: ## Install tuip with go install
	go install $(MAIN)

test: ## Run tests
	go test ./...

lint: ## Run golangci-lint
	golangci-lint run

fmt: ## Format Go files with golangci-lint formatters
	golangci-lint fmt

release-snapshot: ## Build a local GoReleaser snapshot into ./dist
	goreleaser release --snapshot --clean

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) $(APP) dist
