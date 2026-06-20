import { useEffect, useState } from "react"

import {
  listOrganizations,
  listSuppliers,
  type Organization,
  type Supplier,
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

// A supplier is a role on an organization, so the display name comes from the
// organization. We fetch both lists and join by organization_id client-side,
// reusing the existing endpoints rather than adding a joined backend query.
interface SupplierData {
  suppliers: Supplier[]
  orgsById: Map<number, Organization>
}

export function Suppliers() {
  const [data, setData] = useState<SupplierData | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([listSuppliers(), listOrganizations()])
      .then(([suppliers, organizations]) => {
        if (cancelled) return
        const orgsById = new Map(organizations.map((o) => [o.id, o]))
        setData({ suppliers, orgsById })
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
        <h1 className="text-2xl font-semibold tracking-tight">Suppliers</h1>
        <p className="text-sm text-muted-foreground">
          Organizations with a supplier role, ordered by id.
        </p>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load suppliers: {error}
        </p>
      )}

      {error === null && data === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {data !== null && data.suppliers.length === 0 && (
        <p className="text-sm text-muted-foreground">No suppliers yet.</p>
      )}

      {data !== null && data.suppliers.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Supplier #</TableHead>
              <TableHead>Currency</TableHead>
              <TableHead>Tax Code</TableHead>
              <TableHead>Terms</TableHead>
              <TableHead>Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.suppliers.map((s) => {
              const org = data.orgsById.get(s.organization_id)
              return (
                <TableRow key={s.id}>
                  <TableCell className="font-medium">
                    {org?.name ?? `#${s.organization_id}`}
                  </TableCell>
                  <TableCell className="font-mono">
                    {s.supplier_number ?? "—"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {s.currency_code ?? "—"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {s.tax_code ?? "—"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {s.payment_terms_code ?? "—"}
                  </TableCell>
                  <TableCell>
                    <Badge variant={s.is_active ? "default" : "outline"}>
                      {s.is_active ? "Active" : "Inactive"}
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
