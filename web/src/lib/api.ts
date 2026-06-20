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

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { Accept: "application/json" },
  })
  if (!res.ok) {
    // The backend reports errors as { "error": "..." }; fall back to the status.
    let message = `request failed (${res.status})`
    try {
      const body = (await res.json()) as { error?: string }
      if (body?.error) message = body.error
    } catch {
      // Non-JSON error body; keep the status-based message.
    }
    throw new ApiError(res.status, message)
  }
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

export function listCustomers(): Promise<Customer[]> {
  return get<Customer[]>("/customers")
}
