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

/** A random single far-future day for document tests, in years 2900–3599 —
 *  disjoint from uniqueFarYear's 2200–2899 so document-test periods can never
 *  overlap the fiscal-calendar tests' whole-year ranges. Documents must be
 *  dated inside an open accounting period to post, and the dev database is not
 *  guaranteed to have one for today, so each test brings its own one-day
 *  period. */
export function uniqueDocDay(): string {
  const year = 2900 + Math.floor(Math.random() * 700)
  const month = 1 + Math.floor(Math.random() * 12)
  const day = 1 + Math.floor(Math.random() * 28)
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${year}-${pad(month)}-${pad(day)}`
}

/** Create a postable E2E account of the given type and return it. The code
 *  carries the E2E_PREFIX so global-teardown removes the row. */
export async function createAccount(
  request: APIRequestContext,
  accountType: "asset" | "liability" | "equity" | "revenue" | "expense",
): Promise<{ id: number; code: string }> {
  const code = uniqueCode()
  const res = await request.post("/api/accounts", {
    data: {
      code,
      name: `${code} ${accountType}`,
      account_type: accountType,
      parent_id: null,
      currency_code: null,
      is_postable: true,
      is_active: true,
    },
  })
  expect(res.ok(), `create account failed (${res.status()})`).toBeTruthy()
  return { id: ((await res.json()) as { id: number }).id, code }
}

/** Everything a sales-document test needs, created via the API: a customer
 *  (on a throwaway organization) wired to a fresh AR account, revenue and
 *  cash accounts to post against, and a one-day fiscal year + open period so
 *  documents dated on `day` can post. Every row carries the E2E- prefix. */
export interface SalesFixture {
  day: string
  orgName: string
  customerId: number
  arAccount: { id: number; code: string }
  revenueAccount: { id: number; code: string }
  cashAccount: { id: number; code: string }
}

export async function salesFixture(
  request: APIRequestContext,
): Promise<SalesFixture> {
  const day = uniqueDocDay()
  const orgName = uniqueOrgName()
  const [arAccount, revenueAccount, cashAccount, orgId] = await Promise.all([
    createAccount(request, "asset"),
    createAccount(request, "revenue"),
    createAccount(request, "asset"),
    createOrganization(request, orgName),
  ])
  const [customerId, fiscalYearId] = await Promise.all([
    createCustomer(request, orgId, {
      ar_account_id: arAccount.id,
      currency_code: "USD",
    }),
    createFiscalYear(request, `${E2E_PREFIX}FY ${day}`, day, day),
  ])
  await createPeriod(request, fiscalYearId, `${E2E_PREFIX}P ${day}`, day, day)
  return { day, orgName, customerId, arAccount, revenueAccount, cashAccount }
}

/** The AP mirror of SalesFixture: a supplier wired to a fresh AP account,
 *  an expense account to post against, a cash account to pay from, and the
 *  same one-day fiscal year + open period. */
export interface PurchaseFixture {
  day: string
  orgName: string
  supplierId: number
  apAccount: { id: number; code: string }
  expenseAccount: { id: number; code: string }
  cashAccount: { id: number; code: string }
}

export async function purchaseFixture(
  request: APIRequestContext,
): Promise<PurchaseFixture> {
  const day = uniqueDocDay()
  const orgName = uniqueOrgName()
  const [apAccount, expenseAccount, cashAccount, orgId] = await Promise.all([
    createAccount(request, "liability"),
    createAccount(request, "expense"),
    createAccount(request, "asset"),
    createOrganization(request, orgName),
  ])
  const [supplierId, fiscalYearId] = await Promise.all([
    createSupplier(request, orgId, {
      ap_account_id: apAccount.id,
      currency_code: "USD",
    }),
    createFiscalYear(request, `${E2E_PREFIX}FY ${day}`, day, day),
  ])
  await createPeriod(request, fiscalYearId, `${E2E_PREFIX}P ${day}`, day, day)
  return { day, orgName, supplierId, apAccount, expenseAccount, cashAccount }
}

/** POST an action endpoint (post/confirm/apply/…) and expect success. */
export async function apiPost(
  request: APIRequestContext,
  url: string,
): Promise<void> {
  const res = await request.post(url)
  expect(res.ok(), `POST ${url} failed (${res.status()})`).toBeTruthy()
}

/** Create a draft sales invoice with one free-form line and return its id. */
export async function createInvoiceDraft(
  request: APIRequestContext,
  fixture: SalesFixture,
  number: string,
  unitPrice = "100",
): Promise<number> {
  const res = await request.post("/api/sales-invoices", {
    data: {
      invoice_number: number,
      customer_id: fixture.customerId,
      invoice_date: fixture.day,
      due_date: null,
      currency_code: "USD",
      reference: null,
      memo: null,
      lines: [
        {
          product_id: null,
          description: "E2E line",
          quantity: "1",
          unit_price: unitPrice,
          revenue_account_id: fixture.revenueAccount.id,
          tax_code: null,
          tax_rate: "0",
        },
      ],
    },
  })
  expect(res.ok(), `create invoice failed (${res.status()})`).toBeTruthy()
  return ((await res.json()) as { id: number }).id
}

/** Create a draft customer payment and return its id. */
export async function createCustomerPaymentDraft(
  request: APIRequestContext,
  fixture: SalesFixture,
  amount: string,
): Promise<number> {
  const res = await request.post("/api/customer-payments", {
    data: {
      customer_id: fixture.customerId,
      payment_date: fixture.day,
      currency_code: "USD",
      amount,
      method: null,
      reference: null,
      deposit_account_id: fixture.cashAccount.id,
    },
  })
  expect(res.ok(), `create payment failed (${res.status()})`).toBeTruthy()
  return ((await res.json()) as { id: number }).id
}

/** Create a draft sales credit note with one free-form line, return its id. */
export async function createCreditNoteDraft(
  request: APIRequestContext,
  fixture: SalesFixture,
  number: string,
  unitPrice: string,
): Promise<number> {
  const res = await request.post("/api/sales-credit-notes", {
    data: {
      credit_note_number: number,
      customer_id: fixture.customerId,
      credit_note_date: fixture.day,
      currency_code: "USD",
      reference: null,
      memo: null,
      lines: [
        {
          product_id: null,
          description: "E2E credit line",
          quantity: "1",
          unit_price: unitPrice,
          revenue_account_id: fixture.revenueAccount.id,
          tax_code: null,
          tax_rate: "0",
        },
      ],
    },
  })
  expect(res.ok(), `create credit note failed (${res.status()})`).toBeTruthy()
  return ((await res.json()) as { id: number }).id
}

/** Create a draft sales order with one free-form line and return its id. */
export async function createSalesOrderDraft(
  request: APIRequestContext,
  fixture: SalesFixture,
  number: string,
  unitPrice = "80",
): Promise<number> {
  const res = await request.post("/api/sales-orders", {
    data: {
      order_number: number,
      customer_id: fixture.customerId,
      order_date: fixture.day,
      expected_ship_date: null,
      currency_code: "USD",
      reference: null,
      memo: null,
      lines: [
        {
          product_id: null,
          description: "E2E ordered line",
          quantity: "1",
          unit_price: unitPrice,
          revenue_account_id: fixture.revenueAccount.id,
          tax_code: null,
          tax_rate: "0",
        },
      ],
    },
  })
  expect(res.ok(), `create sales order failed (${res.status()})`).toBeTruthy()
  return ((await res.json()) as { id: number }).id
}

/** Create a draft purchase bill with one free-form line and return its id. */
export async function createBillDraft(
  request: APIRequestContext,
  fixture: PurchaseFixture,
  number: string,
  unitCost = "100",
): Promise<number> {
  const res = await request.post("/api/purchase-bills", {
    data: {
      bill_number: number,
      supplier_id: fixture.supplierId,
      bill_date: fixture.day,
      due_date: null,
      currency_code: "USD",
      reference: null,
      memo: null,
      lines: [
        {
          product_id: null,
          description: "E2E billed line",
          quantity: "1",
          unit_cost: unitCost,
          expense_account_id: fixture.expenseAccount.id,
          tax_code: null,
          tax_rate: "0",
        },
      ],
    },
  })
  expect(res.ok(), `create bill failed (${res.status()})`).toBeTruthy()
  return ((await res.json()) as { id: number }).id
}

/** Create a draft supplier payment and return its id. */
export async function createSupplierPaymentDraft(
  request: APIRequestContext,
  fixture: PurchaseFixture,
  amount: string,
): Promise<number> {
  const res = await request.post("/api/supplier-payments", {
    data: {
      supplier_id: fixture.supplierId,
      payment_date: fixture.day,
      currency_code: "USD",
      amount,
      method: null,
      reference: null,
      payment_account_id: fixture.cashAccount.id,
    },
  })
  expect(res.ok(), `create supplier payment failed (${res.status()})`).toBeTruthy()
  return ((await res.json()) as { id: number }).id
}

/** Create a draft purchase credit note with one free-form line, return its id. */
export async function createSupplierCreditDraft(
  request: APIRequestContext,
  fixture: PurchaseFixture,
  number: string,
  unitCost: string,
): Promise<number> {
  const res = await request.post("/api/purchase-credit-notes", {
    data: {
      credit_note_number: number,
      supplier_id: fixture.supplierId,
      credit_note_date: fixture.day,
      currency_code: "USD",
      reference: null,
      memo: null,
      lines: [
        {
          product_id: null,
          description: "E2E returned line",
          quantity: "1",
          unit_cost: unitCost,
          expense_account_id: fixture.expenseAccount.id,
          tax_code: null,
          tax_rate: "0",
        },
      ],
    },
  })
  expect(res.ok(), `create supplier credit failed (${res.status()})`).toBeTruthy()
  return ((await res.json()) as { id: number }).id
}

/** Create a draft purchase order with one free-form line and return its id. */
export async function createPurchaseOrderDraft(
  request: APIRequestContext,
  fixture: PurchaseFixture,
  number: string,
  unitCost = "80",
): Promise<number> {
  const res = await request.post("/api/purchase-orders", {
    data: {
      order_number: number,
      supplier_id: fixture.supplierId,
      order_date: fixture.day,
      expected_receipt_date: null,
      currency_code: "USD",
      reference: null,
      memo: null,
      lines: [
        {
          product_id: null,
          description: "E2E ordered line",
          quantity: "1",
          unit_cost: unitCost,
          expense_account_id: fixture.expenseAccount.id,
          tax_code: null,
          tax_rate: "0",
        },
      ],
    },
  })
  expect(res.ok(), `create purchase order failed (${res.status()})`).toBeTruthy()
  return ((await res.json()) as { id: number }).id
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
