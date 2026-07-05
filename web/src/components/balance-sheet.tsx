import { useEffect, useState } from "react"

import { sumAmounts } from "@/lib/amount"
import { getBalanceSheet, type BalanceSheet } from "@/lib/api"
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

// The balance sheet from GET /api/balance-sheet: asset, liability, and equity
// balances as of a date (blank = all posted entries). Amounts come back in
// natural sign, and the equity section carries the API's current-earnings
// figure — net income not yet closed into retained earnings — so that
// assets = liabilities + equity.
export function BalanceSheetReport() {
  const [asOf, setAsOf] = useState("")
  const [sheet, setSheet] = useState<BalanceSheet | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    getBalanceSheet(asOf)
      .then((data) => {
        if (!cancelled) {
          setSheet(data)
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
  }, [asOf])

  const assets = sheet?.rows.filter((r) => r.account_type === "asset") ?? []
  const liabilities =
    sheet?.rows.filter((r) => r.account_type === "liability") ?? []
  const equity = sheet?.rows.filter((r) => r.account_type === "equity") ?? []
  const totalAssets = sumAmounts(assets.map((r) => r.amount))
  const totalLiabilities = sumAmounts(liabilities.map((r) => r.amount))
  const totalEquity = sumAmounts([
    ...equity.map((r) => r.amount),
    sheet?.current_earnings ?? "0",
  ])
  const empty = sheet !== null && sheet.rows.length === 0

  return (
    <section className="mx-auto w-full max-w-5xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">
          Balance Sheet
        </h1>
        <p className="text-sm text-muted-foreground">
          Assets, liabilities, and equity from posted entries; leave the date
          blank for all time.
        </p>
      </header>

      <div className="mb-6 space-y-2">
        <Label htmlFor="as_of">As of</Label>
        <Input
          id="as_of"
          type="date"
          className="w-44"
          value={asOf}
          onChange={(e) => setAsOf(e.target.value)}
        />
      </div>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load the balance sheet: {error}
        </p>
      )}

      {error === null && sheet === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {empty && (
        <p className="text-sm text-muted-foreground">
          Nothing posted on or before this date.
        </p>
      )}

      {sheet !== null && !empty && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-28">Code</TableHead>
              <TableHead>Account</TableHead>
              <TableHead className="text-right">Amount</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <SectionRows title="Assets" rows={assets} total={totalAssets} />
            <SectionRows
              title="Liabilities"
              rows={liabilities}
              total={totalLiabilities}
            />
            <SectionRows
              title="Equity"
              rows={equity}
              total={totalEquity}
              extraRow={{
                label: "Current earnings",
                amount: sheet.current_earnings,
              }}
            />
          </TableBody>
          <TableFooter>
            <TableRow>
              <TableCell colSpan={2}>Liabilities + Equity</TableCell>
              <AmountCell value={sumAmounts([totalLiabilities, totalEquity])} />
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
  extraRow,
}: {
  title: string
  rows: { account_id: number; code: string; name: string; amount: string }[]
  total: string
  extraRow?: { label: string; amount: string }
}) {
  if (rows.length === 0 && extraRow === undefined) return null
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
      {extraRow !== undefined && (
        <TableRow>
          <TableCell />
          <TableCell className="italic">{extraRow.label}</TableCell>
          <AmountCell value={extraRow.amount} />
        </TableRow>
      )}
      <TableRow className="hover:bg-transparent">
        <TableCell colSpan={2} className="text-sm text-muted-foreground">
          Total {title.toLowerCase()}
        </TableCell>
        <AmountCell value={total} className="border-t" />
      </TableRow>
    </>
  )
}
