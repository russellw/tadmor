# Conventions for a new project

This document is self-contained and meant to be **copied into the repo of any
new project**, so that a Claude Code session working there starts with the
technology choices, dependency policies, and hard-won findings from the tadmor
project instead of re-deriving (or worse, contradicting) them. Its companion is
`belunaro-app-deployment.md`, which covers hosting; copy both.

The canonical copies live in the tadmor repo under `docs/`; if a policy
changes, update it there and re-copy.

**How to read this:** the policies in §2–§4 are standing decisions — follow
them unless the user says otherwise. The concrete library picks in §6 were
made for tadmor under stated assumptions; they are the **defaults**, but
re-check the assumptions (marked ⚠) against the new project before inheriting
them silently.

---

## 1. Technology choices

- **Database:** Postgres.
- **Back end:** Go, standard library first (e.g. `net/http` with 1.22+
  `ServeMux` routing, `encoding/json`, `log/slog`, `crypto/*`) — no frameworks.
- **Front end:** TypeScript + React when there is a significant UI. A tool
  with no real UI doesn't need a front end at all — don't scaffold `web/`
  speculatively.
- **Ancillary scripts:** Python as necessary.

**Schema design:** where a good natural key presents itself (e.g. the ISO
2-letter country code), use it. Where a synthetic key is needed, use an
auto-incrementing integer — this forgoes the distributed-merge advantages of
UUIDs, but gains performance and ease of debugging.

**Version control:** commit directly to the default branch. No feature
branches, no PR workflow.

**Hosting:** fixed-price only — the bill must be bounded by construction.
Hyperscalers (GCP/AWS/Azure) are categorically refused, including their
"serverless" tiers. Apps deploy to the shared belunaro VPS; see
`belunaro-app-deployment.md`.

---

## 2. The supply-chain posture (the organizing principle)

Everything below flows from one concern: supply-chain attacks. The unifying
principle, quoted from tadmor's ratified front-end design:

> **Minimize the number of distinct maintainers whose code we must trust**,
> then pin and integrity-verify what remains, and give any compromised release
> a window to be caught before we can pull it.

Findings that refine how to apply it (each was corrected/confirmed explicitly
during tadmor):

1. **The metric is distinct maintainers/vendors trusted — not lines of code.**
   "It's big, keep it out" is not a supply-chain argument. A large
   self-contained bundle from one reputable vendor is categorically fine; the
   thing to fear is a sprawling tree of many small maintainers. Concrete case:
   Playwright was initially rejected as "heavy," but inspection showed it
   resolves to 3 packages, all Microsoft, zero transitive fan-out — better on
   this metric than dependencies already accepted. It was adopted.
2. **Dev-time counts too.** Tooling runs on the dev machine with full
   filesystem and network access. Judge build/test tools by the same
   maintainer-count metric as runtime deps.
3. **Measure, don't guess.** Before accepting or rejecting a package, inspect
   its *actual* production transitive tree and count distinct maintainers
   (`pnpm why`, the lockfile, `go mod graph`). Several tadmor decisions turned
   on this measurement coming out differently than intuition suggested.
4. **Threat model, in order of real-world frequency:** malicious install
   scripts; compromised maintainer / hijacked package publishing a bad
   version; typosquatting / dependency confusion; transitive bloat making
   audit meaningless; and (public-facing apps only) a compromised runtime dep
   shipped to end users' browsers.

---

## 3. Dependency decision heuristics

- **Prefer a few tens of lines of handwritten code over a dependency** when it
  genuinely does the same job. This applies to the front end too. When
  proposing the handwritten path, name the concrete trigger that should make
  us reconsider the dependency later.
- **The "start as we mean to continue" nuance:** when the need is clearly
  coming and the eventual dependency is inevitable, taking the standard
  dependency up front beats a stopgap handwritten version that must be
  migrated off later. tadmor's router decision went this way — a hand-rolled
  History-API router stays small only for flat top-level screens; route
  params, nested layouts, link-click edge cases, and navigation guards push
  the honest scope well past "tens of lines," so react-router-dom was adopted
  immediately rather than after a painful migration.
- **The boundary between those two rules:** a handwritten alternative only
  wins when it *truly* does the same job in tens of lines. Routing and robust
  browser automation both exceed that; a debounce helper or a tiny date
  formatter does not.
- **New dependencies need a conversation first.** On the Go side this is
  absolute (see §4). On the npm side, weigh candidates by measured transitive
  maintainer count and raise it with the user before installing.

---

## 4. Back-end (Go) policy

- **Allowed third-party modules:** `github.com/jackc/pgx/v5` (the Postgres
  driver) and anything under `golang.org/x/*`. Nothing else without a
  conversation first.
- **Pin and vendor everything:** exact versions in `go.mod`/`go.sum`,
  `go mod vendor` with the `vendor/` tree **committed** to git. Committing
  `vendor/` is correct for Go (small — tadmor's is ~8 MB / 5 modules — flat,
  pure source, deterministic): it gives reproducible offline builds and
  auditable dependency diffs.
- **Hermetic builds.** The Makefile exports, for every target:

  ```make
  export GOFLAGS := -mod=vendor
  export GOTOOLCHAIN := local
  export GOPROXY := off
  ```

  No module or toolchain downloads happen during builds. Go itself is
  installed at `/usr/local/go` and the version pinned by `toolchain` in
  `go.mod` matching it.

- **Password hashing:** PBKDF2 from the stdlib (`crypto/pbkdf2` in Go 1.24+,
  or ~20 lines over `crypto/hmac`) — don't pull a hashing library.
- **Migrations: embed them in the binary** (`//go:embed`) and apply at
  startup, treating zero migration files found as a **fatal error**. Hard
  lesson: tadmor read `./db/migrations` from the CWD, which silently no-ops
  under systemd where the working directory isn't the repo — the deploy
  looked fine and did nothing.
- **Config via environment variables only** (`DATABASE_URL`, `HTTP_ADDR`,
  ...). No config files. This is what the deployment model assumes.
- **Server binary is static and self-contained:** `CGO_ENABLED=0`, front-end
  bundle embedded (§6.8). One file to deploy.

---

## 5. Front-end vendoring & hardening policy (npm ecosystem)

The front end gets as close to the Go posture as npm allows. These rules are
ratified and carry over as-is:

### 5.1 What goes in git

- **Commit `pnpm-lock.yaml`; never commit `node_modules/`.** They look like
  the same "vendor it" idea but aren't: `node_modules/` is a huge build
  artifact containing platform-specific native binaries (a copy committed from
  one OS is wrong on another), whereas the lockfile carries exact versions
  **plus per-tarball integrity hashes** — it is the real `go.sum`+`vendor/`
  analog. `dist/` and caches are ignored too.
- This is "Tier 1" vendoring. Tier 2 (a registry/mirror we control) adds only
  availability and egress control — business-continuity properties, not
  malicious-code protection (a mirror serves whatever is pinned, faithfully) —
  and was judged not worth the operational cost for a solo project. Clean
  upgrade path if that ever changes.

### 5.2 Package manager: pnpm 10, corepack-pinned

`packageManager: "pnpm@..."` in `package.json` makes every machine use the
same integrity-verified pnpm binary — the `GOTOOLCHAIN=local` analog. pnpm
over npm/Yarn because it blocks install scripts by default and has a native
publish cooldown.

### 5.3 Hardening knobs (copy these into every new front end)

- `pnpm-workspace.yaml`:
  - `minimumReleaseAge` — **7-day publish cooldown**; refuse any version
    published less than 7 days ago. The single most effective defense against
    the "compromised version published an hour ago" class. A fresh install
    refusing a week-old version is expected behavior, not a failure — pin
    older.
  - `onlyBuiltDependencies` — minimal install-script allow-list (e.g. just
    `esbuild`).
- `.npmrc`: `frozen-lockfile`, `prefer-frozen-lockfile`,
  `auto-install-peers=false`, `strict-peer-dependencies=true`,
  `ignore-scripts=true`, `registry=` pinned to npmjs (dependency-confusion
  guard).
- `package.json`: `packageManager` + `engines.node`; `.nvmrc` pins the Node
  major.

### 5.4 Node toolchain

Node managed per-user by **fnm**, reading the committed `.nvmrc`
(auto-switch via `fnm env --use-on-cd`); corepack (bundled with fnm's Node)
enables the pnpm pin. WSL notes: install Linux-native Node (Windows Node on
`PATH` can shadow it) and keep the repo on the Linux filesystem, not
`/mnt/c`, for speed and working file-watching.

### 5.5 Residual risks (acknowledged, not solved)

Script-blocking stops install-time code but the build still executes tooling;
a maliciously *pinned* version installs faithfully at every tier; the cooldown
only helps if a compromise is reported within the window. Mitigations:
cooldown on build deps too, `pnpm audit` as backstop.

---

## 6. Front-end stack defaults

⚠ **Context that shaped these picks** (re-check for the new project):
tadmor is public-facing (runtime deps ship to end users' browsers → serve own
bundle, CSP, minimal runtime tree), solo-developed (no team-velocity floor
forcing a big UI kit), and needs heavy grids/charts. If the new project
differs — internal-only, or no charts — some picks below relax or drop.

### 6.1 Build: Vite + React + TypeScript + Tailwind

No meta-framework (Next.js, Remix) — see §6.9.

### 6.2 UI components: shadcn/ui, source vendored in-repo

The shadcn CLI (run transiently via `pnpm dlx`, never a project dependency)
copies component **source** into `src/components/ui/`, committed and reviewed
like code we own. Rejected: batteries-included kits (MUI, Mantine, Chakra) as
large opaque runtime surfaces. Honest caveat: shadcn still sits on Radix
primitives — a real but smaller, individually pinned transitive tree.
Practical notes from tadmor: `shadcn add` only emits components — the theme
tokens `shadcn init` would write go into `src/index.css` by hand; the CLI
reads the **root** `tsconfig.json` for the `@/*` path alias (not
`tsconfig.app.json`).

### 6.3 Data grid: hand-rolled

Plain table + shadcn styling, adding sorting/filtering/virtualization as
screens demand. Zero added runtime trust surface. Fallback if hand-rolling
proves too slow: **TanStack Table** (headless, logic-only, single vendor).
Rejected: AG Grid (large opaque public bundle).

### 6.4 Charts: ECharts via `echarts-for-react`, lazy-loaded

Fewest distinct runtime maintainers among capable options (one large vendor +
one thin wrapper; the wrapper is replaceable with a ~30-line hook if wanted).
Code-split so it's not in the initial payload. Rejected: Recharts (wide
`d3-*` maintainer sprawl — worst on the trust metric), visx (too much code for
solo), Chart.js (capability). ⚠ If the new project has no heavy charts, skip
the dependency entirely.

### 6.5 Routing: react-router-dom v7

Adopted over hand-rolling (see §3) and over alternatives measured on
production transitive tree: react-router-dom adds 4 packages / 3 maintainers;
TanStack Router ~2.5× the packages for SSR/data features a client-only SPA
won't use; wouter is tiny but lacks loaders/nav-blocking and would be
outgrown exactly when the app gets complex.

### 6.6 E2E/UI testing: Playwright, isolated

`@playwright/test` pinned exact, in its **own `e2e/` directory with its own
lockfile** — kept out of the app's runtime dependency tree and audit surface.
Same hardening as `web/` (cooldown, frozen lockfile, pinned registry, blocked
scripts). Browser: Playwright's version-matched Chromium fetched by an
explicit command, not an install script; run headless (works on WSL2
regardless of WSLg). Supply-chain verdict: 3 packages, all Microsoft, zero
fan-out (§2.1).

### 6.7 API shape: namespace under `/api/` from day one

Register JSON routes on an inner mux mounted via `http.StripPrefix("/api", …)`.
tadmor originally served the API at the root and had to migrate when the SPA
arrived, because a catch-all `index.html` collides with root-level API routes.
Keep `/healthz` and `/readyz` at the root for probes.

### 6.8 Serving: embed the bundle in the Go binary, behind a CSP

`//go:embed all:dist` + an SPA handler (static file if it exists, else
`index.html` for client-side routing). No CDN — serving our own bundle removes
a runtime trust party. Set a same-origin CSP (`default-src 'self'`; tadmor
currently allows `'unsafe-inline'` for styles only) and
`X-Content-Type-Options: nosniff`. Keep a committed `dist/.gitkeep` so the
embed compiles before any front-end build. Dev loop: Vite dev server proxies
`/api` → the Go port, so the app calls same-origin `/api/*` in both dev and
prod; develop against the Vite port (hot reload), not the Go port.

### 6.9 No meta-framework / SSR

Next.js/Remix sell *running React on a server* (SSR/SSG/RSC): valuable for
public, SEO-sensitive, first-paint-sensitive content. A login-gated
line-of-business app needs none of it; the server already exists and is Go;
an SSR framework can't be embedded in the Go binary and would mean a second
Node runtime in production; and a large opaque framework surface contradicts
the maintainer-minimization principle. The kernel of the advice we keep:
don't hand-roll core plumbing like routing (hence §6.5). Accepted trade-offs:
manual code-splitting, no file-based routing. Revisit only if a genuinely
public, SEO-sensitive surface appears — as a *separate* deployable, not a
migration.

---

## 7. Testing and dev-loop conventions

- **Go tests run against real Postgres**, not mocks: a dedicated
  `<app>_test` database via `TEST_DATABASE_URL`; integration tests reset it.
  `make test` runs with `-count=1`.
- **The backend migrates and seeds itself on boot** — `make run` against the
  dev database is the whole backend setup after the one-time role/database
  creation.
- **Pin test data to seeded reference data** (tadmor lesson: date-dependent
  tests must pin dates inside the seeded accounting period, or they rot).
- **Makefile as the single entry point** for every dev task (`build`, `run`,
  `test`, `vet`, `fmt`, `web-*`, `e2e*`, `deploy`), with `## ` help comments
  and a `help` default target. Front-end targets go through
  `corepack pnpm ...` so the pin is always honored.
- **E2E cleanup:** if the app has no hard-delete, give Playwright a
  `globalTeardown` that removes throwaway rows via `psql`.

---

## 8. Suggested CLAUDE.md seed for the new project

```markdown
The goal of this project is to develop <what foo is>.

Technology stack:
Postgres for the database.
Go for the back end.
TypeScript and React for the front end.
Python as necessary for ancillary scripts.

Schema design:
Where a good natural key presents itself, such as the ISO 2 letter country
code, it shall be used.
Where a synthetic key is needed, it shall be an auto-incrementing integer.

Dependencies:
Supply-chain conscious throughout; keep the third-party footprint small,
pinned, and reviewable in-repo. See docs/new-project-conventions.md for the
full policy (backend: stdlib first, only jackc/pgx/v5 and golang.org/x/*
permitted, vendored, hermetic builds; frontend: hardened pnpm + vendored
shadcn source; new dependencies need a conversation first).

Deployment:
Deploys to the belunaro VPS; see docs/belunaro-app-deployment.md.

Version control:
Commit directly to the default branch. Do not create feature branches.
```
