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

// The backend reports errors as { "error": "..." }; fall back to the status.
async function failure(res: Response): Promise<ApiError> {
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
