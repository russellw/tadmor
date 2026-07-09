# Roadmap

What's still to do, as of 2026-07-09. The core loop is in place — master data,
GL with P&L / balance sheet / cash flow / trial balance / ledgers / aging /
inventory valuation, invoices, bills, credit notes, payments, sales and purchase orders
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

- ~~**Year-end close.**~~ — done 2026-07-09: admins close a fiscal year from
  the Periods screen; a closing entry (flagged `is_closing`, ignored by the
  P&L) sweeps revenue/expense into a chosen retained-earnings account, all the
  year's periods and the year lock (a DB trigger keeps periods of a closed
  year shut), and the next fiscal year is auto-created — which also resolves
  the fiscal-year-rollover residual from the period auto-creation item below.
  Reopen reverses the closing entry. Years close oldest-first and reopen
  newest-first.
- **Multi-currency.** A currencies table exists, but nothing in the schema or
  handlers references exchange rates — all documents are effectively
  single-currency. If foreign-currency customers/suppliers are in scope, this
  is a large schema-and-posting-logic item.
- ~~**Cash-flow statement**~~ — done 2026-07-09: indirect-method statement at
  Reports → Cash Flow (`GET /api/cash-flow`): net income plus each non-cash
  balance-sheet account's cash impact, grouped operating/investing/financing
  and reconciled to opening/closing cash. Accounts gained `is_cash` (which
  accounts *are* cash; seeded/backfilled by name) and `cash_flow_activity`
  (which section a non-cash account's movements belong to), both editable on
  the account form.
- **Bank reconciliation** — payments post to the GL, but there's no statement
  import or matching.
- **Document output** — PDFs done 2026-07-09: all six printable documents
  (sales invoices, bills, credit notes both ways, sales and purchase orders)
  render as PDFs (`GET /api/<collection>/{id}/pdf`, PDF button on each detail
  screen) via a stdlib-only writer in `internal/pdf` (standard-14 Helvetica,
  widths generated from the Adobe AFMs) and one shared layout in
  `internal/printing` driven by a per-document spec (labels + queries). The
  issuer block comes from the organization flagged `is_self` (checkbox on the
  organization form; at most one). Still open: emailing.
- ~~**New-month period creation is manual ops.**~~ — done 2026-07-08: posting
  now auto-creates the calendar-month period (clipped to the fiscal year's
  bounds) when the document date falls inside an open fiscal year that has no
  period covering it; closed periods and closed fiscal years still reject.
  The fiscal-year-rollover residual was resolved by the year-end close
  (2026-07-09), which auto-creates the next fiscal year.

## Smaller housekeeping

- **README front-matter is stale**: it still says a front end is "to come",
  and its layout section omits `web/`, `deploy/`, and `docs/`.
- **Demo dataset upkeep**: any curated prod-data change must be followed by
  `make demo-snapshot` or the nightly reseed reverts it. A guard (e.g. a
  reminder in the deploy target) could help if this keeps causing surprises.
