# Roadmap

What's still to do, as of 2026-07-08. The core loop is in place — master data,
GL with P&L / balance sheet / trial balance / ledgers / aging / inventory
valuation, invoices, bills, credit notes, payments, sales and purchase orders
with partial fulfilment, stock movements with GRNI, auth + roles — and the demo
is live at https://tadmor.belunaro.com with a nightly reseed. The items below
are what's known to be missing, gathered from the docs' "status and next step"
sections, deferred decisions, and a gap review against the project goal.

## Explicitly documented next steps

- **Broaden e2e coverage** (`docs/e2e-testing.md` §9). The master-data
  screens, reports, and all eight document screens (AR and AP: invoices,
  bills, payments both ways, credit notes both ways, sales and purchase
  orders — including posting and payment/credit application) have specs, and
  `make e2e` runs self-contained against a dedicated `tadmor_e2e` database.
  What's left is optional depth: stock movements, order ship/receive
  fulfilment, and unpost have no specs.
- ~~**Full ISO country/currency seed script**~~ — done 2026-07-08:
  `db/seed/gen_iso_reference.py` generates the committed
  `db/seed/iso_reference.sql` from Debian's `iso-codes` package;
  `make seed-iso` applies it (additive, idempotent).

## Deliberately deferred decisions worth revisiting

- **Dockerfile base-image pinning** (`docs/deployment.md` §4.1) — deferred
  with reasoning recorded; revisit if the container path ever becomes the real
  deployment route (the VPS uses the static binary, so low priority).
- **belunaro.com mail records** — the old OVH MX/SPF records were kept "for
  now" when DNS moved; keep, replace, or drop mail on the domain is still
  undecided.
- **`-adduser` only creates admins** — the guest demo account had to be
  provisioned by a hand-rolled SQL upsert. Let the CLI (or an admin screen
  path) create non-admin users properly.

## Functional gaps toward "comprehensive business management"

- **Year-end close.** Period locking already works — the Periods screen has a
  Close/Reopen toggle and a DB trigger rejects postings into closed periods —
  but there's no year-end workflow: nothing rolls retained earnings into the
  new fiscal year.
- **Multi-currency.** A currencies table exists, but nothing in the schema or
  handlers references exchange rates — all documents are effectively
  single-currency. If foreign-currency customers/suppliers are in scope, this
  is a large schema-and-posting-logic item.
- **Cash-flow statement** — the one classic financial statement missing
  alongside P&L and balance sheet.
- **Bank reconciliation** — payments post to the GL, but there's no statement
  import or matching.
- **Document output** — no PDF/print rendering or emailing of invoices; the
  app is screen-only.
- **New-month period creation is manual ops.** The Periods screen makes it
  easy, but posting still hard-fails when a month rolls over without someone
  adding a period ("no open accounting period for the document date" — it has
  already bitten prod once). Either auto-create the next period or surface a
  louder warning ahead of time.

## Smaller housekeeping

- **README front-matter is stale**: it still says a front end is "to come",
  and its layout section omits `web/`, `deploy/`, and `docs/`.
- **Demo dataset upkeep**: any curated prod-data change must be followed by
  `make demo-snapshot` or the nightly reseed reverts it. A guard (e.g. a
  reminder in the deploy target) could help if this keeps causing surprises.
