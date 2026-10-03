#!/usr/bin/env bash
# Run the Playwright e2e suite end-to-end against a freshly built Go server.
#
# This orchestrates the whole local run in one process: build the server binary
# (which embeds web/dist), start it on :8080 against the dedicated e2e database
# (created on first run, migrated by the server on startup), wait for it to
# accept connections, run the Playwright tests against it, and always tear the
# server down again. Postgres must already be running; the app's globalTeardown
# removes the E2E- rows it creates (see docs/e2e-testing.md §6).
#
# Overridable via the environment:
#   DATABASE_URL  Postgres connection string for the server (default: the
#                 dedicated tadmor_e2e database, so a teardown bug can never
#                 touch dev data). globalSetup/globalTeardown are pointed at
#                 the same database.
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

DATABASE_URL="${DATABASE_URL:-postgres://tadmor:tadmor@127.0.0.1:5432/tadmor_e2e?sslmode=disable}"
BASE_URL="${BASE_URL:-http://localhost:8080}"

# The tests' setup/teardown must operate on the same database the server runs
# on — never let them fall back to their own (dev-DB) default.
export E2E_DATABASE_URL="$DATABASE_URL"

# Create the e2e database on first run. The server migrates on startup, so an
# empty database is all that's needed; the suite provisions its own data.
db_name="${DATABASE_URL##*/}"   # strip scheme://user:pass@host:port/
db_name="${db_name%%\?*}"       # strip ?params
if ! psql "$DATABASE_URL" -qAt -c 'SELECT 1' >/dev/null 2>&1; then
	echo "==> Creating database $db_name"
	admin_url="${DATABASE_URL/\/$db_name/\/postgres}"
	if ! psql "$admin_url" -qAt -c "CREATE DATABASE $db_name" >/dev/null; then
		echo "cannot reach or create $db_name; if Postgres is up, create it once with:" >&2
		echo "  sudo -u postgres createdb --owner=tadmor $db_name" >&2
		exit 1
	fi
fi
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
