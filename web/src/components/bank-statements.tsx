import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import { isZeroAmount } from "@/lib/amount"
import { listBankStatements, type BankStatement } from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
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

// Statement lifecycle badge (open → reconciled), shared with the detail
// screen.
export function ReconciledBadge({ status }: { status: string }) {
  return (
    <Badge variant={status === "reconciled" ? "default" : "outline"}>
      {status}
    </Badge>
  )
}

export function BankStatements() {
  const [statements, setStatements] = useState<BankStatement[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    listBankStatements()
      .then((rows) => {
        if (!cancelled) setStatements(rows)
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
            Bank Reconciliation
          </h1>
          <p className="text-sm text-muted-foreground">
            Bank statements matched against the ledger, newest first.
          </p>
        </div>
        <Button asChild>
          <Link to="/bank-statements/new">New statement</Link>
        </Button>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load: {error}
        </p>
      )}

      {error === null && statements === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {statements !== null && statements.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No bank statements yet. Capture one to start reconciling a cash
          account.
        </p>
      )}

      {statements !== null && statements.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-16">#</TableHead>
              <TableHead>Account</TableHead>
              <TableHead>Date</TableHead>
              <TableHead>Reference</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="text-right">Matched</TableHead>
              <TableHead className="text-right">Closing</TableHead>
              <TableHead className="text-right">Difference</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {statements.map((s) => (
              <TableRow key={s.id}>
                <TableCell className="font-mono">
                  <Link
                    to={`/bank-statements/${s.id}`}
                    className="font-medium text-primary hover:underline"
                  >
                    {s.id}
                  </Link>
                </TableCell>
                <TableCell className="font-medium">
                  {s.account_code} {s.account_name}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {s.statement_date}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {s.reference ?? "—"}
                </TableCell>
                <TableCell>
                  <ReconciledBadge status={s.status} />
                </TableCell>
                <TableCell className="text-right font-mono tabular-nums">
                  {s.matched_count}/{s.line_count}
                </TableCell>
                <AmountCell value={s.closing_balance} />
                <AmountCell
                  value={s.difference}
                  className={
                    isZeroAmount(s.difference) ? undefined : "text-destructive"
                  }
                />
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </section>
  )
}
