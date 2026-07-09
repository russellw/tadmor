# tadmor

Business management software: a Postgres + Go backend with a TypeScript/React
front end, shipped as a single static binary that embeds the built SPA.

## Layout

```
cmd/server/        main entry point (HTTP server + migration-on-startup)
internal/          the backend's domain packages: config, db, httpapi,
                   master, documents, orders, posting, reporting, banking,
                   pdf, printing, auth
db/migrations/     ordered SQL migrations (see db/README.md)
db/seed/           reference-data seeds (ISO countries/currencies)
web/               the TypeScript/React front end (see docs/frontend-stack.md)
e2e/               browser-driven UI tests (Playwright; see docs/e2e-testing.md)
deploy/            deployment assets for the fixed-price VPS
docs/              architecture, deployment, and development docs
vendor/            all third-party Go source, committed and reviewable
```

## Prerequisites

- **Go 1.25.11** (pinned in `go.mod` via the `toolchain` directive)
- **Postgres 16 or 17** reachable via `DATABASE_URL` (the deployment box runs Debian 13's Postgres 17; the suite passes on both)

## Configuration

| Env var          | Required | Default   | Purpose                          |
| ---------------- | -------- | --------- | -------------------------------- |
| `DATABASE_URL`   | yes      | —         | Postgres connection string       |
| `HTTP_ADDR`      | no       | `:8080`   | HTTP listen address              |
| `PORT`           | no       | —         | Listen port; overrides `HTTP_ADDR` when set (Cloud Run injects it) |
| `TEST_DATABASE_URL` | for tests | — | Database the integration tests reset and use |

## Build, run, test

A `Makefile` wraps the common tasks with the pinned toolchain and a hermetic
(offline, vendored) environment. Run `make` to list targets:

```sh
make build      # build bin/server (embeds whatever is in web/dist)
make release    # build the front end into web/dist, then the server
make run        # build and run the server
make test       # full test suite from scratch (integration tests reset the DB)
make web-build  # build only the front end into web/dist
make web-check  # type-check + audit the front end
make vet        # go vet
make fmt        # gofmt -w
```

`make build` embeds the current contents of `web/dist`; use `make release`
(or run `make web-build` first) whenever the front end has changed.

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

## Deployment

The server is self-contained: the Go binary embeds the built SPA and the schema
migrations and serves the API, so the deployable artifact is a single static
binary (a container build also exists; see the repo-root `Dockerfile`). The demo
runs at https://tadmor.belunaro.com on a fixed-price VPS behind Caddy —
`make deploy` redeploys it; see [`docs/deployment.md`](docs/deployment.md) for
the full setup. A guest account (`guest@demo` / `guest123`, non-admin) is open
for anyone who wants to poke around the demo — edit freely, the database is
reseeded nightly.

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
