# End-to-End / UI Testing — Design

**Status:** Adopted 2026-06-26.
**Scope:** Browser-driven UI testing for the `web/` front end — the tooling
choice, its supply-chain justification, the WSL setup, and the test structure.

This is the committed decision plus the reasoning, in the spirit of
`docs/frontend-stack.md`: a future reader should see *why* UI testing looks the
way it does without re-deriving it. It governs `e2e/`.

---

## 1. Context and the question

With the master-data screens (accounts, customers, suppliers, products) reaching
full list + create + edit, it was time to start testing the UI — and to let the
agent drive that testing too, not only a human clicking through manually. The
opening question was concrete: **do we need a browser-automation solution on WSL,
and if so, which one — given this repo's strict supply-chain posture?**

To test a UI programmatically you need a real browser plus an automation layer.
The only real questions were *which* layer, and how it squares with the
dependency policy that governs everything else here (stdlib-first backend;
hardened, minimal, vendored front end — see `docs/frontend-stack.md`).

---

## 2. The decision (summary)

- **Tool:** Playwright (`@playwright/test`), pinned to an exact version.
- **Isolation:** its own `e2e/` project with its own `pnpm-lock.yaml`, kept out
  of `web/`'s runtime dependency tree and `pnpm audit` surface.
- **Hardening:** mirrors `web/` — 7-day publish cooldown, frozen lockfile, pinned
  registry, blocked install scripts, corepack-pinned pnpm + Node major.
- **Browser:** Playwright's own version-matched Chromium, fetched by an explicit
  `pnpm install-browser` command (not an install script).
- **Run mode:** headless Chromium (works on WSL2 regardless of WSLg).
- **Cleanup:** a `globalTeardown` deletes throwaway test rows via `psql`, because
  the app has no hard-delete for master data (§6).

---

## 3. Why Playwright — and the reasoning that nearly went the other way

This decision was *reversed mid-discussion*, and the reversal is the instructive
part, so it is recorded honestly.

**First instinct (rejected): hand-roll it.** The initial lean was to avoid a
heavy test framework and instead drive a system browser over the Chrome DevTools
Protocol (CDP) with a small handwritten driver (~200–300 lines of Go), keeping UI
testing inside the hermetic, no-npm Go world. The arguments were (a) Playwright is
big, and (b) supply-chain risk applies at *development time* too, not just
runtime — a test tool runs a large codebase in the dev shell with full FS/network
access every time it runs.

**Why that was wrong.** The (b) concern is real and worth stating, but (a) used
the wrong metric. The metric this repo optimizes — explicit in
`frontend-stack.md` — is **distinct maintainers/vendors trusted, not lines of
code.** A large, self-contained bundle from one reputable vendor is categorically
different from a sprawl of small maintainers (the "transitive bloat" threat). So
"it's big, keep it out" is not a supply-chain argument. The real question is: does
Playwright fan out into many vendors?

**The empirical check settled it.** Inspecting the actual dependency declarations:

```
playwright        → playwright-core   (+ optional fsevents, macOS-only)
playwright-core   → (no dependencies)
```

The `pnpm install` confirmed it resolves to **exactly three packages** —
`@playwright/test → playwright → playwright-core`, all Microsoft — plus a
macOS-only optional `fsevents` that does not install on Linux. **One vendor, zero
transitive fan-out.** That is *better* on this repo's own metric than things
already accepted into the front end (e.g. react-router added 3 maintainers;
radix/vite/esbuild/echarts are each broader).

**The deciding precedent.** `frontend-stack.md` §4.10 reversed an earlier
hand-rolled-router lean with the rule: *a handwritten alternative only wins when
it truly does the same job in tens of lines.* Robust browser automation
(auto-waiting, a selector engine, click/navigation edge cases) is exactly the
shape that exceeds that — the "~200–300 lines" estimate was the optimistic
version, just as the hand-rolled router looked small until it wasn't. So
Playwright is the *consistent* choice, not an exception to the policy.

**Residual dev-time concern, addressed.** Build/dev-time execution risk is real,
but it is one reputable vendor's code in the dev shell — qualitatively the same
trust already extended to vite/esbuild at build time. Acceptable by the project's
own standard.

---

## 4. Why let Playwright download its own browser

A sub-question: should the browser come from the system (e.g. apt's signed
Chrome) or from Playwright's own download?

**Decision: let Playwright download its version-matched Chromium**, via an
explicit `pnpm install-browser` (`playwright install chromium`).

- **No new vendor.** The build comes from Microsoft — the same vendor as the npm
  package and (under WSL) the OS itself. Pointing at a *system* `google-chrome`
  would instead drag in **Google** as a separate browser vendor, for no benefit.
- **It does *not* poke a hole in script-blocking.** `playwright install` is a
  deliberate CLI command, not a postinstall lifecycle script. The `ignore-scripts`
  / cooldown hardening stays fully intact; the browser is simply not fetched
  during `pnpm install`.
- **More robust than system Chrome.** Playwright ships *patched* Chromium
  version-matched to the npm release. Stock Chrome auto-updates and periodically
  drifts ahead of what a pinned Playwright supports, causing flakes; the managed
  browser avoids that.

**Accepted costs:** the browser binary is not covered by the lockfile integrity
hash (it is version-locked to the pinned, cooldown-gated Playwright release, from
Microsoft's CDN), and CDN availability is a build-continuity dependency — the same
class of risk already accepted for Tier-1 npm in `frontend-stack.md` §4.3.

---

## 5. WSL specifics

The dev environment is WSL2 (Ubuntu 24.04), which shaped a couple of details:

- **Headless needs no display.** Headless Chromium runs on WSL2 regardless of
  WSLg. WSLg *is* present (`DISPLAY=:0`, Wayland), so `--headed` runs also work
  and surface on Windows, but headless is the default and the CI-relevant mode.
- **OS shared libraries are the one privileged step.** A headless browser needs
  system libs (`libnss3`, `libatk`, …) not present on a fresh WSL Ubuntu. These
  are installed once with `sudo … playwright install-deps chromium` — the only
  part of the setup requiring root. (Run it preserving PATH so the fnm-managed
  Node is found: `sudo env "PATH=$PATH" corepack pnpm exec playwright install-deps
  chromium`.)
- **Node toolchain.** Node is fnm-managed (per `frontend-stack.md` §4.9), so it is
  not on a non-interactive shell's PATH by default — relevant when scripting test
  runs outside an interactive shell.

---

## 6. Test structure and the "no hard-delete" finding

Tests live in `e2e/tests/`. Three files today:

- **`smoke.spec.ts`** — the app boots and a representative screen renders (the
  Chart of Accounts heading + the five primary nav links). No data assertions;
  just that the shell and a screen mount.
- **`customers.spec.ts`** — the customer lifecycle through the UI: **create**
  (pick a throwaway organization in the New Customer form, set fields, assert the
  new row lists), **edit** (change customer number + currency, assert the list
  updates), and **deactivate**.
- **`auth.spec.ts`** — the login screen and session lifecycle: an
  unauthenticated visit shows the sign-in form (never the app), a wrong
  password is rejected, and a sign-in/sign-out round trip works.

**Authentication.** The API requires a login session, so a `globalSetup`
(`global-setup.ts`) runs before any test: it upserts a dedicated
`e2e@tadmor.test` user via `psql` with a password that is random per run
(hashed with Node's built-in `pbkdf2Sync` in exactly the format
`internal/auth` verifies — no new dependency), logs in once over the API, and
saves the session cookie as Playwright *storage state*
(`e2e/.auth/state.json`, gitignored). Every browser context and API `request`
fixture starts from that state, so the pre-auth specs are untouched.
`auth.spec.ts` opts back out with an empty `storageState` to exercise the real
form, using the run's password from `E2E_PASSWORD`. Teardown deletes the e2e
user (its sessions cascade).

**The "delete" finding.** The task was "create/edit/delete" specs, but there is
**no hard-delete** anywhere: the backend `master.go` registers only GET/POST/PUT
for customers and organizations, and neither the UI nor the API client exposes a
delete. This is correct by design for an accounting system — a customer may carry
ledger history, so its lifecycle end-state is **deactivation** (`is_active =
false`, shown as an Active/Inactive badge), not removal. The third test therefore
exercises deactivation as the faithful "delete" flow, and is named accordingly
rather than pretending a hard-delete exists.

**Setup and isolation.** `tests/helpers.ts` provides API helpers
(`createOrganization`, `createCustomer`) so the edit/deactivate tests don't have
to re-drive the create form. The customer create form only offers organizations
that don't already have a customer (a UNIQUE constraint), so each test creates its
own throwaway organization via the API, named with a unique `E2E-` prefix — both a
collision-free anchor for locating the row and a marker for cleanup.

**Cleanup via psql.** Because the app cannot delete the rows it creates, a
`globalTeardown` (`global-teardown.ts`) removes the `E2E-`prefixed organizations
and their customers directly via `psql` after the run. `psql` is a system tool, so
this adds **no npm dependency**. Verified: a full run leaves zero `E2E-` rows
behind.

> **CI caveat.** The teardown defaults to the **dev** database (the one `make run`
> uses), which is fine for local runs. For CI, point `E2E_DATABASE_URL` at a
> dedicated test database so a teardown bug can never touch real data.

---

## 7. Layout, hardening, and how to run

```
tadmor/
  e2e/
    package.json            # tadmor-e2e; @playwright/test pinned exact; pnpm + Node pins
    .npmrc                  # frozen lockfile, pinned registry, ignore-scripts (mirrors web/)
    pnpm-workspace.yaml     # 7-day cooldown; onlyBuiltDependencies: [] (no auto browser download)
    .nvmrc                  # Node 22
    .gitignore              # ignores node_modules/, test-results/, playwright-report/; keeps lockfile
    pnpm-lock.yaml          # committed (the integrity-pinned source of truth)
    playwright.config.ts    # chromium-only, headless, BASE_URL-overridable, globalSetup/Teardown, storageState
    global-setup.ts         # seeds the e2e login user (psql) + saves the authenticated storage state
    global-teardown.ts      # psql cleanup of E2E- rows and the e2e login user
    run-local.sh            # one-shot orchestrator for `make e2e` (build+run server, test, tear down)
    tests/
      helpers.ts            # API setup helpers + E2E_PREFIX + E2E_EMAIL
      smoke.spec.ts
      customers.spec.ts
      auth.spec.ts
    README.md               # setup + run instructions
```

The hardening mirrors `web/` exactly (cooldown, frozen install, blocked scripts,
corepack/Node pins). Playwright is pinned to an **exact** version (chosen older
than the 7-day cooldown window).

**One-time setup** (from `e2e/`):

```sh
corepack pnpm install --no-frozen-lockfile          # first time only (no lockfile yet)
corepack pnpm install-browser                        # download Playwright's Chromium
sudo env "PATH=$PATH" corepack pnpm exec playwright install-deps chromium   # OS libs (root)
```

**Running — the one-shot way (preferred).** With Postgres up, `make e2e` does the
whole run itself and needs nothing else standing:

```sh
make e2e              # build+run the embedded-SPA server, wait for it, test, tear down
```

`run-local.sh` builds the Go binary (which embeds `web/dist`), starts it on
:8080, waits for it to accept connections, runs the suite against it, and
**always** tears the server down again via a shell `trap` (even on test failure
or Ctrl-C). `DATABASE_URL` and `BASE_URL` are overridable. Because the run is a
single `make` invocation, it also stays inside the agent's `make:*` permission
allowlist — one command, no per-step prompts.

**Running — against an already-running stack.** If the stack is already up (Vite
dev on :5173 proxying /api → Go on :8080, with Postgres), run the tests alone:

```sh
make web-dev          # :5173
make run              # :8080 Go API
make e2e-test         # or: cd e2e && corepack pnpm test

# Against the embedded production build served by the Go binary instead:
BASE_URL=http://localhost:8080 corepack pnpm test
```

`make e2e-install` and `make e2e-test` mirror the `web-*` Makefile targets;
`make e2e` is the self-contained superset that also manages the server.

---

## 8. Residual risks / notes

- **Build-time execution** of Playwright remains (one trusted vendor) — same class
  as vite/esbuild, mitigated by cooldown + pinning.
- **Browser binary** is outside lockfile integrity (version-locked to the pinned
  release; Microsoft CDN). Build-continuity, not malicious-code, risk.
- **Teardown targets the dev DB by default** — see the §6 CI caveat.
- **Coverage is thin** — smoke + the customer lifecycle. Suppliers and products
  share the identical form pattern, so mirroring those specs is straightforward
  follow-on work.

---

## 9. Status and next step

Done: `e2e/` scaffolded and committed with the hardened pnpm config and Makefile
targets; Chromium installed; smoke specs and the customer create/edit/deactivate
specs written and **verified green (5/5)** against the dev stack, with teardown
confirmed to leave zero test rows.

Next (when wanted): mirror the create/edit/deactivate specs for suppliers and
products; consider a dedicated CI test database via `E2E_DATABASE_URL`.
