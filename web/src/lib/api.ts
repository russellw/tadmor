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

export function listAccounts(): Promise<Account[]> {
  return get<Account[]>("/accounts")
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

export function listOrganizations(): Promise<Organization[]> {
  return get<Organization[]>("/organizations")
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
