import { useEffect, useState } from "react"

import { sumAmounts } from "@/lib/amount"
import { getTrialBalance, type TrialBalanceRow } from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// The trial balance from GET /api/trial-balance: every account's posted debit
// and credit totals, with a footer row showing that debits equal credits.
export function TrialBalance() {
  const [rows, setRows] = useState<TrialBalanceRow[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    getTrialBalance()
      .then((data) => {
        if (!cancelled) setRows(data)
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
        <h1 className="text-2xl font-semibold tracking-tight">Trial Balance</h1>
        <p className="text-sm text-muted-foreground">
          Posted debit and credit totals per account, ordered by code.
        </p>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load the trial balance: {error}
        </p>
      )}

      {error === null && rows === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {rows !== null && rows.length === 0 && (
        <p className="text-sm text-muted-foreground">
          Nothing has been posted yet.
        </p>
      )}

      {rows !== null && rows.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-28">Code</TableHead>
              <TableHead>Account</TableHead>
              <TableHead>Type</TableHead>
              <TableHead className="text-right">Debit</TableHead>
              <TableHead className="text-right">Credit</TableHead>
              <TableHead className="text-right">Balance</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r) => (
              <TableRow key={r.account_id}>
                <TableCell className="font-mono">{r.code}</TableCell>
                <TableCell className="font-medium">{r.name}</TableCell>
                <TableCell>
                  <Badge variant="secondary">{r.account_type}</Badge>
                </TableCell>
                <AmountCell value={r.total_debit} />
                <AmountCell value={r.total_credit} />
                <AmountCell value={r.balance} />
              </TableRow>
            ))}
          </TableBody>
          <TableFooter>
            <TableRow>
              <TableCell colSpan={3}>Total</TableCell>
              <AmountCell value={sumAmounts(rows.map((r) => r.total_debit))} />
              <AmountCell value={sumAmounts(rows.map((r) => r.total_credit))} />
              <AmountCell value={sumAmounts(rows.map((r) => r.balance))} />
            </TableRow>
          </TableFooter>
        </Table>
      )}
    </section>
  )
}
