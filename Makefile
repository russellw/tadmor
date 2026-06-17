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
.PHONY: help build run test vet fmt fmt-check

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*## "}{printf "  make %-10s %s\n", $$1, $$2}'

build: ## Build the server binary into bin/
	go build -o bin/server ./cmd/server

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
