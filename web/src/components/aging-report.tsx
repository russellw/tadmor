import { useEffect, useState } from "react"

import { sumAmounts } from "@/lib/amount"
import { getAPAging, getARAging, type AgingRow } from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// AR and AP aging share the same row shape (reporting.AgingRow), so both
// screens render through one parameterized report component.

export function ARAging() {
  return (
    <AgingReport
      title="AR Aging"
      description="Outstanding customer balances bucketed by days overdue."
      partyLabel="Customer"
      emptyMessage="No outstanding invoices."
      fetchRows={getARAging}
    />
  )
}

export function APAging() {
  return (
    <AgingReport
      title="AP Aging"
      description="Outstanding supplier balances bucketed by days overdue."
      partyLabel="Supplier"
      emptyMessage="No outstanding bills."
      fetchRows={getAPAging}
    />
  )
}

const buckets = [
  { label: "Current", value: (r: AgingRow) => r.not_yet_due },
  { label: "1–30", value: (r: AgingRow) => r.days_1_30 },
  { label: "31–60", value: (r: AgingRow) => r.days_31_60 },
  { label: "61–90", value: (r: AgingRow) => r.days_61_90 },
  { label: "Over 90", value: (r: AgingRow) => r.days_over_90 },
  { label: "Total", value: (r: AgingRow) => r.total_outstanding },
]

function AgingReport({
  title,
  description,
  partyLabel,
  emptyMessage,
  fetchRows,
}: {
  title: string
  description: string
  partyLabel: string
  emptyMessage: string
  fetchRows: () => Promise<AgingRow[]>
}) {
  const [rows, setRows] = useState<AgingRow[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    fetchRows()
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
  }, [fetchRows])

  return (
    <section className="mx-auto w-full max-w-5xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        <p className="text-sm text-muted-foreground">{description}</p>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load the report: {error}
        </p>
      )}

      {error === null && rows === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {rows !== null && rows.length === 0 && (
        <p className="text-sm text-muted-foreground">{emptyMessage}</p>
      )}

      {rows !== null && rows.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{partyLabel}</TableHead>
              {buckets.map((b) => (
                <TableHead key={b.label} className="text-right">
                  {b.label}
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r) => (
              <TableRow key={r.party_id}>
                <TableCell className="font-medium">{r.party_name}</TableCell>
                {buckets.map((b) => (
                  <AmountCell key={b.label} value={b.value(r)} />
                ))}
              </TableRow>
            ))}
          </TableBody>
          <TableFooter>
            <TableRow>
              <TableCell>Total</TableCell>
              {buckets.map((b) => (
                <AmountCell
                  key={b.label}
                  value={sumAmounts(rows.map(b.value))}
                />
              ))}
            </TableRow>
          </TableFooter>
        </Table>
      )}
    </section>
  )
}
