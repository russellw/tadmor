# Deployment — VPS (tadmor.belunaro.com)

How tadmor is deployed: a single static Go binary (embedding the SPA **and** the
schema migrations) running as a hardened systemd service on a fixed-price VPS,
behind Caddy for TLS. No hyperscaler, no per-request billing — the monthly cost
is bounded by construction.

For the local dev loop see `docs/local-development.md`.

---

## 1. The shape of the deployment

One box (OVH VPS, Debian 13, `belunaro.com`) shared by several small apps:

| Piece | What runs it |
|---|---|
| TLS + routing | Caddy (Debian main repo), auto-HTTPS via Let's Encrypt, one vhost per app |
| tadmor server | systemd service `tadmor`, listening on `127.0.0.1:8081` |
| Postgres | Postgres 17 (Debian main repo) on the same box, localhost-only |

Per-app isolation conventions on the box: each app gets its own localhost port
behind a Caddy vhost, its own Postgres role + database, and its own systemd
service running as an unprivileged dynamic user. Wildcard DNS
(`*.belunaro.com` → the VPS) means a new app needs no DNS work.

The server binary is fully self-contained: `web/dist` and `db/migrations` are
both embedded (`//go:embed`), and pending migrations are applied on startup.
Deploying is copying one file and restarting one service.

Single instance by design — `db.Apply` takes no cross-instance lock, so the
one-service setup also sidesteps any migration race. If this ever scales to
multiple instances, give `db.Apply` a Postgres advisory lock first.

## 2. Routine deploy

```bash
make deploy
```

which is:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make release   # SPA build + static Go binary
scp bin/server vps:/tmp/tadmor-server
ssh vps 'sudo install -m 755 -o root -g root /tmp/tadmor-server /opt/tadmor/server \
         && rm /tmp/tadmor-server && sudo systemctl restart tadmor'
curl -fsS https://tadmor.belunaro.com/readyz
```

`vps` is an SSH alias for the box (key-only auth). The binary must be built
`CGO_ENABLED=0` so it is static, and the migration runner treats an embedded FS
with zero migration files as a startup error, so a broken build fails loudly
instead of serving against an empty schema.

Logs and status:

```bash
ssh vps 'systemctl status tadmor'
ssh vps 'sudo journalctl -u tadmor -f'
```

### 2.1 Creating login users

The API sits behind session auth and there is no sign-up screen; bootstrap the
first user on the box with the deployed binary and the same `DATABASE_URL` the
service uses (users created this way are administrators — day-to-day accounts
are better created from the app's Users screen):

```bash
read -rsp 'Password: ' PW && echo && printf '%s\n' "$PW" | \
  ssh vps 'sudo sh -c '\''export $(cat /etc/tadmor/env) && \
    /opt/tadmor/server -adduser -email you@example.com -name "Your Name"'\'''
```

(The password is read locally without echo and piped over stdin, so it never
lands in shell history or the process list. The `$(cat /etc/tadmor/env)` must
sit inside the *single*-quoted `sh -c` string: root has to expand it — the
env file is mode 600 root:root, so a double-quoted version fails with
"Permission denied" because the unprivileged login shell expands it first.)

Add `-admin=false` to provision an ordinary (non-admin) login instead — this is
how the demo's public `guest@demo` account is created:

```bash
printf '%s\n' guest123 | \
  ssh vps 'sudo sh -c '\''export $(cat /etc/tadmor/env) && \
    /opt/tadmor/server -adduser -admin=false -email guest@demo -name "Guest"'\'''
```

### 2.2 Nightly demo reseed

The public guest account (`guest@demo`, see the README) lets anyone edit the
demo data, so every night at 04:17 UTC `tadmor-reseed.timer` rebuilds the
database from a snapshot: stop `tadmor`, `DROP DATABASE` + `CREATE DATABASE`,
restore `/var/lib/tadmor-demo/seed.sql`, ensure a fiscal year and an open
accounting period cover the current date (so posting keeps working as the
snapshot ages), start `tadmor`. The script and units live in `deploy/`
(`reseed.sh`, `tadmor-reseed.service`, `tadmor-reseed.timer`).

Consequences worth remembering:

- **Everything reverts nightly** — including users, password changes, and any
  data curated through the app. To make a change permanent, make it in the
  app and then refresh the snapshot:

  ```bash
  make demo-snapshot
  ```

  (a `pg_dump` of the live database, minus `sessions` rows, written atomically
  over the seed file — no downtime).
- The snapshot may predate the running binary's newest migrations; that is
  fine, because restarting `tadmor` re-applies pending migrations to the
  restored database. Refreshing the snapshot after schema-changing deploys is
  tidy but not required.
- If a restore fails, the script still restarts `tadmor` (which then
  crash-loops on the missing database) and the failure is visible in
  `systemctl status tadmor-reseed.service`.

Install/update on the box (also the rebuild procedure):

```bash
ssh vps 'sudo install -d -m 700 -o postgres -g postgres /var/lib/tadmor-demo'
make demo-snapshot
scp deploy/reseed.sh deploy/tadmor-reseed.service deploy/tadmor-reseed.timer vps:/tmp/
ssh vps 'sudo install -m 755 -o root -g root /tmp/reseed.sh /opt/tadmor/reseed.sh \
         && sudo install -m 644 -o root -g root /tmp/tadmor-reseed.service /tmp/tadmor-reseed.timer /etc/systemd/system/ \
         && rm /tmp/reseed.sh /tmp/tadmor-reseed.service /tmp/tadmor-reseed.timer \
         && sudo systemctl daemon-reload && sudo systemctl enable --now tadmor-reseed.timer'
```

## 3. One-time box setup (already done; recorded for rebuild)

The box-level hardening (ufw default-deny with only 22/80/443, SSH key-only,
unattended-upgrades) predates tadmor and is not repeated here.

### 3.1 Postgres

Postgres comes from Debian main (17 on Debian 13) — deliberately not PGDG,
same repo-preference call as Caddy; the test suite passes on both 16 and 17.
It listens on localhost only (the Debian default).

```bash
sudo apt-get install -y postgresql
sudo -u postgres psql \
  -c "CREATE ROLE tadmor LOGIN PASSWORD '<generated>'" \
  -c "CREATE DATABASE tadmor OWNER tadmor"
```

The connection string lives only on the box, in `/etc/tadmor/env`
(root:root, mode 600), which systemd reads as root before dropping privileges:

```
DATABASE_URL=postgres://tadmor:<generated>@127.0.0.1:5432/tadmor?sslmode=disable
```

`sslmode=disable` is fine here and only here: the connection never leaves
loopback.

### 3.2 systemd service

The unit is committed at [`deploy/tadmor.service`](../deploy/tadmor.service):
`DynamicUser` (no app user to manage, no writable filesystem), strict
sandboxing (the process needs nothing but loopback TCP), `EnvironmentFile`
for the secret, `HTTP_ADDR` pinned to the app's localhost port.

```bash
scp deploy/tadmor.service vps:/tmp/
ssh vps 'sudo install -m 644 -o root -g root /tmp/tadmor.service /etc/systemd/system/tadmor.service \
         && rm /tmp/tadmor.service && sudo systemctl daemon-reload && sudo systemctl enable --now tadmor'
```

### 3.3 Caddy vhost

In `/etc/caddy/Caddyfile` on the box:

```
tadmor.belunaro.com {
	reverse_proxy 127.0.0.1:8081
}
```

then `sudo caddy validate --config /etc/caddy/Caddyfile && sudo systemctl
reload caddy`. Caddy obtains and renews the certificate automatically.

---

## 4. The container build (alternative path)

The repo-root `Dockerfile` still builds a self-contained image (`make image`)
for any container host. It is not used by the VPS deployment, but it mirrors
the same hermetic build discipline and stays maintained:

1. **SPA** — `node:22` + corepack-pinned `pnpm@10.18.0`, installed from the
   frozen lockfile, `pnpm build` → `web/dist`.
2. **Go** — `golang:1.25.11`, hermetic and vendored (`GOFLAGS=-mod=vendor`,
   `GOPROXY=off`, `GOTOOLCHAIN=local`, `CGO_ENABLED=0`), embedding the SPA from
   stage 1.
3. **Runtime** — `gcr.io/distroless/static:nonroot` carrying just the binary
   (migrations are embedded in it).

### 4.1 On pinning the base images (deferred, deliberately)

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

Calibration: **low probability, high impact.** For a demo the expected value is
small enough that pinning here would be closer to theater than defence, so we
skip it. When this project moves to a real production deployment, pin the two
**builder** images by verified digest (that is the part vendoring cannot
cover), with a deliberate process for bumping them — and justify it
**reproducibility-first** (a rebuild months later is provably the image you
audited, benefit with probability 1), **toolchain-integrity-second** (the
low-probability attack above).

Note that the VPS path (§1–§3) has no container builders at all: the binary is
built by the locally installed, pinned Go toolchain from the vendored tree, so
the base-image question doesn't arise.
