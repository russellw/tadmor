# Roadmap

What's still to do, as of 2026-07-09. The core loop is in place — master data,
GL with P&L / balance sheet / cash flow / trial balance / ledgers / aging /
inventory valuation, invoices, bills, credit notes, payments, sales and purchase orders
with partial fulfilment, stock movements with GRNI, auth + roles — and the demo
is live at https://tadmor.belunaro.com with a nightly reseed. The items below
are what's known to be missing, gathered from the docs' "status and next step"
sections, deferred decisions, and a gap review against the project goal.

## Explicitly documented next steps

- ~~**Broaden e2e coverage**~~ — the three remaining uncovered screens got
  specs 2026-07-09: `stock-movements.spec.ts` (create a receipt, post/unpost it,
  delete an unposted one), `fulfilment.spec.ts` (the stock axis of orders —
  ship a sales order and receive a purchase order, each with a stocked line),
  and `unpost.spec.ts` (the admin-only unpost that reverses a posted invoice or
  bill back to draft). The suite is now twenty-two spec files and still tears
  down to zero rows (teardown gained a stock-movements sweep). What's left is
  optional *depth*, not whole screens: partial fulfilment, the transfer/
  adjustment movement types, and the document email button once one exists.
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
- ~~**Multi-currency.**~~ — done 2026-07-09: the ledger has a base
  (functional) currency and an FX gain/loss account (Setup → Settings, backed
  by a one-row `gl_settings` a trigger freezes once entries exist), plus
  manually maintained `exchange_rates` (Accounting → Exchange Rates, one rate
  per currency per date). Every journal line now carries its amount twice — in
  the document's transaction currency (`debit`/`credit`, what bank rec matches)
  and converted to base at the document date's latest rate
  (`base_debit`/`base_credit`, what every report sums); `journal_entries`
  records the `exchange_rate` used, and a document with no covering rate can't
  post. Settling a payment or credit note against a document booked at a
  different rate posts a realized FX entry to the gain/loss account (linked via
  `fx_journal_entry_id` on the application, reversed on unpost). Reports, the
  trial balance, the account ledger, and the journal-entry drill-down all read
  base amounts and surface the transaction/base split on foreign-currency
  rows. Deliberately out of scope for now, each worth its own item later:
  **cross-currency settlement** (a payment must still match its document's
  currency — the application triggers enforce it); **period-end revaluation**
  of open foreign-currency AR/AP/bank balances at a closing rate (only
  *realized* differences post today); and a sub-cent rounding residual can
  remain on a foreign document settled across several partial payments (each
  installment rounds independently).
- ~~**Cash-flow statement**~~ — done 2026-07-09: indirect-method statement at
  Reports → Cash Flow (`GET /api/cash-flow`): net income plus each non-cash
  balance-sheet account's cash impact, grouped operating/investing/financing
  and reconciled to opening/closing cash. Accounts gained `is_cash` (which
  accounts *are* cash; seeded/backfilled by name) and `cash_flow_activity`
  (which section a non-cash account's movements belong to), both editable on
  the account form.
- ~~**Bank reconciliation**~~ — done 2026-07-09: statements are captured per
  cash account at Accounting → Bank Reconciliation (CSV import —
  `date,description,amount[,reference]` — or manual lines), matched 1:1
  against posted journal lines on the account (auto-match pairs equal amounts
  preferring the nearest entry date; a per-line picker resolves the rest),
  and reconciled once every line is matched and opening + lines = closing.
  Database triggers enforce the invariants (cash accounts only, match
  amount/account/posted checks, reconciled statements frozen); unpost refuses
  entries with matched lines, and reopen is admin-only.
- **Document output** — PDFs done 2026-07-09: all six printable documents
  (sales invoices, bills, credit notes both ways, sales and purchase orders)
  render as PDFs (`GET /api/<collection>/{id}/pdf`, PDF button on each detail
  screen) via a stdlib-only writer in `internal/pdf` (standard-14 Helvetica,
  widths generated from the Adobe AFMs) and one shared layout in
  `internal/printing` driven by a per-document spec (labels + queries). The
  issuer block comes from the organization flagged `is_self` (checkbox on the
  organization form; at most one). Emailing is built but inert (2026-07-09):
  `internal/mailer` is a stdlib-only (`net/smtp`) sender behind a `Mailer`
  interface whose default is a no-op that reports `ErrNotConfigured`, selected
  whenever `SMTP_ADDR` is unset — so the demo, which sets no SMTP environment,
  never sends. `POST /api/<collection>/{id}/email` renders the same PDF as the
  download endpoint and attaches it; with no mailer configured it returns 501.
  Turning it on in production is a config flip (`SMTP_ADDR`, `SMTP_USER`,
  `SMTP_PASS`, `MAIL_FROM`) plus two follow-ups: an `organizations.email`
  column so the recipient resolves from the counterparty (today it comes from
  an optional `to` in the request body), and the belunaro.com mail records
  (SPF/DKIM) the deferred mail-records decision covers. No front-end button yet.
- ~~**New-month period creation is manual ops.**~~ — done 2026-07-08: posting
  now auto-creates the calendar-month period (clipped to the fiscal year's
  bounds) when the document date falls inside an open fiscal year that has no
  period covering it; closed periods and closed fiscal years still reject.
  The fiscal-year-rollover residual was resolved by the year-end close
  (2026-07-09), which auto-creates the next fiscal year.

## Smaller housekeeping

- ~~**README front-matter is stale**~~ — done 2026-07-09 (commit e847809): the
  tagline no longer calls the front end "to come", the layout block now lists
  `web/`, `deploy/`, `docs/`, and `db/seed/`, and the build section notes that
  `make build` embeds the current `web/dist` (use `make release` / `make
  web-build` after a front-end change).
- ~~**Demo dataset upkeep**~~ — done 2026-07-09: any curated prod-data change
  must be followed by `make demo-snapshot` or the nightly reseed reverts it.
  The `deploy` target now prints that reminder on success as a guard.
