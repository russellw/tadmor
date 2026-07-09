import { useEffect, useState } from "react"
import { Link, useParams } from "react-router-dom"

import { sumAmounts } from "@/lib/amount"
import { getJournalEntry, type JournalEntry } from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// One journal entry from GET /api/journal-entries/{id}: header and its lines
// with accounts resolved. Account cells link onward to each account's ledger;
// the footer shows the entry balances (total debits = total credits).
export function JournalEntryDetail() {
  const { id } = useParams()
  const entryId = Number(id)

  const [entry, setEntry] = useState<JournalEntry | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!Number.isInteger(entryId) || entryId <= 0) {
      setError("Invalid journal entry id.")
      return
    }
    let cancelled = false
    getJournalEntry(entryId)
      .then((data) => {
        if (!cancelled) setEntry(data)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [entryId])

  return (
    <section className="mx-auto w-full max-w-5xl p-6">
      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/reports/trial-balance">Back to trial balance</Link>
          </Button>
        </div>
      )}

      {error === null && entry === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {entry !== null && (
        <>
          {(() => {
            // Show the base-currency conversion only for a foreign-currency
            // entry (one whose transaction and base amounts differ on a line).
            const foreign = entry.lines.some(
              (l) => l.debit !== l.base_debit || l.credit !== l.base_credit,
            )
            return (
              <>
                <header className="mb-6 flex items-start justify-between gap-4">
                  <div>
                    <div className="flex items-center gap-3">
                      <h1 className="text-2xl font-semibold tracking-tight">
                        Journal Entry #{entry.id}
                      </h1>
                      <Badge
                        variant={
                          entry.status === "posted" ? "default" : "outline"
                        }
                      >
                        {entry.status}
                      </Badge>
                    </div>
                    <p className="text-sm text-muted-foreground">
                      {entry.entry_date}
                      {entry.reference !== null && ` · ${entry.reference}`}
                      {" · "}
                      {entry.currency_code}
                      {foreign && ` · rate ${entry.exchange_rate}`}
                    </p>
                    {entry.memo !== null && (
                      <p className="mt-1 text-sm text-muted-foreground">
                        {entry.memo}
                      </p>
                    )}
                  </div>
                  <Button variant="outline" asChild>
                    <Link to="/reports/trial-balance">
                      Back to trial balance
                    </Link>
                  </Button>
                </header>

                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-10">#</TableHead>
                      <TableHead className="w-28">Code</TableHead>
                      <TableHead>Account</TableHead>
                      <TableHead>Memo</TableHead>
                      <TableHead className="text-right">Debit</TableHead>
                      <TableHead className="text-right">Credit</TableHead>
                      {foreign && (
                        <>
                          <TableHead className="text-right">Base Dr</TableHead>
                          <TableHead className="text-right">Base Cr</TableHead>
                        </>
                      )}
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {entry.lines.map((l) => (
                      <TableRow key={l.line_no}>
                        <TableCell className="text-muted-foreground">
                          {l.line_no}
                        </TableCell>
                        <TableCell className="font-mono">
                          {l.account_code}
                        </TableCell>
                        <TableCell>
                          <Link
                            to={`/accounts/${l.account_id}/ledger`}
                            className="font-medium text-primary hover:underline"
                          >
                            {l.account_name}
                          </Link>
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {l.memo ?? "—"}
                        </TableCell>
                        <AmountCell value={l.debit} />
                        <AmountCell value={l.credit} />
                        {foreign && (
                          <>
                            <AmountCell value={l.base_debit} />
                            <AmountCell value={l.base_credit} />
                          </>
                        )}
                      </TableRow>
                    ))}
                  </TableBody>
                  <TableFooter>
                    <TableRow>
                      <TableCell colSpan={4}>
                        {foreign
                          ? `Total (${entry.currency_code} / base)`
                          : "Total"}
                      </TableCell>
                      <AmountCell
                        value={sumAmounts(entry.lines.map((l) => l.debit))}
                      />
                      <AmountCell
                        value={sumAmounts(entry.lines.map((l) => l.credit))}
                      />
                      {foreign && (
                        <>
                          <AmountCell
                            value={sumAmounts(
                              entry.lines.map((l) => l.base_debit),
                            )}
                          />
                          <AmountCell
                            value={sumAmounts(
                              entry.lines.map((l) => l.base_credit),
                            )}
                          />
                        </>
                      )}
                    </TableRow>
                  </TableFooter>
                </Table>
              </>
            )
          })()}
        </>
      )}
    </section>
  )
}
