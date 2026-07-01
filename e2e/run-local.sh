#!/usr/bin/env bash
# Run the Playwright e2e suite end-to-end against a freshly built Go server.
#
# This orchestrates the whole local run in one process: build the server binary
# (which embeds web/dist), start it on :8080, wait for it to accept connections,
# run the Playwright tests against it, and always tear the server down again.
# Postgres must already be running; the app's globalTeardown removes the E2E-
# rows it creates (see docs/e2e-testing.md §6).
#
# Overridable via the environment:
#   DATABASE_URL  Postgres connection string for the server.
#   BASE_URL      URL Playwright targets (default http://localhost:8080).
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

# Match the Makefile's hermetic, vendored Go build so the script is safe to run
# directly, not only via `make`.
export PATH="/usr/local/go/bin:$PATH"
export GOFLAGS="-mod=vendor"
export GOTOOLCHAIN="local"
export GOPROXY="off"

DATABASE_URL="${DATABASE_URL:-postgres://tadmor:tadmor@127.0.0.1:5432/tadmor?sslmode=disable}"
BASE_URL="${BASE_URL:-http://localhost:8080}"
host_port="${BASE_URL#*://}" # strip scheme -> host:port[/path]
host_port="${host_port%%/*}"  # strip any path
host="${host_port%%:*}"
port="${host_port##*:}"

echo "==> Building server"
go build -o bin/server ./cmd/server

echo "==> Starting server ($BASE_URL)"
DATABASE_URL="$DATABASE_URL" ./bin/server &
server_pid=$!
# Graceful shutdown on any exit (success, test failure, or Ctrl-C).
trap 'kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true' EXIT

echo "==> Waiting for server to accept connections"
for _ in $(seq 1 30); do
	if ! kill -0 "$server_pid" 2>/dev/null; then
		echo "server exited before becoming ready" >&2
		exit 1
	fi
	if (exec 3<>"/dev/tcp/$host/$port") 2>/dev/null; then
		exec 3>&- 3<&-
		break
	fi
	sleep 1
done

echo "==> Running Playwright tests"
cd "$repo_root/e2e"
BASE_URL="$BASE_URL" corepack pnpm test
