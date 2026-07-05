import { execFile } from "node:child_process"
import { promisify } from "node:util"

import { E2E_EMAIL, E2E_PREFIX } from "./tests/helpers"

const exec = promisify(execFile)

// The app has no hard-delete for master data (accounting entities are
// deactivated, not removed). Tests create throwaway organizations + customers
// named with E2E_PREFIX; we delete those rows directly via psql after the run so
// the dev database doesn't accumulate test data. psql is a system tool, so this
// adds no npm dependency.
//
// Connection defaults to the dev database used by `make run`; override with
// E2E_DATABASE_URL (e.g. to point at a dedicated test DB).
const DB =
  process.env.E2E_DATABASE_URL ??
  "postgres://tadmor:tadmor@127.0.0.1:5432/tadmor?sslmode=disable"

export default async function globalTeardown(): Promise<void> {
  // Delete customers first (FK to organizations), then the organizations, then
  // the run's login user (its sessions cascade). The prefix and email are fixed
  // literals, not user input, so interpolation is safe here.
  const sql = `
    DELETE FROM customers
     WHERE organization_id IN (
       SELECT id FROM organizations WHERE name LIKE '${E2E_PREFIX}%'
     );
    DELETE FROM organizations WHERE name LIKE '${E2E_PREFIX}%';
    DELETE FROM accounting_periods
     WHERE fiscal_year_id IN (
       SELECT id FROM fiscal_years WHERE name LIKE '${E2E_PREFIX}%'
     );
    DELETE FROM fiscal_years WHERE name LIKE '${E2E_PREFIX}%';
    DELETE FROM tax_codes WHERE code LIKE '${E2E_PREFIX}%';
    DELETE FROM payment_terms WHERE code LIKE '${E2E_PREFIX}%';
    DELETE FROM warehouses WHERE code LIKE '${E2E_PREFIX}%';
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
