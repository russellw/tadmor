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
| Primary | 1. Distinct maintainers trusted | `tools/measure.py`, from the dependency manifest |
| Primary | 2. Dependency count | `tools/measure.py`, from the dependency manifest |
| Primary | 3. How hard a hermetic build is | manual procedure, §3 |
| Nice to have | 4. Third-party source size | `tools/measure.py`, from the dependency manifest |
| Nice to have | 5. Own lines of code | `tools/measure.py` |
| Nice to have | 6. Effort | `tools/measure.py` (git proxy) plus a manual note |
| Nice to have | 7. Runtime performance | manual procedure, §7 |

## Ground rules

- **Measure a complete implementation.** A counterpart is measured at a
  commit where `conformance/` passes completely **and** its own UI covers
  every item of the checklist in `spec/domain.md` §13 (G1 to A5), checked
  by walking through it and recorded item by item in the report. The report names the spec commit from
  `spec/UPSTREAM`. A partial implementation's numbers are not comparable,
  and a backend-only one leaves out the part that dominates tadmor's
  figures.
- **Same definitions, each project's own tooling.** Every implementation
  commits a **dependency manifest**, `dependencies.json` (next section),
  written by its own tooling, which is the only code that knows its
  package manager. `tools/measure.py` lives in tadmor, knows no package
  manager, and applies the definitions below to any checkout's manifest.
  It also counts own lines and effort from git, which need no knowledge
  of the stack:

  ```sh
  tools/measure.py ../tadmor-dotnet [--copied path]
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
  packages, Postgres itself and the shared schema (every implementation
  runs on both; `spec/README.md`), the browser, and git.

## The dependency manifest

Each implementation commits `dependencies.json` at its repository root.
It lists every third-party package the build resolves on linux/x64, with
the publishing identities behind each. It is written by the project's own
tooling (tadmor's is `tools/dependencies.py`; each counterpart's is its
vendoring script) and kept current with the lockfile: regenerate it whenever dependencies change, and commit both
together. Because it is committed, measuring needs no network, a change is
reviewable in a diff, and the figures can be checked against the evidence
the manifest cites.

```json
{
  "format": "tadmor-dependencies/1",
  "generator": "tools/vendor.py (tadmor-dotnet)",
  "platform": "linux/x64",
  "toolchains": ["Microsoft .NET SDK 10.0 (Ubuntu's build)"],
  "packages": [
    {"ecosystem": "nuget", "name": "Npgsql", "version": "10.0.3", "category": "runtime",
     "identities": ["nuget:Npgsql", "nuget:roji", "nuget:brar", "nuget:ninofloris"],
     "evidence": "https://azuresearch-usnc.nuget.org/query?q=packageid:Npgsql"}
  ],
  "sources": [{"label": "NuGet packages (runtime, unpacked)", "bytes": 27280035, "lines": null}]
}
```

- **`packages`**: one entry per package version and category, using the
  definitions of §1 and §2. `identities` is the list of publishing
  identities (§1), or `null` where they could not be determined, which
  the report flags. Each identity string names its namespace, so that
  accounts on different registries never merge: the registry
  (`npm:alice`, `pypi:bob`, `nuget:Npgsql`), or, where publishing is by
  repository, the repository host and owner (`github.com/jackc`).
  `evidence` says where the identities were read: a registry URL, an API
  call, or the rule applied.
- **`toolchains`**: the toolchain publishers (§1), by name.
- **`sources`**: third-party source size (§4), as labeled rows of bytes
  and lines, `null` where unknown.

`tools/measure.py` checks the format and the fields, then counts. It does
not repeat the lookups. A reviewer who doubts a figure follows the
evidence.

## 1. Distinct maintainers trusted

**Definition:** the number of distinct **publishing identities** that can
put code into the build. A publishing identity is an account that can
release a new version of a dependency, so each one is something that can
be compromised (the threat model in `docs/frontend-stack.md` §3).

| Ecosystem | Identity | Source |
| --------- | -------- | ------ |
| npm | each npm account in a package version's `maintainers` | registry metadata (`registry.npmjs.org/<name>/<version>`); no package code is fetched |
| Go modules | the repository owner (user or organization); `golang.org/x/*` counts as one identity, the Go project | module path |
| PyPI | each account with a role (Owner or Maintainer) on the project; a project published through a PyPI organization lists none, and counts as one identity, the organization | PyPI's XML-RPC `package_roles` (the web pages that show roles refuse scripted clients) |
| NuGet | each account in the package's owner list (owners are per package, not per version) | the NuGet search API's `owners` field |
| Packagist | each account in the package's maintainers list (per package) | `packagist.org/packages/<name>.json` |
| Maven Central | the verified Central namespace that can publish the groupId: a reversed domain (`org.apache`), a code host's user namespace (`io.github.user`), or, for a groupId without a domain, its first part (`jakarta`); old domainless groupIds count under their owner's namespace (`commons-io` as `org.apache`). Central publishes no account list | the groupId |
| Others | the registry's owner list where one exists (crates.io owners); otherwise the repository owner | |

The counts are not perfectly fair across ecosystems:

- npm lists individual accounts, so an organization that publishes with
  several employees counts several times. ECharts alone accounts for about
  20 Apache committer accounts.
- Go publishes by pushing a tag to a repository and exposes no account
  list, so a Go organization counts once.
- PyPI hides the members of an organization's teams, so a project such as
  Django, published through the `django` organization, counts once, as a Go
  organization does. Roles are per project, not per version.
- Maven Central, like Go, exposes no accounts, so a namespace counts once,
  but an organization holding several namespaces (Eclipse's `jakarta` and
  `org.eclipse`) counts once for each.
- NuGet lists organizations as owner accounts, and one company may own a
  package through several (Microsoft publishes as `Microsoft`, `aspnet`,
  `dotnetframework`, and others), so a large publisher counts several
  times, as on npm.

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
category, transitively, **as installed on linux/x64**. The manifest lists
them; how each ecosystem's are found is the manifest tooling's business,
and tadmor's own are found as follows. Optional packages
for other platforms (esbuild and rollup binaries for darwin, windows, and
so on) are never installed, so they are excluded.

- Go: modules in `vendor/modules.txt` that contribute at least one package
  to the build.
- PyPI: the wheels listed in `vendor/lock.txt` (tadmor-python's format:
  name, version, wheel filename, sha256). They are unpacked into
  `vendor/site/` and imported, so all count as runtime. The lock lists every
  wheel imported, transitive ones included, because `tools/vendor.py`
  resolves nothing by itself.
- npm (pnpm): the closure of a lockfile importer's `dependencies` (runtime)
  or `devDependencies` (build), following the lockfile's own resolution.
- A lockfile under a test directory (`tools/dependencies.py --test-dir`,
  default `e2e`) counts
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

Each manifest reports these as its `sources` rows. tadmor's are:

- **Bytes** as published:
  - Go: the files in `vendor/`;
  - npm: each package version's `dist.unpackedSize` from registry metadata;
  - source copied into the repo (e.g. shadcn components, `--copied`):
    its files.
- **Lines** (non-blank) wherever the source is on disk: Go `vendor/`,
  copied source, and npm packages only when `node_modules` is installed.

npm sizes are as published, so they include multiple builds, source maps,
and typings. They measure what you must trust, not what ships to the
browser. Where an ecosystem ships compiled packages, as NuGet does, bytes
are the unpacked package contents and lines are `null`.

## 5. Own lines of code

**Definition:** non-blank lines in tracked source files, by language.

**Excludes:**

- the shared `spec/`, `conformance/`, and `db/migrations/` (the schema
  every implementation runs on; a counterpart's own added migrations,
  kept elsewhere, do count);
- `vendor/` and `node_modules/`;
- generated files, meaning any with a "Code generated … DO NOT EDIT",
  "@generated", or "generated by" marker in its first five lines;
- copied third-party source (`--copied`);
- `tools/measure.py`, the shared measuring tool. A project's own manifest
  tooling (tadmor's `tools/dependencies.py`, a counterpart's vendoring
  script) counts, like any code it maintains;
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
linux/x64 with Postgres 17. Re-measured on 2026-10-05 through the
dependency manifest, once `tools/measure.py` stopped parsing lockfiles
itself: the dependency, maintainer, and source-size figures were
identical. The manifest tooling, 243 non-blank lines of Python in
`tools/dependencies.py`, now counts as tadmor's own code.

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
| Own lines | **33,815**: TypeScript 18,276, Go 15,064, Python 240, CSS 118, Shell 105, HTML 12. Measured as 36,595 before the shared schema was excluded (§5): `db/migrations/` held SQL 2,773 and Go 7 |
| Effort (git proxy) | 123 commits on 18 days, 2026-06-16 to 2026-10-03 (not comparable; see §6) |
| Time to ready | 31 ms |
| Idle memory | 13.9 MB RSS |
| After a conformance run | 20.1 MB RSS (peak 20.1 MB) |
| Conformance suite time | 6.5 s (32 cases) |
| Deployable size | 17.0 MB server binary, measured **without** the SPA embedded (`web/dist` was not built on the measuring machine); re-measure after `make release` |
