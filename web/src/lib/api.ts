// Minimal typed client for the same-origin JSON API under /api. In dev the Vite
// dev server proxies /api to the Go backend; in production the Go backend serves
// both the SPA and the API, so the path is identical in both environments.

const BASE = "/api"

/** Error carrying the HTTP status and the server's error message, if any. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message)
    this.name = "ApiError"
  }
}

/** Fired on any 401 response so the app can drop back to the login screen when
 *  the session expires mid-use. */
export const UNAUTHORIZED_EVENT = "tadmor:unauthorized"

// The backend reports errors as { "error": "..." }; fall back to the status.
async function failure(res: Response): Promise<ApiError> {
  if (res.status === 401) {
    window.dispatchEvent(new Event(UNAUTHORIZED_EVENT))
  }
  let message = `request failed (${res.status})`
  try {
    const body = (await res.json()) as { error?: string }
    if (body?.error) message = body.error
  } catch {
    // Non-JSON error body; keep the status-based message.
  }
  return new ApiError(res.status, message)
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { Accept: "application/json" },
  })
  if (!res.ok) throw await failure(res)
  return (await res.json()) as T
}

// Send a body-bearing request (POST/PUT). Used for writes; the backend's PUT
// updates return 204 with no body, so nothing is parsed on success.
async function send(method: string, path: string, body: unknown): Promise<void> {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw await failure(res)
}

// DELETE a resource. Draft-document deletes return 200 with { "status": "ok" };
// nothing is parsed on success.
async function del(path: string): Promise<void> {
  const res = await fetch(`${BASE}${path}`, {
    method: "DELETE",
    headers: { Accept: "application/json" },
  })
  if (!res.ok) throw await failure(res)
}

// POST a body and parse the JSON response (creates return 201 with { "id": ... }).
async function post<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw await failure(res)
  return (await res.json()) as T
}

// Authentication. The session rides in an HttpOnly cookie the browser attaches
// automatically (fetch sends cookies on same-origin requests by default), so
// no token handling happens in JS.

/** The logged-in user, mirroring auth.User on the backend. */
export interface User {
  id: number
  email: string
  full_name: string
  is_admin: boolean
}

export function login(email: string, password: string): Promise<User> {
  return post<User>("/auth/login", { email, password })
}

export function logout(): Promise<void> {
  return send("POST", "/auth/logout", {})
}

/** The session probe the app runs on load; 401 means "show the login screen". */
export function me(): Promise<User> {
  return get<User>("/auth/me")
}

/** A user row as the admin screen sees it, mirroring auth.UserRecord (the
 *  password hash never leaves the backend). */
export interface UserRecord {
  id: number
  email: string
  full_name: string
  is_active: boolean
  is_admin: boolean
}

export function listUsers(): Promise<UserRecord[]> {
  return get<UserRecord[]>("/users")
}

export function getUser(id: number): Promise<UserRecord> {
  return get<UserRecord>(`/users/${id}`)
}

export function createUser(input: {
  email: string
  full_name: string
  password: string
  is_admin: boolean
}): Promise<{ id: number }> {
  return post<{ id: number }>("/users", input)
}

export function updateUser(
  id: number,
  input: {
    email: string
    full_name: string
    is_active: boolean
    is_admin: boolean
  },
): Promise<void> {
  return send("PUT", `/users/${id}`, input)
}

/** Set a new password for the user, revoking all of their sessions. */
export function setUserPassword(id: number, password: string): Promise<void> {
  return send("POST", `/users/${id}/password`, { password })
}

/** A general-ledger account, mirroring master.Account on the backend.
 *  is_cash marks cash/cash-equivalent accounts (assets only) — the cash-flow
 *  statement explains the change in their combined balance. cash_flow_activity
 *  classifies a non-cash balance-sheet account's movements for that statement;
 *  it is ignored for revenue/expense accounts. */
export interface Account {
  id: number
  code: string
  name: string
  account_type: string
  parent_id: number | null
  currency_code: string | null
  is_postable: boolean
  is_active: boolean
  is_cash: boolean
  cash_flow_activity: string
}

/** The writable fields of an account (Account without its id), mirroring
 *  master.AccountInput. PUT is a full replace. */
export interface AccountInput {
  code: string
  name: string
  account_type: string
  parent_id: number | null
  currency_code: string | null
  is_postable: boolean
  is_active: boolean
  is_cash: boolean
  cash_flow_activity: string
}

// The fixed account_types lookup (db/migrations/000004). A closed set with no
// list endpoint, so it's mirrored here rather than fetched.
export const ACCOUNT_TYPES = [
  { code: "asset", name: "Asset" },
  { code: "liability", name: "Liability" },
  { code: "equity", name: "Equity" },
  { code: "revenue", name: "Revenue" },
  { code: "expense", name: "Expense" },
] as const

// The cash-flow statement's activity sections (db/migrations/000015). A
// closed set with no list endpoint, so it's mirrored here rather than fetched.
export const CASH_FLOW_ACTIVITIES = [
  { code: "operating", name: "Operating" },
  { code: "investing", name: "Investing" },
  { code: "financing", name: "Financing" },
] as const

export function listAccounts(): Promise<Account[]> {
  return get<Account[]>("/accounts")
}

export function getAccount(id: number): Promise<Account> {
  return get<Account>(`/accounts/${id}`)
}

export function createAccount(input: AccountInput): Promise<{ id: number }> {
  return post<{ id: number }>("/accounts", input)
}

export function updateAccount(id: number, input: AccountInput): Promise<void> {
  return send("PUT", `/accounts/${id}`, input)
}

/** An organization (the party a customer/supplier role attaches to). */
export interface Organization {
  id: number
  name: string
  legal_name: string | null
  tax_id: string | null
  country_code: string | null
  default_currency: string | null
  is_self: boolean
}

/** The writable fields of an organization (Organization without its id),
 *  mirroring master.OrganizationInput. PUT is a full replace. */
export interface OrganizationInput {
  name: string
  legal_name: string | null
  tax_id: string | null
  country_code: string | null
  default_currency: string | null
  is_self: boolean
}

export function listOrganizations(): Promise<Organization[]> {
  return get<Organization[]>("/organizations")
}

export function getOrganization(id: number): Promise<Organization> {
  return get<Organization>(`/organizations/${id}`)
}

export function createOrganization(
  input: OrganizationInput,
): Promise<{ id: number }> {
  return post<{ id: number }>("/organizations", input)
}

export function updateOrganization(
  id: number,
  input: OrganizationInput,
): Promise<void> {
  return send("PUT", `/organizations/${id}`, input)
}

/** A customer: a role on an organization. The display name lives on the
 *  organization (join via organization_id), mirroring master.Customer. */
export interface Customer {
  id: number
  organization_id: number
  customer_number: string | null
  ar_account_id: number | null
  payment_terms_code: string | null
  currency_code: string | null
  tax_code: string | null
  credit_limit: string | null
  is_active: boolean
}

/** The writable fields of a customer (Customer without its id), mirroring
 *  master.CustomerInput. PUT is a full replace. */
export interface CustomerInput {
  organization_id: number
  customer_number: string | null
  ar_account_id: number | null
  payment_terms_code: string | null
  currency_code: string | null
  tax_code: string | null
  credit_limit: string | null
  is_active: boolean
}

export function listCustomers(): Promise<Customer[]> {
  return get<Customer[]>("/customers")
}

export function getCustomer(id: number): Promise<Customer> {
  return get<Customer>(`/customers/${id}`)
}

export function createCustomer(input: CustomerInput): Promise<{ id: number }> {
  return post<{ id: number }>("/customers", input)
}

export function updateCustomer(id: number, input: CustomerInput): Promise<void> {
  return send("PUT", `/customers/${id}`, input)
}

/** A warehouse, mirroring master.Warehouse. */
export interface Warehouse {
  id: number
  code: string
  name: string
  address_id: number | null
  is_active: boolean
}

/** The writable fields of a warehouse (Warehouse without its id), mirroring
 *  master.WarehouseInput. PUT is a full replace. */
export interface WarehouseInput {
  code: string
  name: string
  address_id: number | null
  is_active: boolean
}

export function listWarehouses(): Promise<Warehouse[]> {
  return get<Warehouse[]>("/warehouses")
}

export function getWarehouse(id: number): Promise<Warehouse> {
  return get<Warehouse>(`/warehouses/${id}`)
}

export function createWarehouse(
  input: WarehouseInput,
): Promise<{ id: number }> {
  return post<{ id: number }>("/warehouses", input)
}

export function updateWarehouse(
  id: number,
  input: WarehouseInput,
): Promise<void> {
  return send("PUT", `/warehouses/${id}`, input)
}

/** A tax code (natural key: code), mirroring master.TaxCode. rate is a percent
 *  as an exact decimal string, e.g. "8.2500". */
export interface TaxCode {
  code: string
  name: string
  rate: string
  tax_account_id: number | null
  is_active: boolean
}

/** The writable fields of a tax code, mirroring master.TaxCodeInput. The code
 *  is the identity (natural key): sent on create, fixed thereafter. */
export interface TaxCodeInput {
  code: string
  name: string
  rate: string
  tax_account_id: number | null
  is_active: boolean
}

export function listTaxCodes(): Promise<TaxCode[]> {
  return get<TaxCode[]>("/tax-codes")
}

export function getTaxCode(code: string): Promise<TaxCode> {
  return get<TaxCode>(`/tax-codes/${encodeURIComponent(code)}`)
}

export function createTaxCode(input: TaxCodeInput): Promise<{ code: string }> {
  return post<{ code: string }>("/tax-codes", input)
}

export function updateTaxCode(
  code: string,
  input: TaxCodeInput,
): Promise<void> {
  return send("PUT", `/tax-codes/${encodeURIComponent(code)}`, input)
}

/** A payment term (natural key: code), mirroring master.PaymentTerm. The list
 *  comes back ordered by due_days, shortest terms first. */
export interface PaymentTerm {
  code: string
  name: string
  due_days: number
}

/** The writable fields of a payment term, mirroring master.PaymentTermInput.
 *  The code is the identity (natural key): sent on create, fixed thereafter. */
export interface PaymentTermInput {
  code: string
  name: string
  due_days: number
}

export function listPaymentTerms(): Promise<PaymentTerm[]> {
  return get<PaymentTerm[]>("/payment-terms")
}

export function getPaymentTerm(code: string): Promise<PaymentTerm> {
  return get<PaymentTerm>(`/payment-terms/${encodeURIComponent(code)}`)
}

export function createPaymentTerm(
  input: PaymentTermInput,
): Promise<{ code: string }> {
  return post<{ code: string }>("/payment-terms", input)
}

export function updatePaymentTerm(
  code: string,
  input: PaymentTermInput,
): Promise<void> {
  return send("PUT", `/payment-terms/${encodeURIComponent(code)}`, input)
}

/** The ledger's base (functional) currency and the account that absorbs
 *  realized exchange gains/losses, mirroring master.Settings. base_currency is
 *  frozen by the database once journal entries exist. */
export interface Settings {
  base_currency: string
  fx_gain_loss_account_id: number | null
}

export function getSettings(): Promise<Settings> {
  return get<Settings>("/settings")
}

/** Replace the ledger settings. Admin-only; the FX account must be postable
 *  and active, and the base currency can only change on an empty ledger. */
export function updateSettings(input: Settings): Promise<void> {
  return send("PUT", "/settings", input)
}

/** A manually maintained exchange rate (natural key: currency_code +
 *  rate_date), mirroring master.ExchangeRate. rate is how many base-currency
 *  units one unit of currency_code buys; posting uses the latest rate on or
 *  before a document's date. */
export interface ExchangeRate {
  currency_code: string
  rate_date: string
  rate: string
}

export interface ExchangeRateInput {
  currency_code: string
  rate_date: string
  rate: string
}

export function listExchangeRates(): Promise<ExchangeRate[]> {
  return get<ExchangeRate[]>("/exchange-rates")
}

export function createExchangeRate(
  input: ExchangeRateInput,
): Promise<{ currency_code: string; rate_date: string }> {
  return post<{ currency_code: string; rate_date: string }>(
    "/exchange-rates",
    input,
  )
}

export function updateExchangeRate(
  currency: string,
  date: string,
  input: ExchangeRateInput,
): Promise<void> {
  return send(
    "PUT",
    `/exchange-rates/${encodeURIComponent(currency)}/${encodeURIComponent(date)}`,
    input,
  )
}

export function deleteExchangeRate(
  currency: string,
  date: string,
): Promise<void> {
  return del(
    `/exchange-rates/${encodeURIComponent(currency)}/${encodeURIComponent(date)}`,
  )
}

/** A fiscal year, mirroring master.FiscalYear. Dates are YYYY-MM-DD strings. */
export interface FiscalYear {
  id: number
  name: string
  start_date: string
  end_date: string
  status: string
}

/** The writable fields of a fiscal year (FiscalYear without its id or status),
 *  mirroring master.FiscalYearInput. PUT is a full replace. Status is owned by
 *  the year-end close workflow: closeFiscalYear / reopenFiscalYear. */
export interface FiscalYearInput {
  name: string
  start_date: string
  end_date: string
}

export function listFiscalYears(): Promise<FiscalYear[]> {
  return get<FiscalYear[]>("/fiscal-years")
}

export function getFiscalYear(id: number): Promise<FiscalYear> {
  return get<FiscalYear>(`/fiscal-years/${id}`)
}

export function createFiscalYear(
  input: FiscalYearInput,
): Promise<{ id: number }> {
  return post<{ id: number }>("/fiscal-years", input)
}

export function updateFiscalYear(
  id: number,
  input: FiscalYearInput,
): Promise<void> {
  return send("PUT", `/fiscal-years/${id}`, input)
}

/** What a year-end close produced: the closing journal entry (null when the
 *  year had no revenue or expense balances to sweep) and the auto-created next
 *  fiscal year (null when one already existed). */
export interface CloseFiscalYearResult {
  closing_entry_id: number | null
  next_fiscal_year_id: number | null
}

/** Close a fiscal year (admin-only): posts a closing entry sweeping revenue
 *  and expenses into the given retained-earnings account, closes all the
 *  year's periods and the year itself, and rolls the calendar forward. */
export function closeFiscalYear(
  id: number,
  retainedEarningsAccountId: number,
): Promise<CloseFiscalYearResult> {
  return post<CloseFiscalYearResult>(`/fiscal-years/${id}/close`, {
    retained_earnings_account_id: retainedEarningsAccountId,
  })
}

/** Reopen a closed fiscal year (admin-only): reverses its closing entry (id
 *  returned; null when there was none) and reopens the year. Only the period
 *  that held the closing entry is reopened — others stay closed. */
export function reopenFiscalYear(
  id: number,
): Promise<{ reversal_entry_id: number | null }> {
  return post<{ reversal_entry_id: number | null }>(
    `/fiscal-years/${id}/reopen`,
    {},
  )
}

/** An accounting period (the unit that gates posting: documents can only post
 *  into an open period), mirroring master.AccountingPeriod. */
export interface AccountingPeriod {
  id: number
  fiscal_year_id: number
  name: string
  start_date: string
  end_date: string
  status: string
}

/** The writable fields of a period (AccountingPeriod without its id), mirroring
 *  master.AccountingPeriodInput. PUT is a full replace; status is open|closed
 *  and is only honored on update (creates always start open). */
export interface AccountingPeriodInput {
  fiscal_year_id: number
  name: string
  start_date: string
  end_date: string
  status: string
}

export function listAccountingPeriods(): Promise<AccountingPeriod[]> {
  return get<AccountingPeriod[]>("/accounting-periods")
}

export function getAccountingPeriod(id: number): Promise<AccountingPeriod> {
  return get<AccountingPeriod>(`/accounting-periods/${id}`)
}

export function createAccountingPeriod(
  input: AccountingPeriodInput,
): Promise<{ id: number }> {
  return post<{ id: number }>("/accounting-periods", input)
}

export function updateAccountingPeriod(
  id: number,
  input: AccountingPeriodInput,
): Promise<void> {
  return send("PUT", `/accounting-periods/${id}`, input)
}

/** A supplier: a role on an organization, like Customer. The display name lives
 *  on the organization (join via organization_id), mirroring master.Supplier. */
export interface Supplier {
  id: number
  organization_id: number
  supplier_number: string | null
  ap_account_id: number | null
  payment_terms_code: string | null
  currency_code: string | null
  tax_code: string | null
  is_active: boolean
}

/** The writable fields of a supplier (Supplier without its id), mirroring
 *  master.SupplierInput. PUT is a full replace. */
export interface SupplierInput {
  organization_id: number
  supplier_number: string | null
  ap_account_id: number | null
  payment_terms_code: string | null
  currency_code: string | null
  tax_code: string | null
  is_active: boolean
}

export function listSuppliers(): Promise<Supplier[]> {
  return get<Supplier[]>("/suppliers")
}

export function getSupplier(id: number): Promise<Supplier> {
  return get<Supplier>(`/suppliers/${id}`)
}

export function createSupplier(input: SupplierInput): Promise<{ id: number }> {
  return post<{ id: number }>("/suppliers", input)
}

export function updateSupplier(id: number, input: SupplierInput): Promise<void> {
  return send("PUT", `/suppliers/${id}`, input)
}

/** A catalog product/service, mirroring master.Product. Standalone (its own SKU
 *  and name), so no organization join is needed. */
export interface Product {
  id: number
  sku: string
  name: string
  description: string | null
  unit_price: string
  currency_code: string | null
  revenue_account_id: number | null
  tax_code: string | null
  track_inventory: boolean
  inventory_account_id: number | null
  cogs_account_id: number | null
  is_active: boolean
}

/** The writable fields of a product (Product without its id), mirroring
 *  master.ProductInput. PUT is a full replace. */
export interface ProductInput {
  sku: string
  name: string
  description: string | null
  unit_price: string
  currency_code: string | null
  revenue_account_id: number | null
  tax_code: string | null
  track_inventory: boolean
  inventory_account_id: number | null
  cogs_account_id: number | null
  is_active: boolean
}

export function listProducts(): Promise<Product[]> {
  return get<Product[]>("/products")
}

export function getProduct(id: number): Promise<Product> {
  return get<Product>(`/products/${id}`)
}

export function createProduct(input: ProductInput): Promise<{ id: number }> {
  return post<{ id: number }>("/products", input)
}

export function updateProduct(id: number, input: ProductInput): Promise<void> {
  return send("PUT", `/products/${id}`, input)
}

// Subledger documents. Creation makes a draft; posting to the GL and
// unposting (reversal back to draft) are separate lifecycle calls.

/** Email a printable document to its counterparty as a PDF attachment.
 *  `collection` is the API path segment shared with the PDF endpoint (e.g.
 *  "sales-invoices", "purchase-orders"); `to` lists the recipient addresses.
 *  Until organizations carry an email address the caller supplies them.
 *  Rejects with a 501 ApiError ("email sending is not configured") when SMTP is
 *  off, so the button stays inert on the demo. */
export function emailDocument(
  collection: string,
  id: number,
  to: string[],
): Promise<{ status: string }> {
  return post<{ status: string }>(`/${collection}/${id}/email`, { to })
}

/** An invoice's or bill's balance view, mirroring reporting.DocumentBalance.
 *  Monetary values are exact decimal strings. */
export interface DocumentBalance {
  id: number
  number: string
  party_id: number
  currency_code: string
  date: string
  due_date: string | null
  status: string
  total: string
  amount_applied: string
  balance: string
  payment_status: string
  /** Set once the document has been posted. */
  journal_entry_id: number | null
  reference: string | null
  memo: string | null
}

/** One invoice line with its database-computed money, mirroring
 *  reporting.SalesInvoiceLine. */
export interface SalesInvoiceLine {
  line_no: number
  product_id: number | null
  description: string
  quantity: string
  unit_price: string
  tax_code: string | null
  tax_rate: string
  line_subtotal: string
  tax_amount: string
  line_total: string
  revenue_account_id: number | null
  /** Set when the line was produced by order fulfilment; such documents
   *  cannot be edited. Always null on credit-note lines. */
  order_line_id: number | null
}

/** Input for one draft invoice line, mirroring documents.SalesInvoiceLineInput.
 *  Empty quantity/unit_price/tax_rate default server-side to 1/0/0. */
export interface SalesInvoiceLineInput {
  product_id: number | null
  description: string
  quantity: string
  unit_price: string
  revenue_account_id: number | null
  tax_code: string | null
  tax_rate: string
}

/** Input for a draft invoice, mirroring documents.SalesInvoiceInput. */
export interface SalesInvoiceInput {
  invoice_number: string
  customer_id: number
  invoice_date: string
  due_date: string | null
  currency_code: string
  reference: string | null
  memo: string | null
  lines: SalesInvoiceLineInput[]
}

export function listSalesInvoices(): Promise<DocumentBalance[]> {
  return get<DocumentBalance[]>("/sales-invoices")
}

export function getSalesInvoice(id: number): Promise<DocumentBalance> {
  return get<DocumentBalance>(`/sales-invoices/${id}`)
}

export function getSalesInvoiceLines(id: number): Promise<SalesInvoiceLine[]> {
  return get<SalesInvoiceLine[]>(`/sales-invoices/${id}/lines`)
}

export function createSalesInvoice(
  input: SalesInvoiceInput,
): Promise<{ id: number }> {
  return post<{ id: number }>("/sales-invoices", input)
}

/** Rewrite a draft invoice's header and full line set. */
export function updateSalesInvoice(
  id: number,
  input: SalesInvoiceInput,
): Promise<void> {
  return send("PUT", `/sales-invoices/${id}`, input)
}

/** Delete a draft invoice. */
export function deleteSalesInvoice(id: number): Promise<void> {
  return del(`/sales-invoices/${id}`)
}

export function postSalesInvoice(
  id: number,
): Promise<{ journal_entry_id: number }> {
  return post<{ journal_entry_id: number }>(`/sales-invoices/${id}/post`, {})
}

export function unpostSalesInvoice(
  id: number,
): Promise<{ reversal_entry_id: number }> {
  return post<{ reversal_entry_id: number }>(`/sales-invoices/${id}/unpost`, {})
}

/** One bill line with its database-computed money, mirroring
 *  reporting.PurchaseBillLine. */
export interface PurchaseBillLine {
  line_no: number
  product_id: number | null
  description: string
  quantity: string
  unit_cost: string
  tax_code: string | null
  tax_rate: string
  line_subtotal: string
  tax_amount: string
  line_total: string
  expense_account_id: number | null
  /** Set when the line was produced by order fulfilment; such documents
   *  cannot be edited. Always null on credit-note lines. */
  order_line_id: number | null
}

/** Input for one draft bill line, mirroring documents.PurchaseBillLineInput.
 *  Empty quantity/unit_cost/tax_rate default server-side to 1/0/0. */
export interface PurchaseBillLineInput {
  product_id: number | null
  description: string
  quantity: string
  unit_cost: string
  expense_account_id: number | null
  tax_code: string | null
  tax_rate: string
}

/** Input for a draft bill, mirroring documents.PurchaseBillInput. */
export interface PurchaseBillInput {
  bill_number: string
  supplier_id: number
  bill_date: string
  due_date: string | null
  currency_code: string
  reference: string | null
  memo: string | null
  lines: PurchaseBillLineInput[]
}

export function listPurchaseBills(): Promise<DocumentBalance[]> {
  return get<DocumentBalance[]>("/purchase-bills")
}

export function getPurchaseBill(id: number): Promise<DocumentBalance> {
  return get<DocumentBalance>(`/purchase-bills/${id}`)
}

export function getPurchaseBillLines(id: number): Promise<PurchaseBillLine[]> {
  return get<PurchaseBillLine[]>(`/purchase-bills/${id}/lines`)
}

export function createPurchaseBill(
  input: PurchaseBillInput,
): Promise<{ id: number }> {
  return post<{ id: number }>("/purchase-bills", input)
}

/** Rewrite a draft bill's header and full line set. */
export function updatePurchaseBill(
  id: number,
  input: PurchaseBillInput,
): Promise<void> {
  return send("PUT", `/purchase-bills/${id}`, input)
}

/** Delete a draft bill. */
export function deletePurchaseBill(id: number): Promise<void> {
  return del(`/purchase-bills/${id}`)
}

export function postPurchaseBill(
  id: number,
): Promise<{ journal_entry_id: number }> {
  return post<{ journal_entry_id: number }>(`/purchase-bills/${id}/post`, {})
}

export function unpostPurchaseBill(
  id: number,
): Promise<{ reversal_entry_id: number }> {
  return post<{ reversal_entry_id: number }>(`/purchase-bills/${id}/unpost`, {})
}

// Credit notes reuse the invoice/bill shapes: DocumentBalance for the balance
// views (due_date is always null — credits do not age — and payment_status
// carries the application status: open/partial/applied/void), and the
// invoice/bill line types for lines.

/** Input for a draft sales credit note, mirroring
 *  documents.SalesCreditNoteInput. Lines reuse the invoice line shape. */
export interface SalesCreditNoteInput {
  credit_note_number: string
  customer_id: number
  credit_note_date: string
  currency_code: string
  reference: string | null
  memo: string | null
  lines: SalesInvoiceLineInput[]
}

/** Input for a draft purchase credit note, mirroring
 *  documents.PurchaseCreditNoteInput. Lines reuse the bill line shape. */
export interface PurchaseCreditNoteInput {
  credit_note_number: string
  supplier_id: number
  credit_note_date: string
  currency_code: string
  reference: string | null
  memo: string | null
  lines: PurchaseBillLineInput[]
}

export function listSalesCreditNotes(): Promise<DocumentBalance[]> {
  return get<DocumentBalance[]>("/sales-credit-notes")
}

export function getSalesCreditNote(id: number): Promise<DocumentBalance> {
  return get<DocumentBalance>(`/sales-credit-notes/${id}`)
}

export function getSalesCreditNoteLines(
  id: number,
): Promise<SalesInvoiceLine[]> {
  return get<SalesInvoiceLine[]>(`/sales-credit-notes/${id}/lines`)
}

export function getSalesCreditNoteApplications(
  id: number,
): Promise<PaymentApplication[]> {
  return get<PaymentApplication[]>(`/sales-credit-notes/${id}/applications`)
}

export function createSalesCreditNote(
  input: SalesCreditNoteInput,
): Promise<{ id: number }> {
  return post<{ id: number }>("/sales-credit-notes", input)
}

/** Rewrite a draft credit note's header and full line set. */
export function updateSalesCreditNote(
  id: number,
  input: SalesCreditNoteInput,
): Promise<void> {
  return send("PUT", `/sales-credit-notes/${id}`, input)
}

/** Delete a draft credit note. */
export function deleteSalesCreditNote(id: number): Promise<void> {
  return del(`/sales-credit-notes/${id}`)
}

export function postSalesCreditNote(
  id: number,
): Promise<{ journal_entry_id: number }> {
  return post<{ journal_entry_id: number }>(
    `/sales-credit-notes/${id}/post`,
    {},
  )
}

export function applySalesCreditNote(
  id: number,
): Promise<{ applications: { document_id: number; amount: string }[] }> {
  return post(`/sales-credit-notes/${id}/apply`, {})
}

export function unpostSalesCreditNote(
  id: number,
): Promise<{ reversal_entry_id: number }> {
  return post<{ reversal_entry_id: number }>(
    `/sales-credit-notes/${id}/unpost`,
    {},
  )
}

export function listPurchaseCreditNotes(): Promise<DocumentBalance[]> {
  return get<DocumentBalance[]>("/purchase-credit-notes")
}

export function getPurchaseCreditNote(id: number): Promise<DocumentBalance> {
  return get<DocumentBalance>(`/purchase-credit-notes/${id}`)
}

export function getPurchaseCreditNoteLines(
  id: number,
): Promise<PurchaseBillLine[]> {
  return get<PurchaseBillLine[]>(`/purchase-credit-notes/${id}/lines`)
}

export function getPurchaseCreditNoteApplications(
  id: number,
): Promise<PaymentApplication[]> {
  return get<PaymentApplication[]>(`/purchase-credit-notes/${id}/applications`)
}

export function createPurchaseCreditNote(
  input: PurchaseCreditNoteInput,
): Promise<{ id: number }> {
  return post<{ id: number }>("/purchase-credit-notes", input)
}

/** Rewrite a draft credit note's header and full line set. */
export function updatePurchaseCreditNote(
  id: number,
  input: PurchaseCreditNoteInput,
): Promise<void> {
  return send("PUT", `/purchase-credit-notes/${id}`, input)
}

/** Delete a draft credit note. */
export function deletePurchaseCreditNote(id: number): Promise<void> {
  return del(`/purchase-credit-notes/${id}`)
}

export function postPurchaseCreditNote(
  id: number,
): Promise<{ journal_entry_id: number }> {
  return post<{ journal_entry_id: number }>(
    `/purchase-credit-notes/${id}/post`,
    {},
  )
}

export function applyPurchaseCreditNote(
  id: number,
): Promise<{ applications: { document_id: number; amount: string }[] }> {
  return post(`/purchase-credit-notes/${id}/apply`, {})
}

export function unpostPurchaseCreditNote(
  id: number,
): Promise<{ reversal_entry_id: number }> {
  return post<{ reversal_entry_id: number }>(
    `/purchase-credit-notes/${id}/unpost`,
    {},
  )
}

/** A customer or supplier payment with its applied/unapplied split, mirroring
 *  reporting.Payment. */
export interface Payment {
  id: number
  party_id: number
  date: string
  currency_code: string
  amount: string
  method: string | null
  reference: string | null
  status: string
  amount_applied: string
  unapplied: string
  /** Set once the payment has been posted. */
  journal_entry_id: number | null
  /** The cash-side account: deposit_account_id for customer payments,
   *  payment_account_id for supplier payments. */
  account_id: number | null
}

/** One allocation of a payment to an invoice or bill, mirroring
 *  reporting.PaymentApplication. */
export interface PaymentApplication {
  document_id: number
  document_number: string
  amount_applied: string
}

/** Input for a draft customer payment, mirroring documents.CustomerPaymentInput. */
export interface CustomerPaymentInput {
  customer_id: number
  payment_date: string
  currency_code: string
  amount: string
  method: string | null
  reference: string | null
  deposit_account_id: number | null
}

/** Input for a draft supplier payment, mirroring documents.SupplierPaymentInput. */
export interface SupplierPaymentInput {
  supplier_id: number
  payment_date: string
  currency_code: string
  amount: string
  method: string | null
  reference: string | null
  payment_account_id: number | null
}

// The payment-method CHECK constraint's closed set (db/migrations/000005),
// mirrored here rather than fetched.
export const PAYMENT_METHODS = [
  "cash",
  "check",
  "card",
  "transfer",
  "other",
] as const

export function listCustomerPayments(): Promise<Payment[]> {
  return get<Payment[]>("/customer-payments")
}

export function getCustomerPayment(id: number): Promise<Payment> {
  return get<Payment>(`/customer-payments/${id}`)
}

export function getCustomerPaymentApplications(
  id: number,
): Promise<PaymentApplication[]> {
  return get<PaymentApplication[]>(`/customer-payments/${id}/applications`)
}

export function createCustomerPayment(
  input: CustomerPaymentInput,
): Promise<{ id: number }> {
  return post<{ id: number }>("/customer-payments", input)
}

/** Rewrite a draft customer payment. */
export function updateCustomerPayment(
  id: number,
  input: CustomerPaymentInput,
): Promise<void> {
  return send("PUT", `/customer-payments/${id}`, input)
}

/** Delete a draft customer payment. */
export function deleteCustomerPayment(id: number): Promise<void> {
  return del(`/customer-payments/${id}`)
}

export function postCustomerPayment(
  id: number,
): Promise<{ journal_entry_id: number }> {
  return post<{ journal_entry_id: number }>(`/customer-payments/${id}/post`, {})
}

export function unpostCustomerPayment(
  id: number,
): Promise<{ reversal_entry_id: number }> {
  return post<{ reversal_entry_id: number }>(
    `/customer-payments/${id}/unpost`,
    {},
  )
}

export function applyCustomerPayment(
  id: number,
): Promise<{ applications: { document_id: number; amount: string }[] }> {
  return post(`/customer-payments/${id}/apply`, {})
}

export function listSupplierPayments(): Promise<Payment[]> {
  return get<Payment[]>("/supplier-payments")
}

export function getSupplierPayment(id: number): Promise<Payment> {
  return get<Payment>(`/supplier-payments/${id}`)
}

export function getSupplierPaymentApplications(
  id: number,
): Promise<PaymentApplication[]> {
  return get<PaymentApplication[]>(`/supplier-payments/${id}/applications`)
}

export function createSupplierPayment(
  input: SupplierPaymentInput,
): Promise<{ id: number }> {
  return post<{ id: number }>("/supplier-payments", input)
}

/** Rewrite a draft supplier payment. */
export function updateSupplierPayment(
  id: number,
  input: SupplierPaymentInput,
): Promise<void> {
  return send("PUT", `/supplier-payments/${id}`, input)
}

/** Delete a draft supplier payment. */
export function deleteSupplierPayment(id: number): Promise<void> {
  return del(`/supplier-payments/${id}`)
}

export function postSupplierPayment(
  id: number,
): Promise<{ journal_entry_id: number }> {
  return post<{ journal_entry_id: number }>(`/supplier-payments/${id}/post`, {})
}

export function unpostSupplierPayment(
  id: number,
): Promise<{ reversal_entry_id: number }> {
  return post<{ reversal_entry_id: number }>(
    `/supplier-payments/${id}/unpost`,
    {},
  )
}

export function applySupplierPayment(
  id: number,
): Promise<{ applications: { document_id: number; amount: string }[] }> {
  return post(`/supplier-payments/${id}/apply`, {})
}

/** One inventory movement, mirroring reporting.StockMovement. There is no
 *  status column: a movement is posted iff journal_entry_id is set, and only
 *  receipts and issues ever post. */
export interface StockMovement {
  id: number
  product_id: number
  warehouse_id: number
  date: string
  movement_type: string
  quantity: string
  unit_cost: string
  total_cost: string
  reference: string | null
  notes: string | null
  journal_entry_id: number | null
  /** Set when the movement was produced by order fulfilment; such movements
   *  cannot be edited (though they may be deleted while unposted). */
  source_type: string | null
}

/** Input for a stock movement, mirroring documents.StockMovementInput.
 *  Quantity is signed: the CHECK constraint requires receipt/transfer_in
 *  positive and issue/transfer_out negative; adjustments go either way. */
export interface StockMovementInput {
  product_id: number
  warehouse_id: number
  movement_type: string
  movement_date: string | null
  quantity: string
  unit_cost: string
  reference: string | null
  notes: string | null
}

// The movement-type CHECK constraint's closed set (db/migrations/000007),
// mirrored here rather than fetched. Only receipt and issue post to the GL.
export const MOVEMENT_TYPES = [
  "receipt",
  "issue",
  "adjustment",
  "transfer_in",
  "transfer_out",
] as const

export function listStockMovements(): Promise<StockMovement[]> {
  return get<StockMovement[]>("/stock-movements")
}

export function getStockMovement(id: number): Promise<StockMovement> {
  return get<StockMovement>(`/stock-movements/${id}`)
}

export function createStockMovement(
  input: StockMovementInput,
): Promise<{ id: number }> {
  return post<{ id: number }>("/stock-movements", input)
}

/** Rewrite an unposted, non-fulfilment stock movement. */
export function updateStockMovement(
  id: number,
  input: StockMovementInput,
): Promise<void> {
  return send("PUT", `/stock-movements/${id}`, input)
}

/** Delete an unposted stock movement. */
export function deleteStockMovement(id: number): Promise<void> {
  return del(`/stock-movements/${id}`)
}

/** Post a movement to the GL. Stock movements carry no currency of their own
 *  and post in the base currency; credit_account_id is the clearing account a
 *  receipt credits (ignored for issues). */
export function postStockMovement(
  id: number,
  creditAccountId: number | null,
): Promise<{ journal_entry_id: number }> {
  return post<{ journal_entry_id: number }>(`/stock-movements/${id}/post`, {
    credit_account_id: creditAccountId ?? 0,
  })
}

export function unpostStockMovement(
  id: number,
): Promise<{ reversal_entry_id: number }> {
  return post<{ reversal_entry_id: number }>(
    `/stock-movements/${id}/unpost`,
    {},
  )
}

// Reporting endpoints are read-only views over posted journal entries. All
// monetary values are exact decimal strings (see reporting.go); render them
// with the helpers in @/lib/amount, never through Number.

/** One account's posted totals in the trial balance, mirroring
 *  reporting.TrialBalanceRow. */
export interface TrialBalanceRow {
  account_id: number
  code: string
  name: string
  account_type: string
  total_debit: string
  total_credit: string
  balance: string
}

export function getTrialBalance(): Promise<TrialBalanceRow[]> {
  return get<TrialBalanceRow[]>("/trial-balance")
}

/** One account's amount on a financial statement, mirroring
 *  reporting.AccountActivityRow. Amounts carry the account's natural sign:
 *  debit-positive for assets and expenses, credit-positive for liabilities,
 *  equity, and revenue. Accounts without activity in range are omitted. */
export interface AccountActivityRow {
  account_id: number
  code: string
  name: string
  account_type: string
  amount: string
}

/** The balance sheet, mirroring reporting.BalanceSheet. current_earnings is
 *  credit-positive net income to date — the figure that balances the sheet
 *  until a year-end close moves it into retained earnings. */
export interface BalanceSheet {
  rows: AccountActivityRow[]
  current_earnings: string
}

/** Revenue and expense activity over the inclusive [from, to] entry-date
 *  range; an empty bound is unbounded. */
export function getProfitAndLoss(
  from: string,
  to: string,
): Promise<AccountActivityRow[]> {
  const params = new URLSearchParams()
  if (from !== "") params.set("from", from)
  if (to !== "") params.set("to", to)
  const qs = params.toString()
  return get<AccountActivityRow[]>(
    `/profit-and-loss${qs === "" ? "" : `?${qs}`}`,
  )
}

/** Balances from posted entries dated on or before asOf ("" = all). */
export function getBalanceSheet(asOf: string): Promise<BalanceSheet> {
  return get<BalanceSheet>(
    asOf === "" ? "/balance-sheet" : `/balance-sheet?as_of=${asOf}`,
  )
}

/** One non-cash balance-sheet account's line on the cash-flow statement,
 *  mirroring reporting.CashFlowRow. amount is the account's cash impact —
 *  credit minus debit, so a source of cash is positive and a use of cash is
 *  negative. */
export interface CashFlowRow {
  account_id: number
  code: string
  name: string
  activity: string
  amount: string
}

/** The cash-flow statement, mirroring reporting.CashFlow (indirect method).
 *  net_income plus the rows always equals net_cash_flow, the movement of the
 *  cash accounts, and opening_cash + net_cash_flow = closing_cash. */
export interface CashFlow {
  net_income: string
  rows: CashFlowRow[]
  net_cash_flow: string
  opening_cash: string
  closing_cash: string
}

/** The cash-flow statement over the inclusive [from, to] entry-date range;
 *  an empty bound is unbounded. */
export function getCashFlow(from: string, to: string): Promise<CashFlow> {
  const params = new URLSearchParams()
  if (from !== "") params.set("from", from)
  if (to !== "") params.set("to", to)
  const qs = params.toString()
  return get<CashFlow>(`/cash-flow${qs === "" ? "" : `?${qs}`}`)
}

/** One posted journal line on an account's ledger, mirroring
 *  reporting.LedgerRow. memo prefers the line's own memo, falling back to
 *  the entry's. debit/credit are the entry's transaction-currency amounts;
 *  base_debit/base_credit are the base-currency conversions the reported
 *  balance sums. */
export interface LedgerRow {
  journal_entry_id: number
  entry_date: string
  reference: string | null
  memo: string | null
  currency_code: string
  debit: string
  credit: string
  base_debit: string
  base_credit: string
}

/** An account's posted journal lines in entry order over the inclusive
 *  [from, to] entry-date range; an empty bound is unbounded. */
export function getAccountLedger(
  accountId: number,
  from: string,
  to: string,
): Promise<LedgerRow[]> {
  const params = new URLSearchParams()
  if (from !== "") params.set("from", from)
  if (to !== "") params.set("to", to)
  const qs = params.toString()
  return get<LedgerRow[]>(
    `/accounts/${accountId}/ledger${qs === "" ? "" : `?${qs}`}`,
  )
}

/** One line of a journal entry, mirroring reporting.JournalEntryLine.
 *  debit/credit are transaction-currency amounts; base_debit/base_credit are
 *  their base-currency conversions. */
export interface JournalEntryLine {
  line_no: number
  account_id: number
  account_code: string
  account_name: string
  memo: string | null
  debit: string
  credit: string
  base_debit: string
  base_credit: string
}

/** A journal entry with all of its lines, mirroring reporting.JournalEntry.
 *  exchange_rate is the rate the entry's currency was converted to base at
 *  (1 for base-currency entries). */
export interface JournalEntry {
  id: number
  entry_date: string
  currency_code: string
  exchange_rate: string
  reference: string | null
  memo: string | null
  status: string
  lines: JournalEntryLine[]
}

export function getJournalEntry(id: number): Promise<JournalEntry> {
  return get<JournalEntry>(`/journal-entries/${id}`)
}

/** One party's outstanding balance bucketed by days overdue, mirroring
 *  reporting.AgingRow. Serves both AR (customer) and AP (supplier) aging. */
export interface AgingRow {
  party_id: number
  party_name: string
  total_outstanding: string
  not_yet_due: string
  days_1_30: string
  days_31_60: string
  days_61_90: string
  days_over_90: string
}

export function getARAging(): Promise<AgingRow[]> {
  return get<AgingRow[]>("/ar-aging")
}

export function getAPAging(): Promise<AgingRow[]> {
  return get<AgingRow[]>("/ap-aging")
}

/** A product's stock on hand across warehouses, mirroring
 *  reporting.StockValuationRow. */
export interface StockValuationRow {
  product_id: number
  sku: string
  name: string
  qty_on_hand: string
  value_on_hand: string
  avg_unit_cost: string
}

export function getInventoryValuation(): Promise<StockValuationRow[]> {
  return get<StockValuationRow[]>("/inventory/valuation")
}

// ---------------------------------------------------------------------------
// Sales orders and purchase orders
// ---------------------------------------------------------------------------

// Orders are commercial documents that never post to the GL; their lifecycle is
// draft -> open -> closed (or cancelled). Fulfilment creates ordinary invoices/
// bills and stock movements that draw down the order lines, and the derived
// fulfilment state (none/partial/invoiced|shipped|billed|received) is reported
// by the header views below.

/** Header view of a sales order, mirroring reporting.SalesOrderSummary. */
export interface SalesOrderSummary {
  id: number
  order_number: string
  customer_id: number
  order_date: string
  expected_ship_date: string | null
  currency_code: string
  status: string
  total: string
  invoiced_status: string
  shipped_status: string
  reference: string | null
  memo: string | null
}

/** Header view of a purchase order, mirroring reporting.PurchaseOrderSummary. */
export interface PurchaseOrderSummary {
  id: number
  order_number: string
  supplier_id: number
  order_date: string
  expected_receipt_date: string | null
  currency_code: string
  status: string
  total: string
  billed_status: string
  received_status: string
  reference: string | null
  memo: string | null
}

/** One sales-order line with its money and derived fulfilment quantities. */
export interface SalesOrderLine {
  line_no: number
  order_line_id: number
  product_id: number | null
  description: string
  quantity: string
  unit_price: string
  tax_code: string | null
  tax_rate: string
  line_subtotal: string
  tax_amount: string
  line_total: string
  revenue_account_id: number | null
  qty_invoiced: string
  qty_shipped: string
  qty_to_invoice: string
  qty_to_ship: string
}

/** One purchase-order line with its money and derived fulfilment quantities. */
export interface PurchaseOrderLine {
  line_no: number
  order_line_id: number
  product_id: number | null
  description: string
  quantity: string
  unit_cost: string
  tax_code: string | null
  tax_rate: string
  line_subtotal: string
  tax_amount: string
  line_total: string
  expense_account_id: number | null
  qty_billed: string
  qty_received: string
  qty_to_bill: string
  qty_to_receive: string
}

export interface SalesOrderLineInput {
  product_id: number | null
  description: string
  quantity: string
  unit_price: string
  revenue_account_id: number | null
  tax_code: string | null
  tax_rate: string
}

export interface SalesOrderInput {
  order_number: string
  customer_id: number
  order_date: string
  expected_ship_date: string | null
  currency_code: string
  reference: string | null
  memo: string | null
  lines: SalesOrderLineInput[]
}

export interface PurchaseOrderLineInput {
  product_id: number | null
  description: string
  quantity: string
  unit_cost: string
  expense_account_id: number | null
  tax_code: string | null
  tax_rate: string
}

export interface PurchaseOrderInput {
  order_number: string
  supplier_id: number
  order_date: string
  expected_receipt_date: string | null
  currency_code: string
  reference: string | null
  memo: string | null
  lines: PurchaseOrderLineInput[]
}

export function listSalesOrders(): Promise<SalesOrderSummary[]> {
  return get<SalesOrderSummary[]>("/sales-orders")
}

export function getSalesOrder(id: number): Promise<SalesOrderSummary> {
  return get<SalesOrderSummary>(`/sales-orders/${id}`)
}

export function getSalesOrderLines(id: number): Promise<SalesOrderLine[]> {
  return get<SalesOrderLine[]>(`/sales-orders/${id}/lines`)
}

export function createSalesOrder(
  input: SalesOrderInput,
): Promise<{ id: number }> {
  return post<{ id: number }>("/sales-orders", input)
}

/** Rewrite a draft order's header and full line set. */
export function updateSalesOrder(
  id: number,
  input: SalesOrderInput,
): Promise<void> {
  return send("PUT", `/sales-orders/${id}`, input)
}

/** Delete a draft order. */
export function deleteSalesOrder(id: number): Promise<void> {
  return del(`/sales-orders/${id}`)
}

export function listPurchaseOrders(): Promise<PurchaseOrderSummary[]> {
  return get<PurchaseOrderSummary[]>("/purchase-orders")
}

export function getPurchaseOrder(id: number): Promise<PurchaseOrderSummary> {
  return get<PurchaseOrderSummary>(`/purchase-orders/${id}`)
}

export function getPurchaseOrderLines(id: number): Promise<PurchaseOrderLine[]> {
  return get<PurchaseOrderLine[]>(`/purchase-orders/${id}/lines`)
}

export function createPurchaseOrder(
  input: PurchaseOrderInput,
): Promise<{ id: number }> {
  return post<{ id: number }>("/purchase-orders", input)
}

/** Rewrite a draft order's header and full line set. */
export function updatePurchaseOrder(
  id: number,
  input: PurchaseOrderInput,
): Promise<void> {
  return send("PUT", `/purchase-orders/${id}`, input)
}

/** Delete a draft order. */
export function deletePurchaseOrder(id: number): Promise<void> {
  return del(`/purchase-orders/${id}`)
}

// Lifecycle transitions. Each returns { status: "ok" } and moves the order
// between draft/open/closed/cancelled; the caller reloads to see the new state.
export function confirmSalesOrder(id: number): Promise<{ status: string }> {
  return post<{ status: string }>(`/sales-orders/${id}/confirm`, {})
}
export function closeSalesOrder(id: number): Promise<{ status: string }> {
  return post<{ status: string }>(`/sales-orders/${id}/close`, {})
}
export function cancelSalesOrder(id: number): Promise<{ status: string }> {
  return post<{ status: string }>(`/sales-orders/${id}/cancel`, {})
}
export function confirmPurchaseOrder(id: number): Promise<{ status: string }> {
  return post<{ status: string }>(`/purchase-orders/${id}/confirm`, {})
}
export function closePurchaseOrder(id: number): Promise<{ status: string }> {
  return post<{ status: string }>(`/purchase-orders/${id}/close`, {})
}
export function cancelPurchaseOrder(id: number): Promise<{ status: string }> {
  return post<{ status: string }>(`/purchase-orders/${id}/cancel`, {})
}

/** Fulfil one order line partially; omit the array to fulfil all remaining. */
export interface OrderLineQty {
  order_line_id: number
  quantity: string
}

export interface InvoiceFromOrderInput {
  invoice_number: string
  invoice_date: string
  due_date: string | null
  lines?: OrderLineQty[]
}

export interface BillFromOrderInput {
  bill_number: string
  bill_date: string
  due_date: string | null
  lines?: OrderLineQty[]
}

export interface FulfilMovementInput {
  warehouse_id: number
  movement_date: string | null
  reference: string | null
  lines?: OrderLineQty[]
}

export function invoiceSalesOrder(
  id: number,
  input: InvoiceFromOrderInput,
): Promise<{ invoice_id: number }> {
  return post<{ invoice_id: number }>(`/sales-orders/${id}/invoice`, input)
}

export function shipSalesOrder(
  id: number,
  input: FulfilMovementInput,
): Promise<{ movement_ids: number[] }> {
  return post<{ movement_ids: number[] }>(`/sales-orders/${id}/ship`, input)
}

export function billPurchaseOrder(
  id: number,
  input: BillFromOrderInput,
): Promise<{ bill_id: number }> {
  return post<{ bill_id: number }>(`/purchase-orders/${id}/bill`, input)
}

export function receivePurchaseOrder(
  id: number,
  input: FulfilMovementInput,
): Promise<{ movement_ids: number[] }> {
  return post<{ movement_ids: number[] }>(`/purchase-orders/${id}/receive`, input)
}

// ---------------------------------------------------------------------------
// Bank reconciliation
// ---------------------------------------------------------------------------

/** A bank statement header with derived matching progress, mirroring
 *  reporting.BankStatementSummary. lines_total is the signed sum of the line
 *  amounts (deposits positive); difference is opening + lines_total - closing,
 *  so "0.0000" means the statement adds up. */
export interface BankStatement {
  id: number
  account_id: number
  account_code: string
  account_name: string
  statement_date: string
  opening_balance: string
  closing_balance: string
  reference: string | null
  status: string
  line_count: number
  matched_count: number
  lines_total: string
  difference: string
}

/** The writable header of a bank statement, mirroring banking.StatementInput. */
export interface BankStatementInput {
  account_id: number
  statement_date: string
  opening_balance: string
  closing_balance: string
  reference: string | null
}

/** One statement transaction, mirroring reporting.BankStatementLine. The
 *  journal fields are set when the line is matched. */
export interface BankStatementLine {
  id: number
  line_no: number
  txn_date: string
  description: string
  reference: string | null
  amount: string
  journal_line_id: number | null
  journal_entry_id: number | null
  entry_date: string | null
  entry_memo: string | null
}

/** Input for one statement transaction, mirroring banking.LineInput. Amount
 *  is signed from the book's perspective: deposits positive. */
export interface BankStatementLineInput {
  txn_date: string
  description: string
  reference: string | null
  amount: string
}

/** A posted journal line on the statement's account that no statement line
 *  has claimed yet, mirroring reporting.BankMatchCandidate. Amount is signed
 *  debit-positive, matching the statement-line convention. */
export interface BankMatchCandidate {
  journal_line_id: number
  journal_entry_id: number
  entry_date: string
  reference: string | null
  memo: string | null
  amount: string
}

export function listBankStatements(): Promise<BankStatement[]> {
  return get<BankStatement[]>("/bank-statements")
}

export function getBankStatement(id: number): Promise<BankStatement> {
  return get<BankStatement>(`/bank-statements/${id}`)
}

export function getBankStatementLines(
  id: number,
): Promise<BankStatementLine[]> {
  return get<BankStatementLine[]>(`/bank-statements/${id}/lines`)
}

export function getBankMatchCandidates(
  id: number,
): Promise<BankMatchCandidate[]> {
  return get<BankMatchCandidate[]>(`/bank-statements/${id}/candidates`)
}

export function createBankStatement(
  input: BankStatementInput,
): Promise<{ id: number }> {
  return post<{ id: number }>("/bank-statements", input)
}

/** Rewrite an open statement's header. */
export function updateBankStatement(
  id: number,
  input: BankStatementInput,
): Promise<void> {
  return send("PUT", `/bank-statements/${id}`, input)
}

/** Delete an open statement; its lines go with it. */
export function deleteBankStatement(id: number): Promise<void> {
  return del(`/bank-statements/${id}`)
}

/** Append one transaction to an open statement. */
export function addBankStatementLine(
  id: number,
  input: BankStatementLineInput,
): Promise<{ id: number }> {
  return post<{ id: number }>(`/bank-statements/${id}/lines`, input)
}

/** Append transactions parsed from CSV text: date (YYYY-MM-DD), description,
 *  amount, optional reference. A header row is skipped automatically. */
export function importBankStatementCSV(
  id: number,
  csv: string,
): Promise<{ imported: number }> {
  return post<{ imported: number }>(`/bank-statements/${id}/import`, { csv })
}

/** Pair each unmatched line with the unused posted journal line of the same
 *  amount nearest in date; lines with no candidate are skipped. */
export function autoMatchBankStatement(
  id: number,
): Promise<{ matched: number }> {
  return post<{ matched: number }>(`/bank-statements/${id}/auto-match`, {})
}

/** Finalize a fully matched statement whose lines add up to the closing
 *  balance. */
export function reconcileBankStatement(id: number): Promise<void> {
  return send("POST", `/bank-statements/${id}/reconcile`, {})
}

/** Return a reconciled statement to open so it can be corrected (admin-only). */
export function reopenBankStatement(id: number): Promise<void> {
  return send("POST", `/bank-statements/${id}/reopen`, {})
}

/** Pair one statement line with a posted journal line on the statement's
 *  account carrying the same signed amount. */
export function matchBankStatementLine(
  lineId: number,
  journalLineId: number,
): Promise<void> {
  return send("POST", `/bank-statement-lines/${lineId}/match`, {
    journal_line_id: journalLineId,
  })
}

/** Release a statement line's match. */
export function unmatchBankStatementLine(lineId: number): Promise<void> {
  return send("POST", `/bank-statement-lines/${lineId}/unmatch`, {})
}

/** Remove a transaction from an open statement (releasing its match). */
export function deleteBankStatementLine(lineId: number): Promise<void> {
  return del(`/bank-statement-lines/${lineId}`)
}
