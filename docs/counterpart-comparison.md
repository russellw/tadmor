# Counterpart comparison

**Status:** measured 2026-10-06.

**Scope:** the figures of `docs/counterpart-metrics.md` for tadmor and its
six counterparts, measured the same way on the same machine, and what they
say. The metric definitions and ground rules are in that doc; this one is
the report it asks for.

## What was measured

| Implementation | Stack | Measured at | Spec |
| -------------- | ----- | ----------- | ---- |
| tadmor | Go `net/http` + pgx; React SPA (Vite, shadcn) | `261de42` | own |
| tadmor-dotnet | ASP.NET Core 10, EF Core + Npgsql, Razor Pages | `1c93a6e` | `8c3938a`¹ |
| tadmor-java | Spring Boot 4.1, JDBC, Thymeleaf | `76eddba` | `ec905fa` |
| tadmor-php | Laravel 13 (query builder), Blade | `8272f71` | `ec905fa` |
| tadmor-python | Django 6.1, psycopg 3, Django templates, gunicorn | `5aef699` | `ec905fa` |
| tadmor-ruby | Rails 8.1, `pg`, ERB, Puma | `0168371` | `ec905fa` |
| tadmor-rust | Axum, SQLx, Askama, lettre | `c21f06c` | `261de42` |

¹ The spec, suite, and schema are byte-identical across `8c3938a`,
`ec905fa`, and `261de42`; only `spec/UPSTREAM` differs.

**Every implementation is complete** under the ground rules:

- **Conformance:** all seven pass all 35 cases, run fresh on 2026-10-06.
- **UI checklist:** each counterpart's `docs/ui-coverage.md` records, item
  by item, how every item of `spec/domain.md` §13 (G1 to A5) is checked,
  through its own UI tests and a browser walk-through in headless Chromium.
- **Own tests:** each counterpart's `make test` passes.

Checking for loose ends before measuring turned up these, now done:

- **tadmor-java** still owed a real-browser check of its page script: the
  line editor's fills, its lines, its preview, and the delete
  confirmations. All of that passed in Chromium, with every screen reachable
  by links loading cleanly. The README still said "Status: early".
- **tadmor-dotnet** had three flows never clicked through: a credit note's
  Apply, shipping a sales order, and a statement's manual match, unmatch,
  and line deletion. All three now pass in the browser.
- **tadmor-php** and **tadmor-python** had no item-by-item UI record, and
  PHP's page script had never run in a browser. Both now have
  `docs/ui-coverage.md`, and both pass the same walk-through as Java,
  including no Content Security Policy violations.
- **Hermetic builds:** the level-4 results for dotnet, Java, and Rust had
  been measured at their scaffold commits, before the application existed.
  Re-measured at the finished commits, Java and Rust were still
  byte-identical. **dotnet was not**: the SDK's static web assets manifest
  recorded each `wwwroot` file's Last-Modified time, which a fresh clone
  sets to the checkout time. The server never reads that manifest, so
  tadmor-dotnet turned it off, and its 340-file release is byte-identical
  again.
- **`tools/measure.py`** skipped `.erb`, `.rake`, and `.ru` files, so 725
  lines of tadmor-ruby's templates went uncounted. It now counts them.

## Primary metrics

Supply-chain surface, from each repository's `dependencies.json`.
**runtime+build** is the headline figure (`docs/counterpart-metrics.md`,
ground rules).

| | Maintainers, runtime+build | Dependencies, runtime+build | Maintainers, all | Dependencies, all | Hermetic level (whole) |
| --- | ---: | ---: | ---: | ---: | :---: |
| tadmor | **179** | **179** | 183 | 182 | 1 |
| tadmor-dotnet | **8** | **6** | 11 | 15 | 4 |
| tadmor-java | **31** | **144** | 44 | 172 | 4 |
| tadmor-php | **37** | **73** | 42 | 98 | 3 |
| tadmor-python | **7** | **5** | 7 | 5 | 3 |
| tadmor-ruby | **72** | **52** | 72 | 52 | 4 |
| tadmor-rust | **126** | **183** | 126 | 183 | 4 |

By category:

| | Runtime deps / maintainers | Build deps / maintainers | Test deps / maintainers |
| --- | ---: | ---: | ---: |
| tadmor | 99 / 58 | 80 / 130 | 3 / 4 |
| tadmor-dotnet | 6 / 8 | 0 / 0 | 9 / 4 |
| tadmor-java | 68 / 17 | 76 / 20 | 28 / 17 |
| tadmor-php | 73 / 37 | 0 / 0 | 25 / 6 |
| tadmor-python | 5 / 7 | 0 / 0 | 0 / 0 |
| tadmor-ruby | 52 / 72 | 0 / 0 | 0 / 0 |
| tadmor-rust | 149 / 113 | 34 / 33 | 0 / 0 |

**Toolchains** (named, not counted):

| | Toolchain publishers |
| --- | --- |
| tadmor | the Go project; the Node.js project; pnpm (fetched by corepack); Microsoft's Playwright browser (tests only) |
| tadmor-dotnet | Microsoft's .NET SDK 10 (Ubuntu's build); .NET runtime packs from NuGet, bundled into the self-contained release |
| tadmor-java | OpenJDK 25 (the OS's build); Apache Maven via the Maven Wrapper (downloaded once, sha256-checked) |
| tadmor-php | PHP 8.4+ and Composer, from the OS |
| tadmor-python | CPython 3.13+ from the OS; the OS's libpq |
| tadmor-ruby | Ruby 3.3 with RubyGems and Bundler, a C compiler, libpq and libyaml headers, all from the OS |
| tadmor-rust | Rust 1.93 and Cargo (Ubuntu's packages); pkg-config and OpenSSL headers |

**Hermetic builds**, in detail:

| | Level | Install-time code | Manual steps |
| --- | --- | --- | --- |
| tadmor | Go server **4**; front end **1** (the registry is needed at install), so the release is **1** | blocked by policy (`ignore-scripts`) | Node via fnm; pnpm via corepack |
| tadmor-dotnet | **4**: two clean clones built offline give a byte-identical self-contained release | MSBuild `.props`/`.targets` and Roslyn analyzers from Microsoft packages; cannot be blocked | install the SDK |
| tadmor-java | **4**: two clean clones give byte-identical jars, offline from the committed Maven repository | Maven plugins run at build (counted as build dependencies) | install the JDK |
| tadmor-php | **3**: nothing is built; a clean clone runs offline (checked by booting it with the network off). Two checkouts are trivially identical | none: Composer runs with no scripts or plugins | install PHP and its extensions |
| tadmor-python | **3**: as PHP; a clean clone imports and resolves every route with the network off | none: wheels are unpacked, never installed | install Python and libpq |
| tadmor-ruby | **4** for the installed tree: `bundle install --local` offline, with byte-identical native extensions | **required**: 11 gems compile C extensions at install, nokogiri through `mini_portile2`; RubyGems cannot turn it off | install Ruby, a compiler, and headers; `make install` (about 90 s) |
| tadmor-rust | **4**: two clean clones built offline give byte-identical binaries | **required**: 18 crates with build scripts and 13 proc-macro crates run at every build; cannot be turned off | install rustc, Cargo, pkg-config, and OpenSSL headers |

## Nice-to-have metrics

**Size.** Third-party sizes are the manifests' `sources` rows. Own lines
are non-blank lines from `tools/measure.py` (`docs/counterpart-metrics.md`
§5).

| | Third-party, runtime+build | Third-party, test | Own lines | Own lines by language |
| --- | ---: | ---: | ---: | --- |
| tadmor | 203.6 MB / 1,190,321 lines | 17.6 MB | **34,346** | TypeScript 18,438, Go 15,190, Python 483, CSS 118, Shell 105, HTML 12 |
| tadmor-dotnet | 27.3 MB (compiled) | 47.5 MB | **10,133** | C# 8,140, Razor 1,478, Python 248, JavaScript 146, CSS 80, Shell 41 |
| tadmor-java | 58.5 MB (compiled) | 11.0 MB | **12,125** | Java 10,414, HTML 1,205, Python 219, JavaScript 130, CSS 115, Shell 42 |
| tadmor-php | 23.2 MB / 563,980 lines | 6.2 MB | **8,264** | PHP 7,972, JavaScript 122, CSS 115, Shell 55 |
| tadmor-python | 25.6 MB / 185,727 lines | none | **6,973** | Python 5,870, HTML 822, JavaScript 122, CSS 115, Shell 44 |
| tadmor-ruby | 35.7 MB / 821,501 lines | none | **5,915** | Ruby 4,910, ERB 725, JavaScript 122, CSS 115, Shell 43 |
| tadmor-rust | 74.1 MB / 1,665,782 lines | none | **12,816** | Rust 12,109, Python 316, HTML 144, JavaScript 123, Shell 73, CSS 51 |

tadmor's figure includes 464 lines of copied shadcn source as third-party,
not own. The counterparts' Python is their vendoring script, which counts
as own code (§5). PHP's Blade templates count as PHP.

**Effort.** Every counterpart was built from the finished spec and suite
by Claude Code sessions (Opus 5.5): every commit but each repository's
initial one is co-authored. Their commit counts reflect each session's
commit style more than the work: tadmor-ruby landed in one large commit.
Counted up to each project's last commit before this review:

| | Commits | Days with commits | Note |
| --- | ---: | ---: | --- |
| tadmor | 138 | 19 | designed from nothing, 2026-06-16 on; includes deployment and operations; not comparable (§6) |
| tadmor-dotnet | 12 | 1 | |
| tadmor-java | 19 | 2 | |
| tadmor-php | 8 | 2 | business logic ported from tadmor-python |
| tadmor-python | 10 | 2 | |
| tadmor-ruby | 4 | 1 | |
| tadmor-rust | 22 | 2 | |

**Runtime performance.** Measured on one machine against the same
Postgres 17 container, with migrations already applied, each server
started as its conformance script starts it. Time to ready and idle memory
are the median of three starts. Memory is the sum over the server's
process tree; for the multi-process servers PSS, which counts shared pages
once, is given too.

| | Server shape | Time to ready | Idle RSS | RSS after a suite run (peak) | Suite time (35 cases) | Deployable |
| --- | --- | ---: | ---: | ---: | ---: | --- |
| tadmor | one process | 17 ms | 13.4 MB | 19.9 MB (19.9) | 7.1 s | 17.7 MB static binary, SPA embedded |
| tadmor-dotnet | one process | 1.57 s | 129.7 MB | 242.7 MB (242.7) | 10.6 s | 118.6 MB self-contained directory, .NET runtime included |
| tadmor-java | one JVM, default heap | 5.73 s | 259.3 MB | 277.7 MB (319.2) | 7.0 s | 29.7 MB jar, plus a JRE |
| tadmor-php | PHP's built-in server, 4 workers | 0.25 s | 91.8 MB (PSS 44.9) | 225.2 MB (PSS 62.6) | 32.0 s | 0.4 MB source + 23.2 MB `vendor/`, plus PHP |
| tadmor-python | gunicorn, 4 workers | 0.46 s | 240.4 MB (PSS 171.5) | 252.9 MB (PSS 182.9) | 17.4 s | 0.8 MB source + 25.6 MB `vendor/site`, plus Python |
| tadmor-ruby | Puma, one process, 5 threads | 1.81 s | 95.8 MB | 108.1 MB (108.1) | 10.9 s | 0.8 MB app + 78.3 MB installed bundle, plus Ruby |
| tadmor-rust | one process | 18 ms | 11.9 MB | 14.1 MB (14.1) | 9.2 s | 13.1 MB binary |

Suite time is dominated by the deliberate password-hashing cost at each
login (§7), so it says little about request throughput. The servers are
not tuned alike: PHP's built-in server and gunicorn are run with four
workers each, and the JVM with its default heap. Differences inside one
order of magnitude should not be read as rankings.

## What it says

- **tadmor's back end is as lean as any counterpart; its front end is
  not.** The Go server alone is 6 dependencies from 2 maintainers, close to
  dotnet (6 from 8) and Python (5 from 7), and it is the fastest and
  smallest at runtime with Rust. Its headline 179 maintainers come almost
  entirely from the React SPA and its Vite build, and that is also what
  holds its hermetic level at 1. Every counterpart that rendered its UI on
  the server needed no npm at all, and **none** of them comes near tadmor's
  surface except Rust.
- **The leanest whole products are Python and dotnet**, at 7 and 8
  maintainers. Python is a batteries-included framework on a large
  standard library: Django (one organization) plus four packages from six
  accounts. dotnet is one vendor's platform plus a database driver: four
  Microsoft owner accounts and Npgsql's four, for EF Core and its provider,
  with ASP.NET Core itself coming with the SDK. The ecosystems count
  differently (next point), but the gap to the rest is real.
- **Counting is not equally fair across ecosystems.** Maven Central and Go
  expose no account lists, so Java's 31 counts namespaces, each standing
  for an organization of many people, where RubyGems' 72 and npm's figures
  count individual accounts: two teams (Rails core, 12 accounts; Ruby
  core) are 32 of Ruby's 72. NuGet lists organizations as owners, so
  Microsoft appears several times in dotnet's 8. Read the figures as
  orders of magnitude: about 10 (dotnet, Python), 30 to 70 (Java, PHP,
  Ruby), more than 100 (Rust, tadmor).
- **Rust is the surprise.** Its runtime has the most packages (149
  crates from 113 owners), the largest third-party source (1.7 million
  lines), and build scripts and proc macros that run unsandboxed on every
  build, with no switch to stop them. It matches Go at runtime, but it is
  the only counterpart whose supply-chain surface rivals the React front
  end's.
- **Install-time code is a property of the ecosystem.** It can be avoided
  completely in PHP and Python, blocked by policy on npm, accepted from a
  single vendor in .NET and Maven, and is unavoidable in Ruby (C
  extensions) and Rust (build scripts and proc macros).
- **Hermeticity is easy everywhere except npm.** Four counterparts reach
  level 4, and the two with nothing to build sit at 3 only because there is
  nothing to compare. The one regression found (dotnet) came from a
  generated file that embedded a timestamp, which is why the level has to
  be re-measured at the finished commit rather than at the scaffold.
- **Code size is similar.** Without a separate front end, every
  counterpart is 6,000 to 13,000 own lines against tadmor's 34,000, about
  half of which is the React SPA. The dynamic languages are smallest
  (Ruby 5,900, Python 7,000, PHP 8,300), and the statically typed ones
  are larger (dotnet 10,100, Java 12,100, Rust 12,800).
- **At runtime, Go and Rust stand apart**: each starts in under 20 ms
  and idles in under 15 MB. That is a tenth to a twentieth of the memory
  of .NET, the JVM, and multi-worker Python, and 15 to 340 times faster to
  start than any of the others.

## Reproducing

- Dependencies, sizes, own lines, and effort: `tools/measure.py ../<repo>`
  (tadmor: `--copied web/src/components/ui`).
- Hermetic levels: the procedure in `docs/counterpart-metrics.md` §3, run
  in `--network=none` containers from fresh clones. Each counterpart's
  `docs/stack.md` records its run.
- Runtime performance and the browser walk-throughs used scripts kept
  outside the repositories, as the counterparts' earlier walk-throughs
  did, driving tadmor's Playwright install and the conformance suite's
  binary. They are described above and in each `docs/ui-coverage.md`, not
  committed.
