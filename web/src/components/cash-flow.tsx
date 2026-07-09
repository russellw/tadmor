import { useEffect, useState } from "react"

import { isZeroAmount, sumAmounts } from "@/lib/amount"
import { getCashFlow, type CashFlow, type CashFlowRow } from "@/lib/api"
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

// The cash-flow statement from GET /api/cash-flow (indirect method): net
// income plus each non-cash balance-sheet account's cash impact, grouped into
// operating / investing / financing activities. Amounts are cash impact —
// sources of cash positive, uses negative — so the sections sum to the net
// change in cash, reconciled to the cash accounts' opening and closing
// balances.
export function CashFlowStatement() {
  const [from, setFrom] = useState("")
  const [to, setTo] = useState("")
  const [statement, setStatement] = useState<CashFlow | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    getCashFlow(from, to)
      .then((data) => {
        if (!cancelled) {
          setStatement(data)
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

  const operating =
    statement?.rows.filter((r) => r.activity === "operating") ?? []
  const investing =
    statement?.rows.filter((r) => r.activity === "investing") ?? []
  const financing =
    statement?.rows.filter((r) => r.activity === "financing") ?? []
  const operatingTotal = sumAmounts([
    statement?.net_income ?? "0",
    ...operating.map((r) => r.amount),
  ])
  const investingTotal = sumAmounts(investing.map((r) => r.amount))
  const financingTotal = sumAmounts(financing.map((r) => r.amount))
  const empty =
    statement !== null &&
    statement.rows.length === 0 &&
    isZeroAmount(statement.net_income) &&
    isZeroAmount(statement.net_cash_flow)

  return (
    <section className="mx-auto w-full max-w-5xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">Cash Flow</h1>
        <p className="text-sm text-muted-foreground">
          Sources and uses of cash over a date range (indirect method); leave
          a bound blank for all time.
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
          Failed to load the cash flow: {error}
        </p>
      )}

      {error === null && statement === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {empty && (
        <p className="text-sm text-muted-foreground">
          No posted activity in this range.
        </p>
      )}

      {statement !== null && !empty && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-28">Code</TableHead>
              <TableHead>Account</TableHead>
              <TableHead className="text-right">Amount</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <SectionRows
              title="Operating activities"
              rows={operating}
              total={operatingTotal}
              leadRow={{ label: "Net income", amount: statement.net_income }}
            />
            <SectionRows
              title="Investing activities"
              rows={investing}
              total={investingTotal}
            />
            <SectionRows
              title="Financing activities"
              rows={financing}
              total={financingTotal}
            />
          </TableBody>
          <TableFooter>
            <TableRow>
              <TableCell colSpan={2}>Net change in cash</TableCell>
              <AmountCell value={statement.net_cash_flow} />
            </TableRow>
            <TableRow>
              <TableCell colSpan={2}>Cash at start of range</TableCell>
              <AmountCell value={statement.opening_cash} />
            </TableRow>
            <TableRow>
              <TableCell colSpan={2}>Cash at end of range</TableCell>
              <AmountCell value={statement.closing_cash} />
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
  leadRow,
}: {
  title: string
  rows: CashFlowRow[]
  total: string
  leadRow?: { label: string; amount: string }
}) {
  if (rows.length === 0 && leadRow === undefined) return null
  return (
    <>
      <TableRow className="hover:bg-transparent">
        <TableCell colSpan={3} className="pt-4 font-semibold">
          {title}
        </TableCell>
      </TableRow>
      {leadRow !== undefined && (
        <TableRow>
          <TableCell />
          <TableCell className="italic">{leadRow.label}</TableCell>
          <AmountCell value={leadRow.amount} />
        </TableRow>
      )}
      {rows.map((r) => (
        <TableRow key={r.account_id}>
          <TableCell className="font-mono">{r.code}</TableCell>
          <TableCell>{r.name}</TableCell>
          <AmountCell value={r.amount} />
        </TableRow>
      ))}
      <TableRow className="hover:bg-transparent">
        <TableCell colSpan={2} className="text-sm text-muted-foreground">
          Net cash from {title.toLowerCase()}
        </TableCell>
        <AmountCell value={total} className="border-t" />
      </TableRow>
    </>
  )
}
