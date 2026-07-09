import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import {
  ApiError,
  listAccountingPeriods,
  listFiscalYears,
  reopenFiscalYear,
  updateAccountingPeriod,
  type AccountingPeriod,
  type FiscalYear,
} from "@/lib/api"
import { useCurrentUser } from "@/lib/current-user"
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

function StatusBadge({ status }: { status: string }) {
  return (
    <Badge variant={status === "open" ? "default" : "outline"}>{status}</Badge>
  )
}

// The fiscal calendar: fiscal years with their accounting periods nested under
// them. Posting a document requires an open period covering its date, so the
// recurring chore this screen exists for is "add next month's period" (the New
// period form prefills it) and closing finished months — hence the inline
// close/reopen toggle rather than a trip through the edit form. Admins also
// run the year-end close (and its undo) from here.
export function Periods() {
  const currentUser = useCurrentUser()
  const [years, setYears] = useState<FiscalYear[] | null>(null)
  const [periods, setPeriods] = useState<AccountingPeriod[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  // The id of the period whose status toggle is in flight, if any.
  const [togglingId, setTogglingId] = useState<number | null>(null)
  const [toggleError, setToggleError] = useState<string | null>(null)

  // The id of the fiscal year whose reopen is in flight, if any.
  const [reopeningId, setReopeningId] = useState<number | null>(null)
  const [reopenError, setReopenError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([listFiscalYears(), listAccountingPeriods()])
      .then(([fy, ap]) => {
        if (cancelled) return
        setYears(fy)
        setPeriods(ap)
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

  function toggleStatus(p: AccountingPeriod) {
    const status = p.status === "open" ? "closed" : "open"
    setTogglingId(p.id)
    setToggleError(null)
    updateAccountingPeriod(p.id, {
      fiscal_year_id: p.fiscal_year_id,
      name: p.name,
      start_date: p.start_date,
      end_date: p.end_date,
      status,
    })
      .then(() => {
        setPeriods(
          (prev) =>
            prev?.map((x) => (x.id === p.id ? { ...x, status } : x)) ?? prev,
        )
      })
      .catch((err: unknown) => {
        setToggleError(err instanceof ApiError ? err.message : String(err))
      })
      .finally(() => setTogglingId(null))
  }

  function reopenYear(fy: FiscalYear) {
    if (
      !window.confirm(
        `Reopen ${fy.name}? This reverses its year-end closing entry, restoring the revenue and expense balances.`,
      )
    ) {
      return
    }
    setReopeningId(fy.id)
    setReopenError(null)
    reopenFiscalYear(fy.id)
      // Reload rather than patch state: reopening also reopens the period
      // that held the closing entry.
      .then(() => Promise.all([listFiscalYears(), listAccountingPeriods()]))
      .then(([fy, ap]) => {
        setYears(fy)
        setPeriods(ap)
      })
      .catch((err: unknown) => {
        setReopenError(err instanceof ApiError ? err.message : String(err))
      })
      .finally(() => setReopeningId(null))
  }

  const loaded = years !== null && periods !== null

  return (
    <section className="mx-auto w-full max-w-5xl p-6">
      <header className="mb-6 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Periods</h1>
          <p className="text-sm text-muted-foreground">
            The fiscal calendar. Documents can only post into an open period,
            so each new month needs a period here before anything can post in
            it.
          </p>
        </div>
        <div className="flex shrink-0 gap-2">
          <Button asChild>
            <Link to="/periods/new">New period</Link>
          </Button>
          <Button variant="outline" asChild>
            <Link to="/fiscal-years/new">New fiscal year</Link>
          </Button>
        </div>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load periods: {error}
        </p>
      )}

      {error === null && !loaded && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {loaded && years.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No fiscal years yet. Create one, then add its periods.
        </p>
      )}

      {toggleError !== null && (
        <p className="mb-4 text-sm text-destructive" role="alert">
          Failed to update period: {toggleError}
        </p>
      )}

      {reopenError !== null && (
        <p className="mb-4 text-sm text-destructive" role="alert">
          Failed to reopen fiscal year: {reopenError}
        </p>
      )}

      {loaded &&
        years.map((fy) => {
          const fyPeriods = periods.filter((p) => p.fiscal_year_id === fy.id)
          return (
            <div key={fy.id} className="mb-8">
              <div className="mb-2 flex items-center gap-3">
                <h2 className="text-lg font-semibold">{fy.name}</h2>
                <span className="text-sm text-muted-foreground">
                  {fy.start_date} → {fy.end_date}
                </span>
                <StatusBadge status={fy.status} />
                <div className="ml-auto flex items-center gap-3">
                  {currentUser.is_admin &&
                    (fy.status === "open" ? (
                      <Button variant="outline" size="sm" asChild>
                        <Link to={`/fiscal-years/${fy.id}/close`}>
                          Close year
                        </Link>
                      </Button>
                    ) : (
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={reopeningId === fy.id}
                        onClick={() => reopenYear(fy)}
                      >
                        Reopen year
                      </Button>
                    ))}
                  <Link
                    to={`/fiscal-years/${fy.id}`}
                    className="text-sm font-medium text-primary hover:underline"
                  >
                    Edit
                  </Link>
                </div>
              </div>
              {fyPeriods.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No periods in this fiscal year yet.
                </p>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Period</TableHead>
                      <TableHead>Start</TableHead>
                      <TableHead>End</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead className="w-0"></TableHead>
                      <TableHead className="w-0"></TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {fyPeriods.map((p) => (
                      <TableRow key={p.id}>
                        <TableCell className="font-medium">{p.name}</TableCell>
                        <TableCell className="text-muted-foreground">
                          {p.start_date}
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {p.end_date}
                        </TableCell>
                        <TableCell>
                          <StatusBadge status={p.status} />
                        </TableCell>
                        <TableCell className="text-right">
                          <Button
                            variant="outline"
                            size="sm"
                            // Periods in a closed year are locked (the
                            // database enforces it); reopen the year first.
                            disabled={
                              togglingId === p.id || fy.status === "closed"
                            }
                            onClick={() => toggleStatus(p)}
                          >
                            {p.status === "open" ? "Close" : "Reopen"}
                          </Button>
                        </TableCell>
                        <TableCell className="text-right">
                          <Link
                            to={`/periods/${p.id}`}
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
            </div>
          )
        })}
    </section>
  )
}
