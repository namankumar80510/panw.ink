# Pawn.Ink developer and release targets (IMPLEMENTATION.md §14.1).
# Nothing here deletes volumes, drops databases or touches production.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help

export GOPRIVATE ?= github.com/xmfunew/collar
COMPOSE := docker compose -f deployments/docker-compose.dev.yml
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/namankumar80510/panw.ink/internal/boot.Version=$(VERSION)
BINARIES := server worker collar migrate maintenance

.PHONY: help deps dev-env dev-db dev-db-stop migrate migrate-status seed-dev owner dev-api dev-web dev-worker dev \
        test test-go test-integration test-web test-e2e lint typecheck security build build-web build-go smoke backup release-check clean-build

help: ## Print targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

deps: ## go mod download + npm ci
	go mod download
	cd web && npm ci

dev-env: ## Write .env with fresh development secrets (refuses to overwrite)
	scripts/dev.sh env

dev-db: ## Start disposable MariaDB + Redis + MinIO + Mailpit (never deletes volumes)
	$(COMPOSE) up -d --wait

dev-db-stop: ## Stop the disposable services, keeping their volumes
	$(COMPOSE) stop

migrate: ## Goose app migrations + Collar system migrations, then config check
	go run ./cmd/migrate up
	go run ./cmd/collar migrate
	go run ./cmd/collar config check

migrate-status: ## List pending migrations (flags destructive DDL)
	go run ./cmd/migrate status

seed-dev: ## Development fixtures (refuses outside development)
	go run ./cmd/seed

owner: ## One-time application owner bootstrap (prompts for a password; never over HTTP)
	go run ./cmd/maintenance owner-bootstrap --email "$${OWNER_EMAIL:-}"

dev-api: ## Run the API (serves web/dist from disk when WEB_DIST_DIR is set)
	go run ./cmd/server

dev-web: ## Parcel watch build of both SPAs into web/dist
	cd web && npm run watch

dev-worker: ## Run the worker (Collar jobs + scheduler)
	go run ./cmd/worker

dev: ## Parcel watch + API + worker together (Ctrl-C stops all)
	scripts/dev.sh run

test: test-go test-web ## Go unit tests + frontend unit tests (add test-integration with dev-db running)

test-go: ## Go unit tests
	go test ./...

test-integration: ## Integration tests against the disposable dev MariaDB/Redis (make dev-db)
	go test -race -tags integration -count=1 ./tests/integration/...

test-web: ## Frontend unit tests (Vitest)
	cd web && npm test

test-e2e: ## Playwright end-to-end tests (added in a later phase)
	@echo "Playwright suite is added in Phase 9; see docs/TEST_REPORT.md" && exit 1

typecheck: ## TypeScript type check (Parcel transpiles; it does not type check)
	cd web && npm run typecheck

lint: typecheck ## gofmt + go vet + TypeScript check
	@test -z "$$(gofmt -l cmd internal migrations tests web/embed.go)" || (gofmt -l cmd internal migrations tests web/embed.go; exit 1)
	go vet ./...
	go vet -tags integration ./tests/...

security: ## govulncheck + npm audit
	go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
	cd web && npm audit --omit=dev --audit-level=high

build-web: ## npm ci, type check and Parcel build of both HTML entrypoints
	cd web && npm ci && npm run typecheck && npm run build

build-go: ## Go binaries with the embedded UI into ./bin
	@mkdir -p bin
	@for b in $(BINARIES); do CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/pawn-ink-$$b ./cmd/$$b; echo "built bin/pawn-ink-$$b"; done

build: build-web build-go ## Full production build

smoke: ## Health and boundary checks against SMOKE_URL (default http://127.0.0.1:14021)
	scripts/smoke.sh

backup: ## Coordinated backup (see deployments/backup.sh; never run against unknown hosts)
	deployments/backup.sh

release-check: lint test build ## Pre-release gate: lint, tests, build, pending-migration report
	go run ./cmd/migrate status

clean-build: ## Remove build outputs only (bin/, web/dist contents, Parcel cache)
	rm -rf bin web/.parcel-cache
	find web/dist -mindepth 1 ! -name .gitkeep -delete
