# Local Development — Running and Testing the Front End

How to bring up tadmor locally for interactive front-end work: the Postgres
instance, the Go backend (which migrates and seeds itself), and the Vite dev
server with hot reload.

For *why* the front-end stack looks the way it does, see
`docs/frontend-stack.md`. For the schema and its invariants, see `db/README.md`.

---

## 1. The shape of the dev loop

Two long-running processes:

| Process | Command | Port | Role |
|---|---|---|---|
| Go backend | `make run` | `:8080` | Serves `/api/*`; applies migrations + seed data on boot |
| Vite dev server | `make web-dev` | `:5173` | Serves the React app with hot module reload |

The Vite dev server proxies `/api` → `http://localhost:8080`
(`web/vite.config.ts`), so the app calls same-origin `/api/*` in both dev and
production. **Open the Vite URL (`:5173`), not `:8080`** — that's the one with
hot reload.

`:8080` serves the *embedded production* bundle (built into the Go binary via
`//go:embed all:dist`). Use it only to test the production-like build / CSP, via
`make release` then run the binary — but you lose hot reload there.

---

## 2. One-time setup

### 2.1 Create the Postgres role and databases

The Makefile's default connection strings expect role `tadmor` / password
`tadmor`, databases `tadmor` (dev), `tadmor_test` (Go tests), and `tadmor_e2e`
(Playwright UI tests):

```
DATABASE_URL      ?= postgres://tadmor:tadmor@127.0.0.1:5432/tadmor?sslmode=disable
TEST_DATABASE_URL ?= postgres://tadmor:tadmor@127.0.0.1:5432/tadmor_test?sslmode=disable
E2E_DATABASE_URL  ?= postgres://tadmor:tadmor@127.0.0.1:5432/tadmor_e2e?sslmode=disable
```

Run these as the Postgres superuser (`postgres`):

```bash
sudo -u postgres psql <<'SQL'
CREATE ROLE tadmor WITH LOGIN PASSWORD 'tadmor';
CREATE DATABASE tadmor OWNER tadmor;
CREATE DATABASE tadmor_test OWNER tadmor;
CREATE DATABASE tadmor_e2e OWNER tadmor;
SQL
```

Equivalent using the wrapper tools (`createuser` prompts for the password —
there's no inline-password flag; enter `tadmor` twice):

```bash
sudo -u postgres createuser --login --pwprompt tadmor
sudo -u postgres createdb --owner=tadmor tadmor
sudo -u postgres createdb --owner=tadmor tadmor_test
sudo -u postgres createdb --owner=tadmor tadmor_e2e
```

**Why separate databases:** `tadmor` is your dev data; `tadmor_test` is used by
`make test`, whose integration tests **reset the DB** each run, so it must stay
separate from dev data. `tadmor_e2e` is used by `make e2e`, whose teardown
**deletes rows via psql** — keeping it separate means a teardown bug can never
touch dev data. If `tadmor_e2e` is missing, `make e2e` will try to create it
itself (that works only when the `tadmor` role has `CREATEDB`; the setup above
doesn't grant it, hence the explicit `createdb`). Schema needs no hand care in
any of them: the server migrates on startup, and the Go tests reset their DB.

### 2.2 Allow password auth in `pg_hba.conf`

Your `DATABASE_URL` connects over TCP to `127.0.0.1`. A fresh Postgres often has
the IPv4 localhost rule set to `peer`/`ident` (authenticate by OS user), which
rejects password connections even when the role exists. Fix it:

1. Find the active file:

   ```bash
   sudo -u postgres psql -tA -c 'SHOW hba_file;'
   ```

   Debian/Ubuntu (incl. WSL): usually
   `/etc/postgresql/<version>/main/pg_hba.conf`. RHEL/Arch/Homebrew: usually
   under the data directory (`SHOW data_directory;`).

2. Inspect the host rules:

   ```bash
   sudo grep -nE '^[^#]*\b(host|local)\b' "$(sudo -u postgres psql -tA -c 'SHOW hba_file;')"
   ```

3. Set the **last column** (METHOD) of the localhost lines to
   `scram-sha-256`:

   ```
   # TYPE  DATABASE  USER  ADDRESS         METHOD
   host    all       all   127.0.0.1/32    scram-sha-256
   host    all       all   ::1/128         scram-sha-256
   ```

   `scram-sha-256` is the modern default. On older servers initialized with
   `password_encryption = md5`, use `md5` instead — the client password is the
   same either way.

4. Reload (no restart / no data loss):

   ```bash
   sudo -u postgres psql -c 'SELECT pg_reload_conf();'
   # or: sudo systemctl reload postgresql
   ```

5. Verify:

   ```bash
   psql 'postgres://tadmor:tadmor@127.0.0.1:5432/tadmor?sslmode=disable' -c '\conninfo'
   ```

**Gotchas**

- If you changed a line *from* `md5` to `scram-sha-256` on an older server, make
  sure the role's stored password is a SCRAM hash:
  `ALTER ROLE tadmor PASSWORD 'tadmor';` rewrites it under the active method. A
  role created on an already-SCRAM server (the case above) is fine as-is.
- **WSL:** if Postgres isn't running, start it with
  `sudo service postgresql start`. systemd is often not the init system under
  WSL, so `systemctl` may not work — `service` does.

### 2.3 Install front-end dependencies

```bash
make web-install     # corepack pnpm install --frozen-lockfile
```

Requires the pinned Node 22 + corepack pnpm toolchain; fnm auto-switches from
`web/.nvmrc` (see `docs/frontend-stack.md` §4.9).

### 2.4 Create your login user

The whole API (and therefore the app) sits behind session auth; there is no
sign-up screen. Bootstrap (or reset the password of) a user with the server
binary — the password is read from stdin:

```bash
make build
echo 'your-password-here' | ./bin/server -adduser \
    -email you@example.com -name 'Your Name'
```

Re-running with the same email resets that user's password. Users created this
way are administrators (they can manage users and unpost documents); create
regular users from the app's Users screen. (The e2e suite manages its own
throwaway login user; you don't need one for it.)

---

## 3. Migrations and seeding — automatic, no separate step

**There is no separate migrate or seed command.** The server applies pending
migrations itself on every boot: `cmd/server/main.go` calls
`db.Apply(ctx, pool, "db/migrations")` after connecting and before serving. It
logs `applied migrations …` (or `database schema up to date`).

The migration files follow the golang-migrate naming convention
(`db/migrations/NNNNNN_name.{up,down}.sql`), so the same files also work from the
`migrate` CLI if you ever need manual control or rollback (`db/README.md`):

```bash
migrate -path db/migrations -database "$DATABASE_URL" up
migrate -path db/migrations -database "$DATABASE_URL" down 1   # roll back one step
```

**Seed data is baked into the `.up.sql` migrations** (with
`ON CONFLICT DO NOTHING`, so re-running is harmless) — this is why the screens
have data out of the box. There is no standalone seed script:

| Migration | Seeds |
|---|---|
| `000002_reference` | starter `countries`, `currencies` (a common subset, not the full ISO lists) |
| `000004_accounting` | starter chart of `accounts` + `account_types` (the ~11 seeded accounts) |
| `000005_sales` | `payment_terms` (Net 15/30/…), `tax_codes` |
| `000008_grni` | the GRNI clearing account |

The full ISO country/currency lists are deferred to "an ancillary seed script"
(noted in `000002_reference.up.sql`); that script does not exist in the repo
yet — the migrations seed only enough to make local dev and tests work.

---

## 4. Daily startup

```bash
# Terminal 1 — backend: connects, migrates + seeds automatically, serves :8080
export DATABASE_URL='postgres://tadmor:tadmor@127.0.0.1:5432/tadmor?sslmode=disable'
make run

# Terminal 2 — front end with hot reload, proxies /api → :8080
make web-dev          # open the http://localhost:5173 URL it prints
```

Because the Makefile already defaults `DATABASE_URL` to that exact string, once
the role/DB exist you can run `make run` with no env var set at all.

---

## 5. Useful Makefile targets

| Target | What it does |
|---|---|
| `make run` | Build + run the server (migrates/seeds, serves `:8080`) |
| `make web-dev` | Vite dev server with hot reload (`:5173`) |
| `make web-install` | Frozen-lockfile front-end install |
| `make web-build` | Production bundle into `web/dist` |
| `make release` | `web-build` then build the server with the bundle embedded |
| `make web-check` | Front-end typecheck + `pnpm audit` |
| `make test` | Full Go suite against `tadmor_test` (resets the DB) |

---

## 6. Quick troubleshooting

- **`config: DATABASE_URL is required`** — env var unset and you overrode the
  Makefile default; export `DATABASE_URL` or use `make run` (which passes it).
- **Auth failed for user `tadmor`** — `pg_hba.conf` still on `peer`/`ident`, or
  the password hash predates a method change (§2.2).
- **Front end loads but API calls 404 / fail** — the Go backend on `:8080` isn't
  running; the Vite proxy has nothing to forward `/api` to.
- **Stuck on the sign-in screen / everything is 401** — no login user exists
  yet (§2.4), or the session expired: sessions last 30 days from login.
- **`pnpm install` refuses a fresh version** — expected: the 7-day publish
  cooldown (`docs/frontend-stack.md` §4.4). Pin to a version older than the
  window.
</content>
</invoke>
