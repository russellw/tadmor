# Deploying an app to the belunaro VPS

This document is self-contained and meant to be **copied into the repo of any
new app** that will be hosted on the belunaro server. It tells you (or a
Claude Code session working in that repo) everything needed to deploy without
consulting any other project. Throughout, `foo` is the placeholder app name —
substitute your app's real name everywhere it appears.

The canonical copy lives in the tadmor repo at
`docs/belunaro-app-deployment.md`; if the server setup changes, update it
there and re-copy.

## The server

- **Box**: OVH VPS `vps-22e743f5`, Debian 13, IP `57.129.138.32`. Fixed-price
  hosting by design — no hyperscalers, no per-request billing.
- **SSH**: alias `vps` in `~/.ssh/config` (user `debian`). Key
  `~/.ssh/ovh-vps` is passphrase-protected and served by an ssh-agent on the
  fixed socket `~/.ssh/agent.sock` (wired up via `IdentityAgent` in the
  config). After a local reboot the agent is empty; reload with:

  ```sh
  SSH_AUTH_SOCK=~/.ssh/agent.sock ssh-add ~/.ssh/ovh-vps
  ```

- **Firewall**: ufw default-deny; only 22, 80, 443 are open. Apps bind
  localhost and are reached only through Caddy. **Never open additional
  ports.**
- **Hardening**: SSH password auth and root login are disabled;
  unattended-upgrades is on. If the key is lost, recovery is via the OVH
  web console/rescue mode only — be careful with `sshd` and ufw changes.
- **DNS**: `belunaro.com` apex and wildcard `*.belunaro.com` both point at
  the box (managed in the OVH panel). A new subdomain therefore requires
  **no DNS work at all** — `foo.belunaro.com` already resolves.

## The multi-app model

Several small apps share this box. Each app gets its own isolation boundary:

| Concern | Per-app resource |
|---|---|
| Process | One systemd service, sandboxed with `DynamicUser=yes` |
| Network | One localhost port, reverse-proxied by a Caddy vhost |
| Data | One Postgres role + one database, never shared |
| Secrets | `/etc/foo/env` (root:root, mode 600), injected via `EnvironmentFile=` |
| Binary | `/opt/foo/server` (or `/srv/www/foo` for static sites) |

Shared infrastructure (already installed, from Debian main repos —
deliberately not vendor/backports repos):

- **Caddy 2.6.2** — TLS termination and routing, `/etc/caddy/Caddyfile`,
  auto-HTTPS via Let's Encrypt. This is the only hand-edited shared file;
  each app contributes one vhost block.
- **Postgres 17** — localhost-only.

**Port registry**: there is no separate registry — the Caddyfile is it. Before
picking a port, read `/etc/caddy/Caddyfile` on the box and take the next free
one. Known allocations at the time of writing:

| Port | App |
|---|---|
| 8081 | tadmor (`tadmor.belunaro.com`) |

## App requirements (build these in from the start)

1. **Bind address from the environment.** The app must listen on an address
   given by an env var (e.g. `HTTP_ADDR=127.0.0.1:8082`), never a hardcoded
   port, and must bind localhost in production.
2. **Config via environment only.** `DATABASE_URL` and everything else comes
   from `/etc/foo/env`; no config files on the box.
3. **Embed migrations in the binary** and apply them at startup, treating
   "zero migration files found" as a fatal error. (Lesson learned on tadmor:
   reading `./db/migrations` from the CWD silently no-ops under systemd,
   because the service's working directory isn't the repo.)
4. **A health endpoint** (e.g. `GET /readyz` returning 200 once the DB is
   reachable) so deploys can be verified mechanically.
5. **A static, self-contained Linux binary** if at all possible:
   `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`. One file to copy, nothing to
   install on the box.
6. **Commit the systemd unit in the app repo** (convention: `deploy/foo.service`)
   so the deployed configuration is version-controlled.

## First-time setup (one-time, manual over SSH)

Run these on the box (`ssh vps`); everything below needs `sudo`.

### 1. Postgres role and database

```sh
PW=$(openssl rand -base64 24)
sudo -u postgres psql -c "CREATE ROLE foo LOGIN PASSWORD '$PW'"
sudo -u postgres createdb -O foo foo
```

### 2. Environment file

```sh
sudo mkdir -p /etc/foo
echo "DATABASE_URL=postgres://foo:$PW@127.0.0.1:5432/foo" | sudo tee /etc/foo/env
sudo chown root:root /etc/foo/env && sudo chmod 600 /etc/foo/env
unset PW
```

The password lives **only** in this file; there is no other copy.

### 3. Install the binary

From the dev machine:

```sh
scp bin/server vps:/tmp/foo-server
ssh vps 'sudo install -D -m 755 -o root -g root /tmp/foo-server /opt/foo/server && rm /tmp/foo-server'
```

### 4. systemd unit

Copy the committed `deploy/foo.service` to `/etc/systemd/system/foo.service`,
then:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now foo
systemctl status foo          # confirm it's running
curl -fsS http://127.0.0.1:8082/readyz && echo
```

Unit template (this is tadmor's proven unit with the name and port changed —
start from it, don't improvise):

```ini
[Unit]
Description=Foo server
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
ExecStart=/opt/foo/server
EnvironmentFile=/etc/foo/env
Environment=HTTP_ADDR=127.0.0.1:8082
Restart=on-failure
RestartSec=2

# Sandboxing: no state on disk, only needs loopback TCP to Postgres/Caddy.
DynamicUser=yes
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
PrivateDevices=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictAddressFamilies=AF_INET AF_INET6
LockPersonality=yes
MemoryDenyWriteExecute=yes
RestrictRealtime=yes
SystemCallFilter=@system-service
CapabilityBoundingSet=

[Install]
WantedBy=multi-user.target
```

Caveats on the sandbox directives: `MemoryDenyWriteExecute=yes` breaks
JIT runtimes (Node, JVM) — drop it for non-Go apps. If the app needs to
write files, add a `StateDirectory=foo` line rather than weakening
`ProtectSystem=strict`.

### 5. Caddy vhost

Append to `/etc/caddy/Caddyfile` on the box:

```
foo.belunaro.com {
	reverse_proxy 127.0.0.1:8082
}
```

Then — **always in this order**, since a bad Caddyfile takes down every site
on the box:

```sh
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

Caddy obtains and renews the Let's Encrypt certificate automatically.

### 6. Verify

```sh
curl -fsS https://foo.belunaro.com/readyz && echo
```

## Redeploys

After first-time setup, redeploying is just: build, copy, restart, verify.
Put this in the app's Makefile (tadmor's working pattern):

```make
deploy: export CGO_ENABLED=0
deploy: export GOOS=linux
deploy: export GOARCH=amd64
deploy: release ## Build and deploy to the VPS
	scp bin/server vps:/tmp/foo-server
	ssh vps 'sudo install -m 755 -o root -g root /tmp/foo-server /opt/foo/server && rm /tmp/foo-server && sudo systemctl restart foo'
	sleep 2 && curl -fsS https://foo.belunaro.com/readyz && echo
```

Schema migrations ride along automatically because they're embedded in the
binary and applied at startup.

## Static sites

For a plain static site there is no service, port, or database. Put the files
under `/srv/www/foo` and use a file-server vhost instead:

```
foo.belunaro.com {
	root * /srv/www/foo
	file_server
}
```

Validate and reload Caddy as above. Done.

## Rules for a shared box

- **Touch only your own resources**: your vhost block, `/opt/foo`,
  `/etc/foo`, your unit, your database. Never edit another app's vhost or
  restart another app's service.
- **Always `caddy validate` before `systemctl reload caddy`.**
- **Never open firewall ports or change sshd config** as part of an app
  deploy.
- **No DNS changes** are ever needed for `*.belunaro.com` subdomains.
- Logs: `journalctl -u foo` for the app, `journalctl -u caddy` for the front
  door.
