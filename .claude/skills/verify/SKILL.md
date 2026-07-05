---
name: verify
description: Build, launch, and drive the tadmor app to verify a change end-to-end at its real surface (API or browser UI).
---

# Verifying tadmor changes at the runtime surface

## Build + launch

```sh
cd web && corepack pnpm build        # if the change touches web/src (embeds into the binary)
git checkout -- web/dist/.gitkeep    # raw `pnpm build` deletes it; `make web-build` restores it itself
make build                           # hermetic vendored Go build -> bin/server
DATABASE_URL='postgres://tadmor:tadmor@127.0.0.1:5432/tadmor?sslmode=disable' \
  HTTP_ADDR=127.0.0.1:8090 ./bin/server &   # 8090 avoids the dev-server default :8080
curl -s http://127.0.0.1:8090/readyz        # {"status":"ready"} when up
```

The binary serves both the API (`/api/*`) and the embedded SPA (everything
else, with index.html fallback for deep links).

## Driving the API

Plain curl. Master data first (FKs), then documents, then post:
create org → supplier/customer role → product/warehouse → document → `POST
/api/<doc>/{id}/post`. Posting requires the document date to fall in an **open
accounting period** — check `GET /api/accounting-periods` and pin dates inside
one (the dev DB's seeded period is 2026-06). Posting a stock movement needs a
JSON body: `{"currency":"USD","credit_account_id":<GRNI account>}`.

## Driving the UI

Playwright + Chromium are already installed in `e2e/` (see docs/e2e-testing.md
for one-time setup). For ad-hoc driving (not tests), put a script **inside
e2e/** so Node resolves `@playwright/test`, run it, delete it:

```sh
cd e2e && cat > .drive.tmp.mjs <<'EOF'
import { chromium } from "@playwright/test"
const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } })
page.on("pageerror", (e) => console.log("pageerror:", e.message))
await page.goto("http://127.0.0.1:8090/")
// click nav, read innerText, page.screenshot({ path: ... }) ...
await browser.close()
EOF
node .drive.tmp.mjs; rm .drive.tmp.mjs
```

Gotchas seen in practice:

- After `click()`, Playwright's mouse stays put — later screenshots show
  `:hover` styling on that nav item. Not an active-state bug; probe the DOM
  (`aria-current`, class list) before concluding anything from pixels.
- Exercise both a nav **click** and a direct **deep link** — the latter tests
  the Go spaHandler's index.html fallback.
- The dev DB persists whatever you create; either delete rows after or use
  obviously-fake names. The e2e suite's teardown only removes `E2E`-prefixed
  rows.
