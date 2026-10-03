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
- ~~**`-adduser` only creates admins**~~ — done 2026-07-10: `-adduser` gained
  an `-admin` flag (defaults to true, preserving the bootstrap behavior);
  `-admin=false` provisions an ordinary login, and `UpsertUser` now carries the
  flag through (resetting an existing user's admin status to match on conflict).
  This replaces the hand-rolled SQL upsert the demo's `guest@demo` account
  needed; `docs/deployment.md` §2.1 documents the guest-account invocation.

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
  Turning it on in production is now a single config flip (`SMTP_ADDR`,
  `SMTP_USER`, `SMTP_PASS`, `MAIL_FROM`); the one remaining follow-up is the
  belunaro.com mail records (SPF/DKIM) the deferred mail-records decision
  covers. Front-end button done 2026-07-09: an Email button on each of the six
  detail screens (shared `EmailDocumentPanel`, wired into both
  `document-detail` and `order-detail`) opens an inline panel to type
  recipient(s) and send; with no SMTP it surfaces the 501 "email sending is not
  configured" inline, so it's a no-op on the demo (covered by
  `e2e/tests/email.spec.ts`). Counterparty-email fallback done 2026-07-09:
  organizations gained a nullable `email` column (migration 000019, editable on
  the organization form); a `/email` request with an empty `to` now resolves
  the recipient from the customer or supplier organization and echoes the
  address it used, an explicit `to` still overrides it, and a counterparty with
  no email on file and no `to` is a 422. The recipient field is therefore
  optional in the panel ("leave blank to use the counterparty's email on
  file").
- ~~**New-month period creation is manual ops.**~~ — done 2026-07-08: posting
  now auto-creates the calendar-month period (clipped to the fiscal year's
  bounds) when the document date falls inside an open fiscal year that has no
  period covering it; closed periods and closed fiscal years still reject.
  The fiscal-year-rollover residual was resolved by the year-end close
  (2026-07-09), which auto-creates the next fiscal year.

## Counterpart implementations

- **Spec and conformance suite** — done 2026-10-03: [`spec/`](../spec/)
  pins down the API and business rules independently of the stack, and
  [`conformance/`](../conformance/) checks any implementation against them
  over HTTP (`make conformance`; tadmor passes all 35 cases). This is the
  shared definition of "done" for counterpart projects on other stacks.
  Writing it surfaced three 500s on client errors (unparseable dates, a
  posting with no exchange rate) and a 200 for lines of a missing bank
  statement, plus a posting bug: a document whose lines netted negative on
  one account (e.g. a discount line on its own revenue account) answered
  500. All are fixed; such an account now posts on the opposite side.
- **Where counterparts live** — decided 2026-10-03: each counterpart is a
  separate repository carrying a copy of `spec/` and `conformance/` taken at
  a known tadmor commit by `spec/export.sh` (recorded in `spec/UPSTREAM`).
  tadmor owns the spec, and copies are re-exported, never edited in place.
  `conformance/` is its own dependency-free Go module so the copy runs
  anywhere (`spec/README.md`).
- **Comparison metrics** — decided 2026-10-03: primary are distinct
  maintainers trusted, dependency count, and how hard a hermetic build is;
  third-party size, own lines, effort, and runtime performance are nice to
  have. Definitions, procedures, and tadmor's baseline are in
  [`docs/counterpart-metrics.md`](counterpart-metrics.md), measured by
  `tools/measure.py`. Headline: the Go backend is 6 dependencies and 2
  maintainers and builds hermetically (level 4); the front end and its
  toolchain carry nearly all of the surface (173 npm packages, 179
  maintainers across runtime and build; level 1).
- **API consistency pass** — done 2026-10-03, before any counterpart
  copies the API: reads use the same field names as writes (e.g.
  `invoice_number`, not `number`); every update, delete, and data-less
  state transition is a 204; 400 means "cannot interpret the request" and
  value rules are 422; stock movements gained a derived `status`; the
  valuation report moved to `/inventory-valuation`; unknown API routes
  answer a JSON 404 (`spec/api.md` §1).
- **Spec gaps closed** — done 2026-10-03, before the first export:
  "today" is the UTC date (tadmor pins its database sessions to UTC);
  request decimals round half away from zero to their stored scale before
  any check, with stated ranges (422 beyond them); every rounded quotient
  is exact (migration 000020: Postgres division could round tax and
  average cost twice on extreme values); the session cookie flags are now
  checked; and receiving a foreign-currency purchase order values stock in
  base at the movement date's rate (it was unconverted), with no rate a
  422. The remaining FX difference between receipt and bill stays in GRNI
  (domain §14).
- **Counterparts build their own UI** — decided 2026-10-03: a counterpart
  is a whole product, with a UI in its own stack and its own code covering
  `spec/domain.md` §13 (not tadmor's `web/`), because the front end
  carries nearly all of tadmor's supply-chain surface. A counterpart is
  measured only once the suite passes and the UI covers §13
  (`docs/counterpart-metrics.md`).
- **Next:** pick the first counterpart stack.

## Smaller housekeeping

- **Front-end dependency advisories** — found 2026-10-03: `pnpm audit`
  reports 13 advisories (7 high), so `make web-check` fails. Shipped to
  browsers: `react-router` (fixed in 7.18.2) and `echarts` (moderate,
  fixed in 6.1.0, a major upgrade). Build-time only: `postcss`, `nanoid`,
  `browserslist`, `baseline-browser-mapping` (via vite and
  `@vitejs/plugin-react`). Upgrade under the cooldown policy
  (`docs/frontend-stack.md`).

- ~~**README front-matter is stale**~~ — done 2026-07-09 (commit e847809): the
  tagline no longer calls the front end "to come", the layout block now lists
  `web/`, `deploy/`, `docs/`, and `db/seed/`, and the build section notes that
  `make build` embeds the current `web/dist` (use `make release` / `make
  web-build` after a front-end change).
- ~~**Demo dataset upkeep**~~ — done 2026-07-09: any curated prod-data change
  must be followed by `make demo-snapshot` or the nightly reseed reverts it.
  The `deploy` target now prints that reminder on success as a guard.
