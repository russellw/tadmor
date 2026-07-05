import { expect, type APIRequestContext } from "@playwright/test"

// Throwaway rows created by the tests are named with this prefix so that
// global-teardown can find and delete them afterwards (the app has no hard-delete
// for master data — see customers.spec.ts).
export const E2E_PREFIX = "E2E-"

/** The login user global-setup creates for the run (and teardown removes).
 *  Its password is random per run and handed to tests via E2E_PASSWORD. */
export const E2E_EMAIL = "e2e@tadmor.test"

/** A unique organization name per test, so parallel workers never collide and
 *  the name is a reliable anchor for locating the row in the list. */
export function uniqueOrgName(): string {
  return `${E2E_PREFIX}${Date.now()}-${Math.random().toString(36).slice(2, 7)}`
}

/** Create a throwaway organization via the API and return its id. Used as setup
 *  so the customer create form (which only offers organizations without a
 *  customer) always has a free organization to choose. */
export async function createOrganization(
  request: APIRequestContext,
  name: string,
): Promise<number> {
  const res = await request.post("/api/organizations", {
    data: {
      name,
      legal_name: null,
      tax_id: null,
      country_code: null,
      default_currency: null,
    },
  })
  expect(res.ok(), `create org failed (${res.status()})`).toBeTruthy()
  return ((await res.json()) as { id: number }).id
}

/** Create a customer on an organization via the API and return its id. Used to
 *  set up the edit/deactivate tests without driving the create form each time. */
export async function createCustomer(
  request: APIRequestContext,
  organizationId: number,
  overrides: Record<string, unknown> = {},
): Promise<number> {
  const res = await request.post("/api/customers", {
    data: {
      organization_id: organizationId,
      customer_number: null,
      ar_account_id: null,
      payment_terms_code: null,
      currency_code: null,
      tax_code: null,
      credit_limit: null,
      is_active: true,
      ...overrides,
    },
  })
  expect(res.ok(), `create customer failed (${res.status()})`).toBeTruthy()
  return ((await res.json()) as { id: number }).id
}
