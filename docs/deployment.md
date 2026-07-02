# Deployment — Demo Hosting on Cloud Run

How to deploy tadmor as a low-cost demo: the Go server (which embeds the SPA and
serves the API) as a single container on Google Cloud Run, backed by a serverless
Postgres on Neon.

For the local dev loop see `docs/local-development.md`. The container build lives
in the repo-root `Dockerfile` / `.dockerignore`.

---

## 1. The shape of the deployment

One container, one database, both scaling to zero when idle:

| Piece | What runs it | Cost when idle |
|---|---|---|
| Go server + embedded SPA | Cloud Run (`--min-instances 0`) | $0 (scaled to zero) |
| Postgres | Neon free tier | $0 (auto-suspends) |

The server is self-contained: the Go binary embeds `web/dist` (`//go:embed
all:dist`) *and* serves `/api/*`, so there is no separate static host. It reads
its Postgres connection string from `DATABASE_URL` and applies pending migrations
from `db/migrations/` on startup — the runtime image carries that directory.

The tradeoff for paying nothing while idle is a couple-second cold start on the
first request after inactivity (Cloud Run wake + Neon wake).

---

## 2. The container build

The repo-root `Dockerfile` is a 3-stage build that mirrors the Makefile's
discipline:

1. **SPA** — `node:22` + corepack-pinned `pnpm@10.18.0`, installed from the
   frozen lockfile, `pnpm build` → `web/dist`.
2. **Go** — `golang:1.25.11`, hermetic and vendored (`GOFLAGS=-mod=vendor`,
   `GOPROXY=off`, `GOTOOLCHAIN=local`, `CGO_ENABLED=0`), embedding the SPA from
   stage 1.
3. **Runtime** — `gcr.io/distroless/static:nonroot` carrying just the binary and
   `db/migrations/`.

Validate it locally before the first deploy (requires a running Docker daemon):

```bash
docker build -t tadmor .
```

If you would rather not run Docker locally, skip this — `gcloud run deploy
--source .` builds the same Dockerfile remotely on Cloud Build (§4).

### 2.1 On pinning the base images (deferred, deliberately)

The three base images float on their tags: `golang:1.25.11-bookworm` pins the Go
version but not the Debian layer under it, `node:22-bookworm-slim` pins only the
major, and `gcr.io/distroless/static:nonroot` floats entirely. They could be
pinned by digest (`golang:1.25.11-bookworm@sha256:...`). We have **not** done so,
and this section records why, so it isn't mistaken for an oversight.

What pinning would and wouldn't buy:

- **It does not affect dependency versions.** Go deps come from the committed
  `vendor/` tree with `GOPROXY=off` (fails closed if anything is missing); npm
  deps come from the frozen `pnpm-lock.yaml`; pnpm itself is corepack-pinned.
  None of that floats regardless of the base images.
- **The real gap it closes is toolchain integrity.** The `golang` and `node`
  images are *builders* — they compile the binary and build the SPA. A poisoned
  builder could inject code into the shipped artifacts, and vendoring plus a
  frozen lockfile do **nothing** against that: they verify the dependencies, not
  the compiler. A repointed official-image tag (Docker Hub push-access
  compromise is the concrete precedent) is exactly the vector that path leaves
  open. Digest pinning means a rebuild keeps using the known-good image instead
  of silently ingesting a swapped one.
- **But it is an exposure-window reduction, not a cure.** It only protects until
  the next deliberate re-pin, at which point trust re-extends to whatever is
  current; it assumes the digest pinned today was itself verified against
  Docker's published digest (pinning whatever the wire happened to serve just
  freezes an unaudited snapshot); and freezing the *runtime* image means it stops
  receiving OS security patches until bumped. So pinning without a renovate-style
  bump discipline trades one risk for another.

Calibration: **low probability, high impact.** For a throwaway demo the expected
value is small enough that pinning here would be closer to theater than defence,
so we skip it. When this project moves to a real production deployment, pin the
two **builder** images by verified digest (that is the part vendoring cannot
cover), with a deliberate process for bumping them — and justify it
**reproducibility-first** (a rebuild months later is provably the image you
audited, benefit with probability 1), **toolchain-integrity-second** (the
low-probability attack above).

---

## 3. One-time setup

### 3.1 Provision the Neon database

Create a Neon project and database, then copy its connection string. Use the
**direct** endpoint (not the `-pooler` one): pgx v5's default prepared-statement
caching misbehaves against Neon's PgBouncer pooler, and at demo scale
(`--max-instances 1`, a single `pgxpool`) you stay well under Neon's connection
limit, so the pooler buys nothing. Require TLS with `sslmode=require`:

```
postgresql://USER:PASS@ep-xxx.REGION.aws.neon.tech/tadmor?sslmode=require
```

### 3.2 Store the connection string as a secret

Never bake `DATABASE_URL` into the image. Put it in Secret Manager:

```bash
printf 'postgresql://USER:PASS@ep-xxx.REGION.aws.neon.tech/tadmor?sslmode=require' \
  | gcloud secrets create tadmor-database-url --data-file=-
```

Grant the Cloud Run runtime service account read access (skip if `gcloud`
prompts to do this for you during deploy):

```bash
PROJECT_NUMBER=$(gcloud projects describe "$(gcloud config get-value project)" \
  --format='value(projectNumber)')
gcloud secrets add-iam-policy-binding tadmor-database-url \
  --member="serviceAccount:${PROJECT_NUMBER}-compute@developer.gserviceaccount.com" \
  --role=roles/secretmanager.secretAccessor
```

---

## 4. Deploy

Cloud Build sees the `Dockerfile` and builds the image; Cloud Run runs it:

```bash
gcloud run deploy tadmor \
  --source . \
  --region us-central1 \
  --allow-unauthenticated \
  --min-instances 0 \
  --max-instances 1 \
  --memory 512Mi \
  --set-secrets DATABASE_URL=tadmor-database-url:latest
```

Redeploying after a change is the same command — rerun it from a clean working
tree.

### Why these flags

- `--min-instances 0` — scale to zero so an idle demo costs nothing.
- `--max-instances 1` — demo scale, and it sidesteps a migration race: `db.Apply`
  takes no cross-instance lock, so two instances cold-starting at once could both
  try to apply the same migration. One instance removes the race entirely.
- `--allow-unauthenticated` — a public demo; drop this for a private one.
- `--set-secrets` — injects `DATABASE_URL` from Secret Manager at runtime.

The port needs no flag: Cloud Run injects `PORT` (default 8080) and
`config.Load` honors it, overriding the `HTTP_ADDR` default.

---

## 5. Scaling past a demo

If you later raise `--max-instances`, two things change:

- **Connections** — switch to Neon's `-pooler` endpoint, and set the pool's query
  exec mode to simple protocol (`pgx.QueryExecModeSimpleProtocol`) so prepared
  statements work through PgBouncer.
- **Migrations** — give `db.Apply` a Postgres advisory lock, or run migrations as
  a one-off step before rolling out, so concurrent instances don't race.
