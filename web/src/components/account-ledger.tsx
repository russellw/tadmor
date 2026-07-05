import { useEffect, useState } from "react"
import { Link, useParams } from "react-router-dom"

import { negateAmount, sumAmounts } from "@/lib/amount"
import {
  getAccount,
  getAccountLedger,
  type Account,
  type LedgerRow,
} from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// One account's general ledger from GET /api/accounts/{id}/ledger: its posted
// journal lines in entry order, with a running debit-positive balance computed
// client-side. Rows link to the journal entry that produced them.
export function AccountLedger() {
  const { id } = useParams()
  const accountId = Number(id)

  const [account, setAccount] = useState<Account | null>(null)
  const [from, setFrom] = useState("")
  const [to, setTo] = useState("")
  const [rows, setRows] = useState<LedgerRow[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!Number.isInteger(accountId) || accountId <= 0) {
      setError("Invalid account id.")
      return
    }
    let cancelled = false
    Promise.all([getAccount(accountId), getAccountLedger(accountId, from, to)])
      .then(([acct, ledger]) => {
        if (!cancelled) {
          setAccount(acct)
          setRows(ledger)
          setError(null)
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [accountId, from, to])

  // Running balance, debit-positive (matching the trial balance's convention).
  const balances: string[] = []
  if (rows !== null) {
    let running = "0.0000"
    for (const r of rows) {
      running = sumAmounts([running, r.debit, negateAmount(r.credit)])
      balances.push(running)
    }
  }

  return (
    <section className="mx-auto w-full max-w-5xl p-6">
      <header className="mb-6 flex items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-semibold tracking-tight">
              {account !== null
                ? `${account.code} — ${account.name}`
                : "Account Ledger"}
            </h1>
            {account !== null && (
              <Badge variant="secondary">{account.account_type}</Badge>
            )}
          </div>
          <p className="text-sm text-muted-foreground">
            Posted journal lines, oldest first, with a running debit-positive
            balance.
          </p>
        </div>
        <Button variant="outline" asChild>
          <Link to="/reports/trial-balance">Back to trial balance</Link>
        </Button>
      </header>

      <div className="mb-6 flex flex-wrap gap-4">
        <div className="space-y-2">
          <Label htmlFor="from">From</Label>
          <Input
            id="from"
            type="date"
            className="w-44"
            value={from}
            onChange={(e) => setFrom(e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="to">To</Label>
          <Input
            id="to"
            type="date"
            className="w-44"
            value={to}
            onChange={(e) => setTo(e.target.value)}
          />
        </div>
      </div>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load the ledger: {error}
        </p>
      )}

      {error === null && rows === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {rows !== null && rows.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No posted activity in this range.
        </p>
      )}

      {rows !== null && rows.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-32">Date</TableHead>
              <TableHead className="w-24">Entry</TableHead>
              <TableHead>Memo</TableHead>
              <TableHead className="text-right">Debit</TableHead>
              <TableHead className="text-right">Credit</TableHead>
              <TableHead className="text-right">Balance</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r, i) => (
              <TableRow key={`${r.journal_entry_id}-${i}`}>
                <TableCell className="font-mono">{r.entry_date}</TableCell>
                <TableCell>
                  <Link
                    to={`/journal-entries/${r.journal_entry_id}`}
                    className="font-medium text-primary hover:underline"
                  >
                    #{r.journal_entry_id}
                  </Link>
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {r.memo ?? r.reference ?? "—"}
                </TableCell>
                <AmountCell value={r.debit} />
                <AmountCell value={r.credit} />
                <AmountCell value={balances[i]} />
              </TableRow>
            ))}
          </TableBody>
          <TableFooter>
            <TableRow>
              <TableCell colSpan={3}>Total</TableCell>
              <AmountCell value={sumAmounts(rows.map((r) => r.debit))} />
              <AmountCell value={sumAmounts(rows.map((r) => r.credit))} />
              <AmountCell value={balances[balances.length - 1]} />
            </TableRow>
          </TableFooter>
        </Table>
      )}
    </section>
  )
}
