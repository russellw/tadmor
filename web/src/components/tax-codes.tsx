import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import { listTaxCodes, type TaxCode } from "@/lib/api"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// Tax codes are keyed by their natural code ('STD', 'ZERO', …), so rows link
// by code, not id. Rates are percent strings straight from the database.
export function TaxCodes() {
  const [taxCodes, setTaxCodes] = useState<TaxCode[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    listTaxCodes()
      .then((data) => {
        if (!cancelled) setTaxCodes(data)
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
          <h1 className="text-2xl font-semibold tracking-tight">Tax Codes</h1>
          <p className="text-sm text-muted-foreground">
            The tax rates document lines can carry, ordered by code.
          </p>
        </div>
        <Button asChild>
          <Link to="/tax-codes/new">New tax code</Link>
        </Button>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load tax codes: {error}
        </p>
      )}

      {error === null && taxCodes === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {taxCodes !== null && taxCodes.length === 0 && (
        <p className="text-sm text-muted-foreground">No tax codes yet.</p>
      )}

      {taxCodes !== null && taxCodes.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Code</TableHead>
              <TableHead>Name</TableHead>
              <TableHead className="text-right">Rate %</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="w-0"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {taxCodes.map((t) => (
              <TableRow key={t.code}>
                <TableCell className="font-mono font-medium">
                  {t.code}
                </TableCell>
                <TableCell>{t.name}</TableCell>
                <TableCell className="text-right font-mono text-muted-foreground">
                  {t.rate}
                </TableCell>
                <TableCell>
                  <Badge variant={t.is_active ? "default" : "outline"}>
                    {t.is_active ? "Active" : "Inactive"}
                  </Badge>
                </TableCell>
                <TableCell className="text-right">
                  <Link
                    to={`/tax-codes/${encodeURIComponent(t.code)}`}
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
