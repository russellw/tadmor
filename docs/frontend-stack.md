# Frontend Stack — Design

**Status:** Ratified 2026-06-19.
**Scope:** The React front end for tadmor (`web/`), and the supply-chain /
vendoring policy that governs it.

This is the committed design document. The blow-by-blow working notes live in
`docs/frontend-stack-design-notes.md`; this file is the distilled decision plus
the reasoning and the alternatives we rejected, so a future reader (or future us)
can see *why* the stack looks the way it does without re-deriving it.

---

## 1. Context and goals

The Go backend follows a strict, supply-chain-conscious dependency policy:
stdlib-first; only `jackc/pgx` + `golang.org/x` as third-party modules; everything
vendored and pinned; hermetic builds (`GOPROXY=off`, `GOTOOLCHAIN=local`,
`-mod=vendor`). The `vendor/` tree is committed to git.

The front end must get **as close to that posture as the npm ecosystem allows**,
without paying so much in developer velocity that a CRUD-heavy business app
becomes impractical to build.

Constraints that shaped the design (all confirmed 2026-06-19):

| Question | Answer | Consequence |
|---|---|---|
| Internal or public-facing? | **Public-facing** | Runtime supply chain matters: a compromised *runtime* dep ships to end users. Serve from the Go backend, not a CDN; add a CSP; keep runtime deps minimal. |
| Solo or team? | **Solo** | Free to minimize the ecosystem aggressively; no team-velocity floor forcing a big UI kit. |
| UI complexity? | **Heavy grids/charts** | Expands the dependency surface beyond a pure forms/tables minimal set. Forced the grid + chart decisions below. |
| Vendoring tier? | **Tier 1** | Commit the lockfile; CI installs frozen. No private mirror. |

---

## 2. The decision (summary)

A **hardened pnpm + shadcn/ui** stack:

- **Build:** Vite + React + TypeScript + Tailwind.
- **Package manager:** pnpm 10, corepack-pinned via `packageManager`.
- **Vendoring:** Tier 1 — commit `pnpm-lock.yaml`, ignore `node_modules/`.
- **Hardening:** install scripts blocked by default, 7-day publish cooldown,
  frozen lockfile, pinned registry.
- **UI components:** shadcn/ui — component *source* vendored into the repo.
- **Data grid:** hand-rolled (no dependency).
- **Charts:** ECharts via `echarts-for-react`, lazy-loaded.
- **Deployment:** bundle served from the Go backend behind a CSP.

The unifying principle: **minimize the number of distinct maintainers whose code
we must trust at runtime**, then pin and integrity-verify what remains, and give
any compromised release a window to be caught before we can pull it.

---

## 3. Threat model

What we are defending against, roughly in order of how often it actually happens:

1. **Malicious lifecycle scripts** (`postinstall` etc.) — attacker code runs at
   *install* time on dev machines and CI.
2. **Compromised maintainer / hijacked package** — a legitimate package ships a
   malicious version (event-stream, the colors/faker sabotage, the
   `@solana/web3.js` key-stealer).
3. **Typosquatting / dependency confusion** — the wrong package is installed, or
   an internal name is shadowed by a public one.
4. **Transitive bloat** — a tree of 1000+ packages cannot be meaningfully
   audited; trust becomes implicit and broad.
5. **Runtime delivery** (public-facing only) — a compromised runtime dependency
   is bundled and served to end users' browsers.

The stack below maps each major choice to the threats it addresses.

---

## 4. Decisions, reasoning, and alternatives

### 4.1 Git vendoring policy: commit Go `vendor/`, ignore `node_modules/`

The original question that kicked this off: *the Go `vendor/` dir is committed —
is that right, and should the (much larger) JS dependency tree be committed too?*

**Decision:** keep committing Go `vendor/`; **never** commit `node_modules/`.

They look like the same thing — "vendored dependencies in git" — but they are
not, and the reasons that make vendoring correct for Go do not transfer to npm.

- **Go `vendor/`** is small (currently ~8 MB, 5 modules), flat, pure source, and
  deterministic. Committing it gives reproducible offline builds (no dependency
  on the module proxy staying up), auditable dependency diffs, and `go build`
  uses it automatically. This is the recommended practice for supply-chain-
  conscious Go and we keep it.
- **`node_modules/`** is huge (hundreds of MB, tens of thousands of files),
  contains **platform/arch-specific native binaries** (esbuild, swc, etc.), and
  is a build artifact, not source. A copy committed from one OS is wrong on
  another. The JS ecosystem pins and verifies via the **lockfile** (exact
  versions + integrity hashes), which is the real analog of `go.sum` + `vendor/`.

| Goal | Go | JS |
|---|---|---|
| Pin exact versions | `vendor/` + `go.sum` | commit `pnpm-lock.yaml` |
| Verify integrity | `go.sum` hashes | integrity hashes in lockfile |
| Offline / reproducible | `vendor/` in git | offline mirror (Tier 2+, not adopted) |
| Build output | not committed (`/bin`) | `node_modules/`, `dist/` not committed |

This is implemented in `web/.gitignore`: `node_modules/`, `dist/`, caches ignored;
`pnpm-lock.yaml` deliberately **not** ignored.

### 4.2 Package manager: pnpm

**Decision:** pnpm 10, pinned via corepack (`packageManager: "pnpm@..."`), which
makes every machine and CI use the exact same integrity-verified pnpm binary —
the `GOTOOLCHAIN=local` analog for the package manager itself.

**Why pnpm over npm / Yarn:** pnpm 10 gives, by default, three things that
directly serve the threat model:

- **Install scripts are blocked by default** (threat 1). A package runs lifecycle
  scripts only if explicitly allow-listed (`onlyBuiltDependencies`). npm runs them
  by default.
- **Publish cooldown** (`minimumReleaseAge`) — refuse any version published less
  than N days ago (threat 2). This is the single most effective defense against
  the "compromised version published an hour ago" class.
- **Strict, content-addressable store** and first-class `--frozen-lockfile`.

**Alternatives considered:**

- **npm** — ubiquitous, but runs install scripts by default and has no native
  cooldown. More hardening would have to be bolted on.
- **Yarn Berry (zero-installs)** — genuinely interesting: it commits a *compressed*
  `.yarn/cache/` rather than an exploded `node_modules`, which is the truest npm
  analog of Go vendoring (small, deterministic, diff-trackable, zero-network
  builds). We declined it because (a) we chose Tier 1, so we don't need committed
  artifacts, and (b) PnP/zero-install ergonomics add friction for a solo dev who
  values velocity. It remains the obvious upgrade path if we ever want Tier 2/3.

### 4.3 Vendoring tier: Tier 1

The three tiers we weighed:

- **Tier 1 (chosen):** commit `pnpm-lock.yaml`; CI installs `--frozen-lockfile`.
  Integrity hashes + cooldown + script-blocking.
- **Tier 2:** a registry we control (Verdaccio, or `pnpm fetch` against a seeded
  store) — the `GOPROXY=off`-against-a-local-mirror analog.
- **Tier 3:** vendor the pnpm content-addressable store into the repo. Awkward
  (the symlinked store doesn't commit cleanly); Tier 2 gets the same guarantee
  with less friction.

**Why Tier 1 is enough, stated precisely** (we examined exactly what Tier 2 adds):

Tier 1's lockfile already carries a **per-tarball integrity hash**, so pnpm
verifies the bytes of every pinned package at install time. Therefore Tier 1
*already* defends against content tampering, version drift, and dependency
confusion. The things Tier 1 does **not** fix — a maliciously *pinned* version,
and build-time code execution — **Tier 2 does not fix either** (the mirror serves
whatever is pinned).

So Tier 2's only marginal gains over Tier 1 are:

1. **Availability / durability.** Integrity is not availability: a hash proves a
   tarball is *correct*, not that it still *exists*. Tier 1 builds only if upstream
   still serves those exact bytes. Tier 2 keeps building through an unpublish/yank
   (left-pad), a deleted account, a registry outage, or an air-gapped runner.
2. **Egress control.** Pulling only from a registry we control lets CI run with no
   public npm egress at all — enforcing confusion-resistance at the network
   boundary rather than trusting config.

Both are **business-continuity / reproducibility** properties, not
malicious-code protection. For a solo, early-stage app with no air-gapped-build
requirement, that is not worth the operational cost today. The upgrade path is
clean if it ever is. (Caveat for the future: a plain Verdaccio *caching* proxy
only delivers durability if its storage is persisted/seeded — otherwise it falls
back to Tier 1's failure modes.)

### 4.4 Hardening configuration

Concrete knobs (full values in the scaffold):

- `pnpm-workspace.yaml`: `minimumReleaseAge` (7-day cooldown);
  `onlyBuiltDependencies` (minimal install-script allow-list, e.g. `esbuild`).
- `.npmrc`: `frozen-lockfile`, `prefer-frozen-lockfile`, `auto-install-peers=false`,
  `strict-peer-dependencies=true`, `ignore-scripts=true` (belt-and-suspenders on
  top of pnpm 10's default), `registry=` pinned to npmjs (dependency-confusion
  guard).
- `package.json`: `packageManager` (corepack pin), `engines.node`; `.nvmrc` pins
  the Node major.

**Operational note:** the 7-day cooldown means a fresh `pnpm install` will refuse
versions published within the last week. This is expected, not a failure — we pin
to versions older than the window.

### 4.5 UI components: shadcn/ui

**Decision:** shadcn/ui — the CLI copies component **source** into
`web/src/components/ui/`, which we commit and review like any other code we own.
The shadcn CLI runs transiently via `pnpm dlx`; it is never a project dependency.

**Why:** it converts an opaque UI-library dependency into reviewed, in-repo source
— the spiritual equivalent of vendoring (threats 2, 4). It also matches the solo /
minimal-ecosystem constraint.

**Honest caveat:** shadcn moves *component* code in-repo, but it builds on Radix
primitives, and each primitive is its own package — a real transitive tree
remains. We trade one big opaque UI kit for smaller, individually pinned,
cooldown-gated, script-blocked packages. That is a real improvement, not zero
deps.

**Alternatives considered:** a batteries-included UI kit (MUI, Mantine, Chakra) —
rejected as a large opaque runtime surface, doubly unattractive for a public
bundle and a solo maintainer who values an auditable dep list.

### 4.6 Data grid: hand-rolled

**Decision:** no grid dependency. Build heavy tables on a plain table + shadcn
styling, adding sorting / filtering / virtualization ourselves as screens demand
them.

**Why:** the strongest supply-chain choice — zero added runtime trust surface —
and viable because the app is solo-owned with full control of requirements. The
cost is reimplementing grid mechanics, accepted deliberately.

**Alternatives considered:**

- **TanStack Table (headless)** — logic-only, single MIT vendor, fits the
  "own your component code" philosophy; would have been the pick if hand-rolling
  proved too slow. Strong second choice and the natural fallback.
- **AG Grid Community** — fastest to a feature-rich grid, but a large opaque
  bundle shipped to public browsers and a bigger surface; enterprise features
  paywalled. Rejected on bundle + surface grounds.

### 4.7 Charts: ECharts via `echarts-for-react`

**Decision:** ECharts, wrapped by `echarts-for-react`, **lazy-loaded / code-split**
so it is not in the initial payload.

**Why:** with a public bundle, the metric we optimize is *distinct maintainers to
trust at runtime*. ECharts is one large vendor plus one thin wrapper — the fewest
distinct maintainers of the options — and is highly capable for the heavy,
interactive charts this app needs. The accepted trade-off is a larger single
bundle, mitigated by code-splitting. The wrapper is the only "extra" maintainer
and is replaceable with a ~30-line `useEffect` hook calling `echarts` directly if
we ever want it gone.

**Alternatives considered:**

- **Recharts** — most popular, fastest velocity, but pulls a wide `d3-*`
  transitive tree: many small packages/maintainers in the public bundle. Worst on
  the trust-surface metric. Rejected.
- **visx (Airbnb)** — modular d3 primitives, smallest tailored bundle, good
  supply-chain story, but the most code to write — too much build effort for solo
  given "heavy charts." Rejected on velocity.
- **Chart.js** — moderate size, simple, small tree, but less flexible for bespoke
  heavy/interactive visualization. Rejected on capability.

### 4.8 Deployment

**Decision:** the production bundle is built into `web/dist` and **served by the Go
backend** (not a third-party CDN), behind a **Content-Security-Policy**.

**Why (public-facing, threat 5):** serving our own bundle removes a CDN as an
additional runtime trust party and delivery attack surface; a CSP limits the blast
radius if a bundled dependency is nonetheless compromised. Cooldown remains the
primary upstream defense; CSP is the runtime containment.

---

## 5. Residual risks (what this does NOT solve)

- **Build-time execution.** Script-blocking stops *install*-time code, but the
  build itself runs our tooling (Vite/esbuild execute our config). A compromised
  build-time dep can still act during the build. Mitigation: cooldown + audit on
  build deps too; build in CI, not on dev machines, where feasible.
- **Maliciously pinned version.** If a compromised version makes it into the
  lockfile, every tier installs it faithfully. Mitigation: cooldown buys a
  detection window; run `pnpm audit` in CI.
- **Cooldown is probabilistic.** It only helps if a compromised release is
  reported within the window (usually, not always). Audit is the backstop.
- **Runtime supply chain remains nonzero.** Public bundle = compromised runtime
  dep can reach users. Minimized (small dep set, self-hosted bundle, CSP), not
  eliminated.

---

## 6. Directory layout

```
tadmor/
  cmd/ internal/ db/ vendor/      # Go backend, unchanged
  web/                            # the React app
    src/
      components/ui/              # shadcn components — vendored INTO repo
      lib/
      App.tsx  main.tsx
    public/
    index.html
    package.json                  # packageManager pin, engines
    pnpm-lock.yaml                # committed (Tier 1; the go.sum analog)
    pnpm-workspace.yaml           # cooldown + script allow-list
    .npmrc                        # frozen lockfile, pinned registry, ignore-scripts
    .nvmrc                        # Node major pin
    components.json               # shadcn config
    tsconfig.json
    vite.config.ts
    .gitignore                    # ignores node_modules/, dist/; keeps lockfile
```

## 7. Build / Makefile targets

Mirroring the Go targets' discipline (corepack-pinned pnpm, frozen install):

```make
WEB := web

web-install:  ## Install frontend deps from the frozen lockfile
	cd $(WEB) && corepack pnpm install --frozen-lockfile

web-dev:      ## Run the Vite dev server
	cd $(WEB) && corepack pnpm dev

web-build:    ## Production build into web/dist
	cd $(WEB) && corepack pnpm build

web-check:    ## Type-check + lint + audit
	cd $(WEB) && corepack pnpm tsc --noEmit && corepack pnpm audit --audit-level=high
```

---

## 8. Status and next step

Done: stack ratified; `web/.gitignore` written (Tier 1). Not yet done: scaffold
the rest of `web/` (config files above + Makefile targets) and run the initial
`pnpm install` to generate the committed lockfile.
