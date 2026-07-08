import { execFile } from "node:child_process"
import { promisify } from "node:util"

import { E2E_EMAIL, E2E_PREFIX } from "./tests/helpers"

const exec = promisify(execFile)

// The app has no hard-delete for master data (accounting entities are
// deactivated, not removed). Tests create throwaway rows (organizations and
// their customer/supplier roles, products, calendars, codes, users) named with
// E2E_PREFIX; we delete those rows directly via psql after the run so
// the dev database doesn't accumulate test data. psql is a system tool, so this
// adds no npm dependency.
//
// `make e2e` (run-local.sh) sets E2E_DATABASE_URL to the dedicated e2e
// database its server runs on. The fallback is the dev database used by
// `make run`, for `make e2e-test` against a running dev stack — where this
// must match that stack's DATABASE_URL.
const DB =
  process.env.E2E_DATABASE_URL ??
  "postgres://tadmor:tadmor@127.0.0.1:5432/tadmor?sslmode=disable"

export default async function globalTeardown(): Promise<void> {
  // Delete customers first (FK to organizations), then the organizations, then
  // the run's login user (its sessions cascade). The prefix and email are fixed
  // literals, not user input, so interpolation is safe here.
  const sql = `
    -- Subledger documents hang off E2E customers, and their journal entries
    -- land in E2E one-day periods (documents must be dated inside the test's
    -- own period to post — see helpers.ts salesFixture). Applications first
    -- (they RESTRICT invoice deletion), then documents (lines cascade),
    -- invoices before orders (invoice lines reference order lines), and the
    -- journal entries after the documents that point at them.
    CREATE TEMP TABLE e2e_customers AS
      SELECT c.id FROM customers c
      JOIN organizations o ON o.id = c.organization_id
     WHERE o.name LIKE '${E2E_PREFIX}%';
    DELETE FROM payment_applications
     WHERE payment_id IN (
       SELECT id FROM customer_payments WHERE customer_id IN (SELECT id FROM e2e_customers)
     );
    DELETE FROM sales_credit_applications
     WHERE credit_note_id IN (
       SELECT id FROM sales_credit_notes WHERE customer_id IN (SELECT id FROM e2e_customers)
     );
    DELETE FROM customer_payments WHERE customer_id IN (SELECT id FROM e2e_customers);
    DELETE FROM sales_credit_notes WHERE customer_id IN (SELECT id FROM e2e_customers);
    DELETE FROM sales_invoices WHERE customer_id IN (SELECT id FROM e2e_customers);
    DELETE FROM sales_orders WHERE customer_id IN (SELECT id FROM e2e_customers);
    -- The AP mirror of the block above.
    CREATE TEMP TABLE e2e_suppliers AS
      SELECT s.id FROM suppliers s
      JOIN organizations o ON o.id = s.organization_id
     WHERE o.name LIKE '${E2E_PREFIX}%';
    DELETE FROM bill_applications
     WHERE payment_id IN (
       SELECT id FROM supplier_payments WHERE supplier_id IN (SELECT id FROM e2e_suppliers)
     );
    DELETE FROM purchase_credit_applications
     WHERE credit_note_id IN (
       SELECT id FROM purchase_credit_notes WHERE supplier_id IN (SELECT id FROM e2e_suppliers)
     );
    DELETE FROM supplier_payments WHERE supplier_id IN (SELECT id FROM e2e_suppliers);
    DELETE FROM purchase_credit_notes WHERE supplier_id IN (SELECT id FROM e2e_suppliers);
    DELETE FROM purchase_bills WHERE supplier_id IN (SELECT id FROM e2e_suppliers);
    DELETE FROM purchase_orders WHERE supplier_id IN (SELECT id FROM e2e_suppliers);
    DELETE FROM journal_entries
     WHERE period_id IN (
       SELECT p.id FROM accounting_periods p
       JOIN fiscal_years f ON f.id = p.fiscal_year_id
      WHERE f.name LIKE '${E2E_PREFIX}%'
     );
    DELETE FROM customers
     WHERE organization_id IN (
       SELECT id FROM organizations WHERE name LIKE '${E2E_PREFIX}%'
     );
    DELETE FROM suppliers
     WHERE organization_id IN (
       SELECT id FROM organizations WHERE name LIKE '${E2E_PREFIX}%'
     );
    DELETE FROM organizations WHERE name LIKE '${E2E_PREFIX}%';
    DELETE FROM products WHERE sku LIKE '${E2E_PREFIX}%';
    DELETE FROM accounting_periods
     WHERE fiscal_year_id IN (
       SELECT id FROM fiscal_years WHERE name LIKE '${E2E_PREFIX}%'
     );
    DELETE FROM fiscal_years WHERE name LIKE '${E2E_PREFIX}%';
    DELETE FROM tax_codes WHERE code LIKE '${E2E_PREFIX}%';
    DELETE FROM payment_terms WHERE code LIKE '${E2E_PREFIX}%';
    DELETE FROM warehouses WHERE code LIKE '${E2E_PREFIX}%';
    -- E2E accounts go last: journal lines (cascaded with their entries above)
    -- and the E2E customers' AR links are gone by now.
    DELETE FROM accounts WHERE code LIKE '${E2E_PREFIX}%';
    DELETE FROM users WHERE email = '${E2E_EMAIL}' OR email LIKE 'e2e-%@tadmor.test';
  `
  try {
    await exec("psql", [DB, "-v", "ON_ERROR_STOP=1", "-q", "-c", sql])
  } catch (err) {
    // Don't fail the whole run on teardown trouble — surface it so leftover rows
    // can be cleaned up manually.
    console.error("e2e global-teardown (psql) failed:", err)
  }
}
