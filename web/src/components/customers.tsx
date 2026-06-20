import { useEffect, useState } from "react"

import {
  listCustomers,
  listOrganizations,
  type Customer,
  type Organization,
} from "@/lib/api"
import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// A customer is a role on an organization, so the display name comes from the
// organization. We fetch both lists and join by organization_id client-side,
// reusing the existing endpoints rather than adding a joined backend query.
interface CustomerData {
  customers: Customer[]
  orgsById: Map<number, Organization>
}

export function Customers() {
  const [data, setData] = useState<CustomerData | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([listCustomers(), listOrganizations()])
      .then(([customers, organizations]) => {
        if (cancelled) return
        const orgsById = new Map(organizations.map((o) => [o.id, o]))
        setData({ customers, orgsById })
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <section className="mx-auto w-full max-w-5xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">Customers</h1>
        <p className="text-sm text-muted-foreground">
          Organizations with a customer role, ordered by id.
        </p>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load customers: {error}
        </p>
      )}

      {error === null && data === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {data !== null && data.customers.length === 0 && (
        <p className="text-sm text-muted-foreground">No customers yet.</p>
      )}

      {data !== null && data.customers.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Customer #</TableHead>
              <TableHead>Currency</TableHead>
              <TableHead>Tax Code</TableHead>
              <TableHead>Terms</TableHead>
              <TableHead className="text-right">Credit Limit</TableHead>
              <TableHead>Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.customers.map((c) => {
              const org = data.orgsById.get(c.organization_id)
              return (
                <TableRow key={c.id}>
                  <TableCell className="font-medium">
                    {org?.name ?? `#${c.organization_id}`}
                  </TableCell>
                  <TableCell className="font-mono">
                    {c.customer_number ?? "—"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {c.currency_code ?? "—"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {c.tax_code ?? "—"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {c.payment_terms_code ?? "—"}
                  </TableCell>
                  <TableCell className="text-right font-mono tabular-nums">
                    {c.credit_limit ?? "—"}
                  </TableCell>
                  <TableCell>
                    <Badge variant={c.is_active ? "default" : "outline"}>
                      {c.is_active ? "Active" : "Inactive"}
                    </Badge>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      )}
    </section>
  )
}
