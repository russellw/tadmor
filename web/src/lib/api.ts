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

/** A general-ledger account, mirroring master.Account on the backend. */
export interface Account {
  id: number
  code: string
  name: string
  account_type: string
  parent_id: number | null
  currency_code: string | null
  is_postable: boolean
  is_active: boolean
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
}

/** The writable fields of an organization (Organization without its id),
 *  mirroring master.OrganizationInput. PUT is a full replace. */
export interface OrganizationInput {
  name: string
  legal_name: string | null
  tax_id: string | null
  country_code: string | null
  default_currency: string | null
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

export function listWarehouses(): Promise<Warehouse[]> {
  return get<Warehouse[]>("/warehouses")
}

/** A tax code (natural key: code), mirroring master.TaxCode. */
export interface TaxCode {
  code: string
  name: string
  rate: string
  tax_account_id: number | null
  is_active: boolean
}

export function listTaxCodes(): Promise<TaxCode[]> {
  return get<TaxCode[]>("/tax-codes")
}

/** A fiscal year, mirroring master.FiscalYear. Dates are YYYY-MM-DD strings. */
export interface FiscalYear {
  id: number
  name: string
  start_date: string
  end_date: string
  status: string
}

/** The writable fields of a fiscal year (FiscalYear without its id), mirroring
 *  master.FiscalYearInput. PUT is a full replace; status is open|closed and is
 *  only honored on update (creates always start open). */
export interface FiscalYearInput {
  name: string
  start_date: string
  end_date: string
  status: string
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

/** Post a movement to the GL. currency is required; credit_account_id is the
 *  clearing account a receipt credits (ignored for issues). */
export function postStockMovement(
  id: number,
  currency: string,
  creditAccountId: number | null,
): Promise<{ journal_entry_id: number }> {
  return post<{ journal_entry_id: number }>(`/stock-movements/${id}/post`, {
    currency,
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
