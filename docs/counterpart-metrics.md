# Counterpart metrics

**Status:** adopted 2026-10-03.

**Scope:** how tadmor and its counterpart implementations (same product,
different stacks; see `spec/README.md`) are measured and compared. This
doc defines each metric precisely enough that two people measuring the
same repository get the same number, and it records tadmor's baseline.

The point of the counterparts is to compare supply-chain attack surface,
so the metrics are ranked:

| Priority | Metric | How it is measured |
| -------- | ------ | ------------------ |
| Primary | 1. Distinct maintainers trusted | `tools/measure.py` |
| Primary | 2. Dependency count | `tools/measure.py` |
| Primary | 3. How hard a hermetic build is | manual procedure, §3 |
| Nice to have | 4. Third-party source size | `tools/measure.py` |
| Nice to have | 5. Own lines of code | `tools/measure.py` |
| Nice to have | 6. Effort | `tools/measure.py` (git proxy) plus a manual note |
| Nice to have | 7. Runtime performance | manual procedure, §7 |

## Ground rules

- **Measure a complete implementation.** A counterpart is measured at a
  commit where `conformance/` passes completely **and** its own UI covers
  every screen and action in `spec/domain.md` §13, checked by walking
  through that list. The report names the spec commit from
  `spec/UPSTREAM`. A partial implementation's numbers are not comparable,
  and a backend-only one leaves out the part that dominates tadmor's
  figures.
- **Same tool, same machine.** `tools/measure.py` lives in tadmor and is
  pointed at each counterpart's checkout:

  ```sh
  tools/measure.py ../tadmor-counterpart [--test-dir e2e] [--copied path]
  ```

  Performance (§7) is measured on one machine, against the same Postgres
  container image.
- **Three categories**, reported separately and then combined:
  - **runtime** is code shipped to production: the server and the browser
    bundle;
  - **build** is code that runs to produce the deployable: compilers,
    bundlers, type checkers, plugins;
  - **test** is code that runs only to test it, such as Playwright.

  Build-time code is not harmless just because it doesn't ship, because
  it runs on developer and CI machines with full access. That is why
  **runtime+build** is the headline figure.
- **Out of scope for every implementation:** the operating system and its
  packages, Postgres itself (shared by all, if the schema is reused), the
  browser, and git. A counterpart that needs a different database engine
  reports it under toolchains.

## 1. Distinct maintainers trusted

**Definition:** the number of distinct **publishing identities** that can
put code into the build. A publishing identity is an account that can
release a new version of a dependency, so each one is something that can
be compromised (the threat model in `docs/frontend-stack.md` §3).

| Ecosystem | Identity | Source |
| --------- | -------- | ------ |
| npm | each npm account in a package version's `maintainers` | registry metadata (`registry.npmjs.org/<name>/<version>`); no package code is fetched |
| Go modules | the repository owner (user or organization); `golang.org/x/*` counts as one identity, the Go project | module path |
| Others | the registry's owner list where one exists (crates.io owners, NuGet owners); otherwise the repository owner | add to `tools/measure.py` when needed |

The counts are not perfectly fair across ecosystems:

- npm lists individual accounts, so an organization that publishes with
  several employees counts several times. ECharts alone accounts for about
  20 Apache committer accounts.
- Go publishes by pushing a tag to a repository and exposes no account
  list, so a Go organization counts once.

Go is therefore *under*-counted relative to npm. Read the gap between, say,
2 and 58 as large but approximate.

Separately, **toolchain publishers** are listed by name and not counted
into the figure. These are the language toolchain and package manager the
build runs: for tadmor, the Go project, the Node.js project, and pnpm
(fetched by corepack), plus, for tests only, Playwright's browser binary
from Microsoft's CDN. They are few, so the comparison of them is
qualitative.

## 2. Dependency count

**Definition:** distinct third-party package versions resolved for each
category, transitively, **as installed on linux/x64**. Optional packages
for other platforms (esbuild and rollup binaries for darwin, windows, and
so on) are never installed, so they are excluded.

- Go: modules in `vendor/modules.txt` that contribute at least one package
  to the build.
- npm (pnpm): the closure of a lockfile importer's `dependencies` (runtime)
  or `devDependencies` (build), following the lockfile's own resolution.
- A lockfile under a test directory (`--test-dir`, default `e2e`) counts
  as test.
- Type-only packages (`@types/*`) are counted where the resolver puts
  them. They hold no executable code, but they are still published by
  someone.

## 3. How hard a hermetic build is

**Definition:** the highest level of this ladder that a **measured run**
reaches, for each deployable part and for the whole. The whole is the
lowest of its parts.

| Level | Meaning |
| ----- | ------- |
| 0 | Build resolves versions at build time (ranges, no lockfile). |
| 1 | Exact versions are pinned with integrity hashes, but the build needs the network. |
| 2 | Offline after a one-time fetch into a cache or mirror outside the repo. |
| 3 | Offline from a **clean clone** with an empty cache: all third-party source is in the repo, so only the pinned toolchain is needed. |
| 4 | Level 3, and two builds produce byte-identical artifacts. |

Each part's report also notes:

- **Install-time code execution**: none, blocked by policy, or required.
- **Manual steps** outside the repo, such as installing the toolchain,
  running `install-deps`, or downloading a browser.

**Procedure** (levels 3 and 4): clone the repo into a fresh directory,
then build inside a container with `--network=none`. Mount the pinned
toolchain read-only and point every cache at an empty directory. Build
twice and compare `sha256sum`. tadmor's run was:

```sh
git clone -q /path/to/tadmor clean
podman run --rm --network=none \
  -v /usr/lib/go-1.26:/usr/lib/go-1.26:ro -v /usr/share/go-1.26:/usr/share/go-1.26:ro \
  -v "$PWD/clean:/src" -w /src -e HOME=/tmp -e GOFLAGS=-mod=vendor -e GOTOOLCHAIN=local \
  -e GOPROXY=off -e GOMODCACHE=/tmp/empty -e CGO_ENABLED=0 debian-image \
  sh -c 'go build -trimpath -o /tmp/a ./cmd/server && go build -trimpath -o /tmp/b ./cmd/server && sha256sum /tmp/a /tmp/b'
```

Building from a clean clone is the step that matters: it found the
`.gitignore` rule that had kept two vendored files out of git, which no
amount of building in the working tree would have shown.

## 4. Third-party source size

**Definition:** the size of third-party source the build consumes, per
category.

- **Bytes** as published:
  - Go: the files in `vendor/`;
  - npm: each package version's `dist.unpackedSize` from registry metadata;
  - source copied into the repo (e.g. shadcn components, `--copied`):
    its files.
- **Lines** (non-blank) wherever the source is on disk: Go `vendor/`,
  copied source, and npm packages only when `node_modules` is installed.

npm sizes are as published, so they include multiple builds, source maps,
and typings. They measure what you must trust, not what ships to the
browser.

## 5. Own lines of code

**Definition:** non-blank lines in tracked source files, by language.

**Excludes:**

- the shared `spec/` and `conformance/`;
- `vendor/` and `node_modules/`;
- generated files, meaning any with a "Code generated … DO NOT EDIT",
  "@generated", or "generated by" marker in its first five lines;
- copied third-party source (`--copied`);
- docs and config (Markdown, JSON, YAML).

Tests count: they are code the project wrote and maintains.

## 6. Effort

This is the hardest metric to measure honestly, so it gets two numbers:

- **Git proxy** (`tools/measure.py`): commits and distinct commit days up
  to the measured commit. For a counterpart, take these at the **first
  complete commit** (suite passes and UI covers §13, as in the ground
  rules).
- **Manual note**: rough working sessions or hours, and anything unusual
  (a stack learned from scratch, a port done mostly by an agent).

tadmor's own figures are not directly comparable. It was designed from
nothing, its spec came afterwards, and its history includes deployment and
operations work. A counterpart starts from a finished spec and suite.

## 7. Runtime performance

These are cheap, repeatable measures on the same machine and Postgres
image, with migrations already applied:

- **time to ready**: process start until `GET /readyz` returns 200;
- **idle memory**: RSS two seconds after ready;
- **memory after load**: RSS and peak RSS (`VmHWM`) after one full
  conformance run;
- **suite time**: the conformance run's wall time, as the suite reports
  it. This is dominated by deliberate password-hashing cost at login, so
  it is a sanity check, not a throughput benchmark;
- **deployable size**: the artifact you would ship, such as a binary or
  image.

If these ever need to say more than "same order of magnitude", add a load
test then (a fixed request mix at fixed concurrency) and define it here
first.

## tadmor baseline

Measured 2026-10-03 at commit `73e4a16` (spec at the same commit), on
linux/x64 with Postgres 17.

**Primary**

| | runtime | build | test | runtime+build | everything |
| --- | ---: | ---: | ---: | ---: | ---: |
| Maintainers trusted | 58 | 130 | 4 | **179** | 183 |
| Dependencies | 99 | 80 | 3 | **179** | 182 |

- Runtime splits into Go (6 modules, 2 identities: `github.com/jackc` and
  the Go project) and the browser bundle's npm tree (93 packages, 56
  accounts).
- The Go backend alone is therefore **6 dependencies and 2 maintainers**.
  Nearly all of tadmor's supply-chain surface is the front end and its
  build toolchain.
- Toolchain publishers: the Go project, the Node.js project, pnpm, and
  Microsoft's Playwright browser (tests only).

| Part | Hermetic level | Install-time code | Manual steps |
| ---- | -------------- | ----------------- | ------------ |
| Go server (`cmd/server`) | **4** (measured: clean clone, empty module cache, no network, two builds identical) | none | install the pinned Go toolchain |
| Front end (`web/`) | **1** (pinned, integrity-hashed lockfile, 7-day cooldown, but `pnpm install` needs the registry; 2 is possible with `pnpm fetch` and `--offline` but not adopted) | blocked by policy (`ignore-scripts`) | Node via fnm; pnpm via corepack (downloaded) |
| Release (server with embedded SPA) | **1** (the lower of its parts) | | |
| UI tests (`e2e/`) | **1**, plus a browser binary fetched outside the lockfile | blocked by policy | `install-browser`, plus `install-deps` as root |

**Nice to have**

| Metric | Value |
| ------ | ----- |
| Third-party source | Go `vendor/` 7.7 MB / 151,846 lines; npm runtime 106.2 MB, build 89.7 MB, test 17.6 MB (as published); copied shadcn components 16.6 KB / 464 lines |
| Own lines | **36,595**: TypeScript 18,276, Go 15,071, SQL 2,773, Python 240, CSS 118, Shell 105, HTML 12 |
| Effort (git proxy) | 123 commits on 18 days, 2026-06-16 to 2026-10-03 (not comparable; see §6) |
| Time to ready | 31 ms |
| Idle memory | 13.9 MB RSS |
| After a conformance run | 20.1 MB RSS (peak 20.1 MB) |
| Conformance suite time | 6.5 s (32 cases) |
| Deployable size | 17.0 MB server binary, measured **without** the SPA embedded (`web/dist` was not built on the measuring machine); re-measure after `make release` |
