import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import { listPaymentTerms, type PaymentTerm } from "@/lib/api"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// Payment terms are keyed by their natural code ('NET30', …), so rows link by
// code, not id. The API returns them ordered by due days, shortest first.
export function PaymentTerms() {
  const [terms, setTerms] = useState<PaymentTerm[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    listPaymentTerms()
      .then((data) => {
        if (!cancelled) setTerms(data)
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
            Payment Terms
          </h1>
          <p className="text-sm text-muted-foreground">
            The payment terms customers and suppliers can carry, ordered by due
            days.
          </p>
        </div>
        <Button asChild>
          <Link to="/payment-terms/new">New payment term</Link>
        </Button>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load payment terms: {error}
        </p>
      )}

      {error === null && terms === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {terms !== null && terms.length === 0 && (
        <p className="text-sm text-muted-foreground">No payment terms yet.</p>
      )}

      {terms !== null && terms.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Code</TableHead>
              <TableHead>Name</TableHead>
              <TableHead className="text-right">Due Days</TableHead>
              <TableHead className="w-0"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {terms.map((t) => (
              <TableRow key={t.code}>
                <TableCell className="font-mono font-medium">
                  {t.code}
                </TableCell>
                <TableCell>{t.name}</TableCell>
                <TableCell className="text-right font-mono text-muted-foreground">
                  {t.due_days}
                </TableCell>
                <TableCell className="text-right">
                  <Link
                    to={`/payment-terms/${encodeURIComponent(t.code)}`}
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
