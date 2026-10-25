# Developer commands. Run `make help` for a list.
SHELL := /bin/bash
NODE ?= worker-1
AGENT_PORT ?= 9101
API ?= http://127.0.0.1:8080
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: help etcd-up etcd-down etcd-reset compose-config run-control-plane run-agent \
        frontend-install frontend-dev test test-race vet fmt fmt-check tidy build \
        frontend-lint frontend-typecheck frontend-test frontend-build lint ci smoke clean-containers

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

## --- local dependencies -----------------------------------------------------
etcd-up: ## Start etcd in Docker and wait until it is healthy
	docker compose up -d --wait etcd

etcd-down: ## Stop etcd (data is kept in the etcd-data volume)
	docker compose down

etcd-reset: ## Stop etcd and DELETE all cluster state (the etcd-data volume)
	docker compose down --volumes

clean-containers: ## Remove every container the agents created
	@ids="$$(docker ps -aq --filter label=io.mini-orchestrator.managed=true)"; \
	if [ -n "$$ids" ]; then docker rm -f $$ids; else echo "nothing to clean"; fi

compose-config: ## Validate docker-compose.yml
	docker compose config --quiet

## --- run --------------------------------------------------------------------
run-control-plane: ## Run the API server, scheduler and controllers
	cd backend && go run -ldflags "-X main.version=$(VERSION)" ./cmd/controlplane

run-agent: ## Run a node agent: make run-agent NODE=worker-2 AGENT_PORT=9102
	cd backend && AGENT_NODE_NAME=$(NODE) AGENT_LISTEN_ADDR=127.0.0.1:$(AGENT_PORT) go run ./cmd/agent

frontend-install: ## Install console dependencies from the lockfile
	cd frontend && npm ci

frontend-dev: ## Run the console on http://localhost:5173
	cd frontend && npm run dev

## --- quality ----------------------------------------------------------------
test: ## Go unit tests (no Docker or etcd needed)
	cd backend && go test ./...

test-race: ## Go unit tests with the race detector
	cd backend && go test -race -count=1 ./...

vet: ## go vet
	cd backend && go vet ./...

fmt: ## Format Go code
	cd backend && gofmt -w .

fmt-check: ## Fail if Go code is not gofmt-formatted
	@cd backend && out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

tidy: ## go mod tidy
	cd backend && go mod tidy

build: ## Build both binaries into backend/bin
	cd backend && go build -ldflags "-X main.version=$(VERSION)" -o bin/controlplane ./cmd/controlplane
	cd backend && go build -o bin/agent ./cmd/agent

frontend-lint: ## ESLint
	cd frontend && npm run lint

frontend-typecheck: ## TypeScript type check
	cd frontend && npm run typecheck

frontend-test: ## Vitest
	cd frontend && npm test

frontend-build: ## Production build of the console
	cd frontend && npm run build

lint: vet fmt-check frontend-lint frontend-typecheck ## All static checks

ci: fmt-check vet test-race build frontend-lint frontend-typecheck frontend-test frontend-build ## Everything CI runs

smoke: ## API smoke test against a running control plane (API=...)
	API=$(API) ./scripts/smoke.sh
