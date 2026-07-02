# Tadmor backend developer tasks.
#
# Uses the pinned Go toolchain and a hermetic, vendored build: no module or
# toolchain downloads happen during these targets.
export PATH := /usr/local/go/bin:$(PATH)
export GOFLAGS := -mod=vendor
export GOTOOLCHAIN := local
export GOPROXY := off

# Connection strings. Override on the command line, e.g.
#   make test TEST_DATABASE_URL=postgres://user:pass@host:5432/db?sslmode=disable
DATABASE_URL ?= postgres://tadmor:tadmor@127.0.0.1:5432/tadmor?sslmode=disable
TEST_DATABASE_URL ?= postgres://tadmor:tadmor@127.0.0.1:5432/tadmor_test?sslmode=disable

.DEFAULT_GOAL := help
.PHONY: help build release image run test vet fmt fmt-check web-install web-dev web-build web-check e2e-install e2e-test e2e

# Frontend lives in web/ (pnpm, corepack-pinned). Mirrors the Go targets'
# discipline: the committed pnpm-lock.yaml is the source of truth and CI installs
# are frozen. See docs/frontend-stack.md.
WEB := web

# End-to-end / UI tests live in e2e/ (Playwright, isolated from web/'s runtime
# dependency tree). See e2e/README.md.
E2E := e2e

# Container image tag for `make image`. See docs/deployment.md.
IMAGE := tadmor

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*## "}{printf "  make %-10s %s\n", $$1, $$2}'

build: ## Build the server binary into bin/ (embeds whatever is in web/dist)
	go build -o bin/server ./cmd/server

release: web-build build ## Build the front-end then the server (embedded SPA)

image: ## Build the deployable container image (self-contained; see docs/deployment.md)
	docker build -t $(IMAGE) .

run: build ## Build and run the server
	DATABASE_URL=$(DATABASE_URL) ./bin/server

test: ## Run the full test suite from scratch (integration tests reset the DB)
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -count=1 ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format all Go sources
	gofmt -w cmd internal

fmt-check: ## Fail if any Go source is not gofmt-clean
	@unformatted=$$(gofmt -l cmd internal); \
	if [ -n "$$unformatted" ]; then \
		echo "unformatted files:"; echo "$$unformatted"; exit 1; \
	fi

web-install: ## Install frontend deps from the frozen lockfile
	cd $(WEB) && corepack pnpm install --frozen-lockfile

web-dev: ## Run the Vite dev server
	cd $(WEB) && corepack pnpm dev

web-build: ## Production build into web/dist (embedded by the Go server)
	cd $(WEB) && corepack pnpm build
	@touch $(WEB)/dist/.gitkeep  # Vite's emptyOutDir wipes it; keep the embed placeholder tracked

web-check: ## Type-check + audit the frontend
	cd $(WEB) && corepack pnpm typecheck && corepack pnpm audit --audit-level=high

e2e-install: ## Install e2e deps (frozen) + Playwright's Chromium (see e2e/README.md for OS deps)
	cd $(E2E) && corepack pnpm install --frozen-lockfile && corepack pnpm install-browser

e2e-test: ## Run the Playwright UI tests (stack must already be running; BASE_URL overridable)
	cd $(E2E) && corepack pnpm test

e2e: ## Build+run the server, run the Playwright UI tests, then tear it down
	DATABASE_URL=$(DATABASE_URL) $(E2E)/run-local.sh
