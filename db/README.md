# Database

Postgres schema, managed as ordered migrations.

## Layout

```
db/migrations/NNNNNN_name.up.sql    -- forward migration
db/migrations/NNNNNN_name.down.sql  -- reverse migration
db/seed/                            -- optional full ISO reference data (see below)
```

Files follow the [golang-migrate](https://github.com/golang-migrate/migrate)
naming convention so the same files work from the CLI and from Go.

## Conventions

- **Natural keys** where a stable international standard exists
  (`countries.code` = ISO 3166-1 alpha-2, `currencies.code` = ISO 4217).
- **Synthetic keys** are `int GENERATED ALWAYS AS IDENTITY` (auto-incrementing).
- Timestamps are `timestamptz`; `updated_at` is maintained by the shared
  `set_updated_at()` trigger (see `000001_init.up.sql`).
- Case-insensitive text (emails) uses the `citext` type.

## Running migrations

With the [migrate CLI](https://github.com/golang-migrate/migrate/tree/master/cmd/migrate):

```sh
export DATABASE_URL='postgres://localhost:5432/tadmor?sslmode=disable'
migrate -path db/migrations -database "$DATABASE_URL" up
migrate -path db/migrations -database "$DATABASE_URL" down 1   # roll back one step
```

## Full ISO reference data

Migrations seed only a starter subset of countries and currencies. The complete
ISO 3166-1 / ISO 4217 lists live in `db/seed/iso_reference.sql`, applied with
`make seed-iso` (additive and idempotent — `ON CONFLICT DO NOTHING`, existing
rows are never modified). The SQL is generated from the Debian `iso-codes`
package by `db/seed/gen_iso_reference.py` (stdlib-only Python); minor units and
display symbols are hand-maintained maps in that script, since neither is in
the iso-codes data. Currency codes with no ISO-defined minor unit (metals,
bond indices, XDR and kin, XTS, XXX) are excluded.

## Current schema

| Table           | Key            | Purpose                                    |
| --------------- | -------------- | ------------------------------------------ |
| `countries`     | natural (a-2)  | ISO 3166-1 reference data                  |
| `currencies`    | natural (a-3)  | ISO 4217 reference data                    |
| `users`         | synthetic int  | People who log in                          |
| `organizations` | synthetic int  | Any tracked business entity                |
| `addresses`     | synthetic int  | Postal addresses for organizations         |
| `contacts`      | synthetic int  | People associated with an organization     |
| `account_types` | natural        | Asset/liability/equity/revenue/expense     |
| `accounts`      | synthetic int  | Chart of accounts (hierarchical)           |
| `fiscal_years`  | synthetic int  | Reporting years                            |
| `accounting_periods` | synthetic int | Bookkeeping periods (open/closed)       |
| `journal_entries` | synthetic int | Balanced transactions (header)            |
| `journal_lines` | synthetic int  | Individual debits/credits (detail)         |
| `trial_balance` | view           | Per-account totals over posted entries     |
| `payment_terms` | natural        | Net 15/30/60 lookup                        |
| `tax_codes`     | natural        | Sales-tax rates → GL liability account     |
| `products`      | synthetic int  | Sellable items → revenue account           |
| `customers`     | synthetic int  | "Customer" role, 1:1 on an organization    |
| `sales_invoices` | synthetic int | Invoice header                             |
| `sales_invoice_lines` | synthetic int | Invoice detail (line money is computed) |
| `customer_payments` | synthetic int | Receipts from customers                  |
| `payment_applications` | synthetic int | Allocation of receipts to invoices    |
| `sales_credit_notes` | synthetic int | Customer credit note header             |
| `sales_credit_note_lines` | synthetic int | Credit note detail (line money is computed) |
| `sales_credit_applications` | synthetic int | Allocation of credit notes to invoices |
| `sales_invoice_balances` | view  | Per-invoice outstanding balance + status   |
| `sales_credit_note_balances` | view | Per-note unapplied credit + status      |
| `ar_aging`      | view           | A/R aging buckets by customer              |
| `suppliers`     | synthetic int  | "Supplier" role, 1:1 on an organization    |
| `purchase_bills` | synthetic int | Vendor bill header                         |
| `purchase_bill_lines` | synthetic int | Bill detail (line money is computed)   |
| `supplier_payments` | synthetic int | Payments made to suppliers               |
| `bill_applications` | synthetic int | Allocation of payments to bills          |
| `purchase_credit_notes` | synthetic int | Supplier credit note header          |
| `purchase_credit_note_lines` | synthetic int | Credit note detail (line money is computed) |
| `purchase_credit_applications` | synthetic int | Allocation of credit notes to bills |
| `purchase_bill_balances` | view  | Per-bill outstanding balance + status      |
| `purchase_credit_note_balances` | view | Per-note unapplied credit + status   |
| `ap_aging`      | view           | A/P aging buckets by supplier              |

### Purchasing / AP invariants (enforced in-database)

Mirror of the Sales/AR rules: bill line money is computed by generated columns;
draft bill headers auto-sync to their lines (posted/void bills frozen); a payment
and the bill it pays must share the same **supplier** and **currency**; neither a
payment nor a bill may be **over-applied**. The supplier's invoice number is
unique **per supplier** (`UNIQUE (supplier_id, bill_number)`), not globally.

| Table           | Key            | Purpose                                    |
| --------------- | -------------- | ------------------------------------------ |
| `warehouses`    | synthetic int  | Stock locations                            |
| `stock_movements` | synthetic int | Append-only ledger of on-hand +/- changes |
| `inventory_levels` | natural (composite) | Per product/warehouse reorder settings |
| `stock_on_hand` | view           | Qty + value on hand per product/warehouse  |
| `stock_valuation` | view         | Qty + value per product (all warehouses)   |
| `stock_below_reorder` | view     | Items at/below their reorder point         |

(`products` also gains `track_inventory`, `inventory_account_id`, `cogs_account_id`.)

### Inventory invariants (enforced in-database)

- The **movement ledger is the source of truth**; quantity-on-hand and value are
  derived by summing movements, never stored as a mutable balance.
- A movement's `quantity` sign must agree with its `movement_type` (receipts
  positive, issues negative, adjustments either way).
- Movements may only reference **inventory-tracked, active** products.
- Valuation is **method-agnostic**: each movement stores the `unit_cost` it
  occurred at; the service layer computes issue costs per its policy.

### Sales / AR invariants (enforced in-database)

- Invoice line money (`line_subtotal`, `tax_amount`, `line_total`) is computed by
  generated columns; draft invoice headers are kept in sync with their lines by
  trigger, and posted/void invoices are frozen.
- A payment and the invoice it pays must share the same **customer** and
  **currency**, and neither a payment nor an invoice may be **over-applied**.
- Invoices and payments carry nullable `journal_entry_id` / `period_id` hooks;
  **creating** the GL journal entry is the service layer's responsibility.

### Accounting invariants (enforced in-database)

- A **posted** journal entry must balance (Σ debits = Σ credits) and have lines —
  checked by a deferred constraint trigger at commit, so drafts can be built up
  across statements.
- Journal lines may only post to **postable, active** accounts.
- Nothing may be written against a **closed** accounting period.
