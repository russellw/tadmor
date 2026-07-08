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

/** A random far-future year for fiscal-calendar tests. Accounting periods have
 *  a global no-overlap constraint (not per fiscal year), so test periods must
 *  sit in date ranges no other run — or the real seeded calendar — touches. */
export function uniqueFarYear(): number {
  return 2200 + Math.floor(Math.random() * 700)
}

/** Create a throwaway fiscal year via the API and return its id. Named with
 *  E2E_PREFIX so global-teardown removes it (and its periods). */
export async function createFiscalYear(
  request: APIRequestContext,
  name: string,
  startDate: string,
  endDate: string,
): Promise<number> {
  const res = await request.post("/api/fiscal-years", {
    data: { name, start_date: startDate, end_date: endDate, status: "open" },
  })
  expect(res.ok(), `create fiscal year failed (${res.status()})`).toBeTruthy()
  return ((await res.json()) as { id: number }).id
}

/** Create an accounting period via the API and return its id. */
export async function createPeriod(
  request: APIRequestContext,
  fiscalYearId: number,
  name: string,
  startDate: string,
  endDate: string,
): Promise<number> {
  const res = await request.post("/api/accounting-periods", {
    data: {
      fiscal_year_id: fiscalYearId,
      name,
      start_date: startDate,
      end_date: endDate,
      status: "open",
    },
  })
  expect(res.ok(), `create period failed (${res.status()})`).toBeTruthy()
  return ((await res.json()) as { id: number }).id
}

/** A unique E2E-prefixed code for natural-key rows (tax codes, warehouses).
 *  Uppercase because the forms normalize codes to uppercase. */
export function uniqueCode(): string {
  return `${E2E_PREFIX}${Date.now().toString(36)}${Math.random()
    .toString(36)
    .slice(2, 5)}`.toUpperCase()
}

/** Create a throwaway tax code via the API and return its code. */
export async function createTaxCode(
  request: APIRequestContext,
  code: string,
  overrides: Record<string, unknown> = {},
): Promise<string> {
  const res = await request.post("/api/tax-codes", {
    data: {
      code,
      name: `${code} tax`,
      rate: "0",
      tax_account_id: null,
      is_active: true,
      ...overrides,
    },
  })
  expect(res.ok(), `create tax code failed (${res.status()})`).toBeTruthy()
  return code
}

/** A unique E2E-prefixed email for user-admin tests, lowercase to match how
 *  emails are usually typed (the users table is citext). */
export function uniqueEmail(): string {
  return `e2e-${Date.now().toString(36)}${Math.random()
    .toString(36)
    .slice(2, 5)}@tadmor.test`
}

/** Create a throwaway login user via the API and return its id. */
export async function createUser(
  request: APIRequestContext,
  email: string,
): Promise<number> {
  const res = await request.post("/api/users", {
    data: { email, full_name: "E2E Throwaway", password: "e2e-password" },
  })
  expect(res.ok(), `create user failed (${res.status()})`).toBeTruthy()
  return ((await res.json()) as { id: number }).id
}

/** Create a throwaway payment term via the API and return its code. */
export async function createPaymentTerm(
  request: APIRequestContext,
  code: string,
  dueDays = 0,
): Promise<string> {
  const res = await request.post("/api/payment-terms", {
    data: { code, name: `${code} terms`, due_days: dueDays },
  })
  expect(res.ok(), `create payment term failed (${res.status()})`).toBeTruthy()
  return code
}

/** Create a throwaway warehouse via the API and return its id. */
export async function createWarehouse(
  request: APIRequestContext,
  code: string,
  name: string,
): Promise<number> {
  const res = await request.post("/api/warehouses", {
    data: { code, name, address_id: null, is_active: true },
  })
  expect(res.ok(), `create warehouse failed (${res.status()})`).toBeTruthy()
  return ((await res.json()) as { id: number }).id
}

/** Create a supplier on an organization via the API and return its id. Used to
 *  set up the edit/deactivate tests without driving the create form each time. */
export async function createSupplier(
  request: APIRequestContext,
  organizationId: number,
  overrides: Record<string, unknown> = {},
): Promise<number> {
  const res = await request.post("/api/suppliers", {
    data: {
      organization_id: organizationId,
      supplier_number: null,
      ap_account_id: null,
      payment_terms_code: null,
      currency_code: null,
      tax_code: null,
      is_active: true,
      ...overrides,
    },
  })
  expect(res.ok(), `create supplier failed (${res.status()})`).toBeTruthy()
  return ((await res.json()) as { id: number }).id
}

/** Create a throwaway catalog product via the API and return its id. The SKU
 *  carries the E2E_PREFIX so global-teardown removes the row. */
export async function createProduct(
  request: APIRequestContext,
  sku: string,
  name: string,
  overrides: Record<string, unknown> = {},
): Promise<number> {
  const res = await request.post("/api/products", {
    data: {
      sku,
      name,
      description: null,
      unit_price: "0",
      currency_code: null,
      revenue_account_id: null,
      tax_code: null,
      track_inventory: false,
      inventory_account_id: null,
      cogs_account_id: null,
      is_active: true,
      ...overrides,
    },
  })
  expect(res.ok(), `create product failed (${res.status()})`).toBeTruthy()
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
