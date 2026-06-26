# tadmor end-to-end / UI tests

Playwright-driven browser tests for the `web/` front end.

## Why this is a separate project (not in `web/`)

Playwright is a **test tool**, never shipped to end users. Keeping it in its own
project with its own `pnpm-lock.yaml` keeps it out of `web/`'s audited *runtime*
dependency tree (and out of `pnpm audit` of the shipped bundle).

Supply-chain note: on the metric this repo optimizes — *distinct maintainers
trusted* (see `docs/frontend-stack.md`) — Playwright is cheap. Its entire npm tree
is `@playwright/test` → `playwright` → `playwright-core` (all Microsoft) plus a
macOS-only optional `fsevents` that does not install on Linux. One vendor, zero
fan-out. The browser binary is downloaded from Microsoft's CDN by an explicit,
auditable command (not an install script) — adding no vendor beyond the Microsoft
we already trust for the toolchain and OS.

The same hardening as `web/` applies here: 7-day publish cooldown
(`pnpm-workspace.yaml`), frozen lockfile + pinned registry + blocked install
scripts (`.npmrc`), corepack-pinned pnpm and Node major (`package.json`,
`.nvmrc`). Playwright is pinned to an exact version (`1.61.0`).

## One-time setup

From `e2e/` (fnm auto-switches to Node 22 on `cd` via `.nvmrc`):

```sh
# 1. Install the npm package (first time only: no lockfile yet, so unfrozen).
#    This does NOT download the browser — ignore-scripts blocks Playwright's
#    postinstall by design.
corepack pnpm install --no-frozen-lockfile     # subsequent installs: just `corepack pnpm install`

# 2. Download Playwright's version-matched Chromium (explicit, auditable).
corepack pnpm install-browser

# 3. Install the OS shared libraries the headless browser needs (root required).
#    `env "PATH=$PATH"` keeps the fnm-managed node on sudo's PATH.
sudo env "PATH=$PATH" corepack pnpm exec playwright install-deps chromium
```

## Running

Start the full stack first (in separate shells):

```sh
make web-dev                         # Vite dev server on :5173 (proxies /api → :8080)
make run                             # Go backend on :8080 (needs Postgres up)
```

Then, from `e2e/`:

```sh
corepack pnpm test                   # headless run against http://localhost:5173
corepack pnpm test:headed            # watch it in a real browser window (WSLg)
corepack pnpm report                 # open the HTML report after a failure

# Against the embedded production build served by the Go binary instead:
BASE_URL=http://localhost:8080 corepack pnpm test
```

Failure artifacts (screenshots, traces, HTML report) land in `test-results/` and
`playwright-report/`, both gitignored.
