import { useEffect, useState } from "react"

import { negateAmount, sumAmounts } from "@/lib/amount"
import { getProfitAndLoss, type AccountActivityRow } from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
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

// The income statement from GET /api/profit-and-loss: revenue and expense
// activity over an entry-date range (blank bounds are unbounded). Amounts come
// back in natural sign — revenue credit-positive, expenses debit-positive — so
// net income is revenue minus expenses.
export function ProfitAndLoss() {
  const [from, setFrom] = useState("")
  const [to, setTo] = useState("")
  const [rows, setRows] = useState<AccountActivityRow[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    getProfitAndLoss(from, to)
      .then((data) => {
        if (!cancelled) {
          setRows(data)
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
  }, [from, to])

  const revenue = rows?.filter((r) => r.account_type === "revenue") ?? []
  const expenses = rows?.filter((r) => r.account_type === "expense") ?? []
  const totalRevenue = sumAmounts(revenue.map((r) => r.amount))
  const totalExpenses = sumAmounts(expenses.map((r) => r.amount))
  const netIncome = sumAmounts([totalRevenue, negateAmount(totalExpenses)])

  return (
    <section className="mx-auto w-full max-w-5xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">
          Profit &amp; Loss
        </h1>
        <p className="text-sm text-muted-foreground">
          Posted revenue and expenses over a date range; leave a bound blank
          for all time.
        </p>
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
          Failed to load the profit &amp; loss: {error}
        </p>
      )}

      {error === null && rows === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {rows !== null && rows.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No posted revenue or expenses in this range.
        </p>
      )}

      {rows !== null && rows.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-28">Code</TableHead>
              <TableHead>Account</TableHead>
              <TableHead className="text-right">Amount</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <SectionRows title="Revenue" rows={revenue} total={totalRevenue} />
            <SectionRows
              title="Expenses"
              rows={expenses}
              total={totalExpenses}
            />
          </TableBody>
          <TableFooter>
            <TableRow>
              <TableCell colSpan={2}>Net Income</TableCell>
              <AmountCell value={netIncome} />
            </TableRow>
          </TableFooter>
        </Table>
      )}
    </section>
  )
}

function SectionRows({
  title,
  rows,
  total,
}: {
  title: string
  rows: AccountActivityRow[]
  total: string
}) {
  if (rows.length === 0) return null
  return (
    <>
      <TableRow className="hover:bg-transparent">
        <TableCell colSpan={3} className="pt-4 font-semibold">
          {title}
        </TableCell>
      </TableRow>
      {rows.map((r) => (
        <TableRow key={r.account_id}>
          <TableCell className="font-mono">{r.code}</TableCell>
          <TableCell>{r.name}</TableCell>
          <AmountCell value={r.amount} />
        </TableRow>
      ))}
      <TableRow className="hover:bg-transparent">
        <TableCell colSpan={2} className="text-sm text-muted-foreground">
          Total {title.toLowerCase()}
        </TableCell>
        <AmountCell value={total} className="border-t" />
      </TableRow>
    </>
  )
}
