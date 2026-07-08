import { execFile } from "node:child_process"
import { pbkdf2Sync, randomBytes } from "node:crypto"
import * as fs from "node:fs"
import * as path from "node:path"
import { promisify } from "node:util"

import { request, type FullConfig } from "@playwright/test"

import { E2E_EMAIL } from "./tests/helpers"

const exec = promisify(execFile)

// Same psql route (and connection default) as global-teardown: no npm
// dependency for database access. `make e2e` (run-local.sh) sets
// E2E_DATABASE_URL to the dedicated e2e database its server runs on; the
// dev-DB fallback exists only for `make e2e-test` against a running dev stack,
// where this must match that stack's DATABASE_URL.
const DB =
  process.env.E2E_DATABASE_URL ??
  "postgres://tadmor:tadmor@127.0.0.1:5432/tadmor?sslmode=disable"

// Every test context (browser and API alike) starts from this saved session —
// see use.storageState in playwright.config.ts.
const STORAGE_STATE = path.join(__dirname, ".auth", "state.json")

// The API requires a login session, so before any test runs we (1) upsert a
// dedicated user with a password that exists only for this run, (2) log in
// once over the API, and (3) persist the session cookie as storage state.
export default async function globalSetup(config: FullConfig): Promise<void> {
  const password = randomBytes(24).toString("base64url")

  // Hash exactly as internal/auth does: PBKDF2-HMAC-SHA256, encoded as
  // scheme$iterations$salt$key. Node's crypto produces the same derived key,
  // so the Go server verifies it directly.
  const iterations = 600_000
  const salt = randomBytes(16)
  const key = pbkdf2Sync(password, salt, iterations, 32, "sha256")
  const hash = `pbkdf2-sha256$${iterations}$${salt.toString("base64")}$${key.toString("base64")}`

  // The email is a fixed literal and the hash is base64 (no quotes), so
  // interpolation is safe here.
  const sql = `
    INSERT INTO users (email, full_name, password_hash, is_admin)
    VALUES ('${E2E_EMAIL}', 'E2E Test User', '${hash}', true)
    ON CONFLICT (email) DO UPDATE
      SET password_hash = EXCLUDED.password_hash, is_active = true, is_admin = true;
  `
  await exec("psql", [DB, "-v", "ON_ERROR_STOP=1", "-q", "-c", sql])

  const baseURL = config.projects[0]?.use?.baseURL ?? "http://localhost:5173"
  const ctx = await request.newContext({ baseURL })
  const res = await ctx.post("/api/auth/login", {
    data: { email: E2E_EMAIL, password },
  })
  if (!res.ok()) {
    throw new Error(`e2e login failed (${res.status()}): ${await res.text()}`)
  }
  fs.mkdirSync(path.dirname(STORAGE_STATE), { recursive: true })
  await ctx.storageState({ path: STORAGE_STATE })
  await ctx.dispose()

  // auth.spec.ts drives the real login form; hand it this run's password.
  process.env.E2E_PASSWORD = password
}
