# tadmor

Business management software. Postgres + Go backend, with a TypeScript/React
front end to come.

## Layout

```
cmd/server/        main entry point (HTTP server + migration-on-startup)
internal/config/   environment configuration
internal/db/       Postgres connectivity + the migration runner
internal/httpapi/  HTTP routes (net/http, no framework)
db/migrations/     ordered SQL migrations (see db/README.md)
vendor/            all third-party source, committed and reviewable
e2e/               browser-driven UI tests (Playwright; see docs/e2e-testing.md)
```

## Prerequisites

- **Go 1.25.11** (pinned in `go.mod` via the `toolchain` directive)
- **Postgres 16** reachable via `DATABASE_URL`

## Configuration

| Env var          | Required | Default   | Purpose                          |
| ---------------- | -------- | --------- | -------------------------------- |
| `DATABASE_URL`   | yes      | —         | Postgres connection string       |
| `HTTP_ADDR`      | no       | `:8080`   | HTTP listen address              |
| `TEST_DATABASE_URL` | for tests | — | Database the integration tests reset and use |

## Build, run, test

A `Makefile` wraps the common tasks with the pinned toolchain and a hermetic
(offline, vendored) environment. Run `make` to list targets:

```sh
make build    # build bin/server
make run      # build and run the server
make test     # full test suite from scratch (integration tests reset the DB)
make vet      # go vet
make fmt      # gofmt -w
```

Override connection strings on the command line, e.g.
`make test TEST_DATABASE_URL=postgres://user:pass@host:5432/db?sslmode=disable`.

The equivalent raw commands:

```sh
go build -o bin/server ./cmd/server
DATABASE_URL='postgres://tadmor:tadmor@127.0.0.1:5432/tadmor?sslmode=disable' go run ./cmd/server
TEST_DATABASE_URL='postgres://tadmor:tadmor@127.0.0.1:5432/tadmor_test?sslmode=disable' go test -count=1 ./...
```

The server applies any pending migrations on startup. Endpoints: `GET /healthz`
(liveness), `GET /readyz` (database reachable).

> The integration test **drops and recreates the `public` schema** of
> `TEST_DATABASE_URL`. Point it only at a throwaway database.

### UI / end-to-end tests

Browser-driven UI tests (Playwright) live in `e2e/`, in their own project so the
test tooling stays out of the front end's runtime dependency tree. With the stack
running, `make e2e-test` drives a headless browser against the app. See
[`docs/e2e-testing.md`](docs/e2e-testing.md) for one-time setup, the supply-chain
rationale, and the test structure.

## Dependency / supply-chain policy

Deliberately minimal and standard-library-first to limit attack surface.

- **Standard library** for HTTP, routing, JSON, logging, crypto.
- **Allowed third-party sources only:** `github.com/jackc/pgx/v5` (Postgres
  driver) and `golang.org/x/*`. Anything else requires discussion first.
- **Everything is vendored and pinned.** All third-party source lives in
  `vendor/` (committed, diffable in review); versions are locked in `go.sum`.
- **Builds are hermetic.** `vendor/` plus a pinned `toolchain` mean builds need
  no network. Set `GOTOOLCHAIN=local` to forbid toolchain auto-downloads and
  `GOPROXY=off` to forbid module fetches:

  ```sh
  go env -w GOTOOLCHAIN=local
  GOPROXY=off go build ./...   # verifies the vendor tree is complete
  ```

Current compiled dependency tree (6 modules): `pgx/v5`, `pgpassfile`,
`pgservicefile`, `puddle/v2`, `golang.org/x/sync`, `golang.org/x/text`.
