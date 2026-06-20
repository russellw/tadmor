import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import { listOrganizations, type Organization } from "@/lib/api"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// Organizations are the parties a customer/supplier role attaches to. They are
// standalone (their own name), so this is a single fetch with no join.
export function Organizations() {
  const [organizations, setOrganizations] = useState<Organization[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    listOrganizations()
      .then((data) => {
        if (!cancelled) setOrganizations(data)
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
      <header className="mb-6 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Organizations
          </h1>
          <p className="text-sm text-muted-foreground">
            The parties customers and suppliers attach to, ordered by name.
          </p>
        </div>
        <Button asChild>
          <Link to="/organizations/new">New organization</Link>
        </Button>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load organizations: {error}
        </p>
      )}

      {error === null && organizations === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {organizations !== null && organizations.length === 0 && (
        <p className="text-sm text-muted-foreground">No organizations yet.</p>
      )}

      {organizations !== null && organizations.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Legal Name</TableHead>
              <TableHead>Tax ID</TableHead>
              <TableHead>Country</TableHead>
              <TableHead>Currency</TableHead>
              <TableHead className="w-0"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {organizations.map((o) => (
              <TableRow key={o.id}>
                <TableCell className="font-medium">{o.name}</TableCell>
                <TableCell className="text-muted-foreground">
                  {o.legal_name ?? "—"}
                </TableCell>
                <TableCell className="font-mono text-muted-foreground">
                  {o.tax_id ?? "—"}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {o.country_code ?? "—"}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {o.default_currency ?? "—"}
                </TableCell>
                <TableCell className="text-right">
                  <Link
                    to={`/organizations/${o.id}`}
                    className="text-sm font-medium text-primary hover:underline"
                  >
                    Edit
                  </Link>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </section>
  )
}
