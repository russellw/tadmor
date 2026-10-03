#!/usr/bin/env bash
# Run the conformance suite against tadmor itself, from scratch.
#
# The suite needs a freshly initialized instance (spec/README.md), so this
# script wipes a dedicated database, builds the server, bootstraps one
# administrator with a random password, starts the server with email sending
# disabled, runs the suite, and always tears the server down again. Extra
# arguments are passed to the suite (e.g. -v, or -run 'banking').
#
# Overridable via the environment:
#   DATABASE_URL  must name a database whose name ends in _conformance, as a
#                 guard against wiping anything else (default:
#                 postgres://tadmor:tadmor@127.0.0.1:5432/tadmor_conformance)
#   HTTP_ADDR     where the server listens (default 127.0.0.1:8091)
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

# Match the Makefile's hermetic, vendored Go build.
export PATH="/usr/local/go/bin:$PATH"
export GOFLAGS="-mod=vendor"
export GOTOOLCHAIN="local"
export GOPROXY="off"

DATABASE_URL="${DATABASE_URL:-postgres://tadmor:tadmor@127.0.0.1:5432/tadmor_conformance?sslmode=disable}"
HTTP_ADDR="${HTTP_ADDR:-127.0.0.1:8091}"

db_name="${DATABASE_URL##*/}" # strip scheme://user:pass@host:port/
db_name="${db_name%%\?*}"     # strip ?params
if [[ "$db_name" != *_conformance ]]; then
	echo "refusing to wipe database '$db_name': its name must end in _conformance" >&2
	exit 1
fi

if ! psql "$DATABASE_URL" -qAt -c 'SELECT 1' >/dev/null 2>&1; then
	echo "==> Creating database $db_name"
	admin_url="${DATABASE_URL/\/$db_name/\/postgres}"
	if ! psql "$admin_url" -qAt -c "CREATE DATABASE $db_name" >/dev/null; then
		echo "cannot reach or create $db_name; if Postgres is up, create it once with:" >&2
		echo "  sudo -u postgres createdb --owner=tadmor $db_name" >&2
		exit 1
	fi
fi

echo "==> Wiping $db_name"
psql "$DATABASE_URL" -qAt -v ON_ERROR_STOP=1 \
	-c 'SET client_min_messages = warning; DROP SCHEMA public CASCADE; CREATE SCHEMA public;' >/dev/null

echo "==> Building server"
go build -o bin/server ./cmd/server

email="admin@conformance.test"
password="$(head -c 18 /dev/urandom | base64 | tr -d '/+=')"
echo "==> Bootstrapping administrator (migrates the empty database)"
printf '%s\n' "$password" | DATABASE_URL="$DATABASE_URL" ./bin/server -adduser -email "$email" -name 'Conformance Admin' >/dev/null

echo "==> Starting server on $HTTP_ADDR (email disabled)"
env -u SMTP_ADDR -u SMTP_USER -u SMTP_PASS -u MAIL_FROM -u PORT \
	DATABASE_URL="$DATABASE_URL" HTTP_ADDR="$HTTP_ADDR" ./bin/server >"$repo_root/bin/conformance-server.log" 2>&1 &
server_pid=$!
trap 'kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true' EXIT

host="${HTTP_ADDR%%:*}"
port="${HTTP_ADDR##*:}"
for _ in $(seq 1 30); do
	if ! kill -0 "$server_pid" 2>/dev/null; then
		echo "server exited before becoming ready; see bin/conformance-server.log" >&2
		exit 1
	fi
	if (exec 3<>"/dev/tcp/$host/$port") 2>/dev/null; then
		exec 3>&- 3<&-
		break
	fi
	sleep 1
done

echo "==> Running the conformance suite"
go run ./conformance -base-url "http://$HTTP_ADDR" -email "$email" -password "$password" "$@"
