import { useEffect, useState } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import {
  ApiError,
  closeFiscalYear,
  getFiscalYear,
  listAccounts,
  type Account,
  type FiscalYear,
} from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

// The confirmation page for the year-end close (admin-only; the backend
// enforces that). The only input is where net income lands: a postable,
// active equity account, defaulting to the conventional Retained Earnings.
export function FiscalYearClose() {
  const { id } = useParams()
  const fiscalYearId = Number(id)
  const navigate = useNavigate()

  const [year, setYear] = useState<FiscalYear | null>(null)
  const [equityAccounts, setEquityAccounts] = useState<Account[] | null>(null)
  const [accountId, setAccountId] = useState<string>("")
  const [error, setError] = useState<string | null>(null)
  const [closing, setClosing] = useState(false)
  const [closeError, setCloseError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    if (!Number.isInteger(fiscalYearId) || fiscalYearId <= 0) {
      setError("Invalid fiscal year id.")
      return
    }
    Promise.all([getFiscalYear(fiscalYearId), listAccounts()])
      .then(([fy, accounts]) => {
        if (cancelled) return
        const equity = accounts.filter(
          (a) => a.account_type === "equity" && a.is_postable && a.is_active,
        )
        setYear(fy)
        setEquityAccounts(equity)
        const preferred =
          equity.find((a) => a.name.toLowerCase() === "retained earnings") ??
          equity[0]
        if (preferred !== undefined) setAccountId(String(preferred.id))
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [fiscalYearId])

  function handleClose() {
    setClosing(true)
    setCloseError(null)
    closeFiscalYear(fiscalYearId, Number(accountId))
      .then(() => navigate("/periods"))
      .catch((err: unknown) => {
        setClosing(false)
        setCloseError(err instanceof ApiError ? err.message : String(err))
      })
  }

  const loaded = year !== null && equityAccounts !== null

  return (
    <section className="mx-auto w-full max-w-2xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">
          Close Fiscal Year{year !== null ? ` — ${year.name}` : ""}
        </h1>
        <p className="text-sm text-muted-foreground">
          The year-end close posts a closing entry that zeroes every revenue
          and expense account into retained earnings, closes all of the
          year&apos;s periods and the year itself, and creates the next fiscal
          year if it doesn&apos;t exist yet. Reports are unaffected: the
          income statement ignores closing entries. A closed year can be
          reopened later, which reverses the closing entry.
        </p>
      </header>

      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/periods">Back to periods</Link>
          </Button>
        </div>
      )}

      {error === null && !loaded && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {loaded && year.status !== "open" && (
        <div className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {year.name} is already closed.
          </p>
          <Button variant="outline" asChild>
            <Link to="/periods">Back to periods</Link>
          </Button>
        </div>
      )}

      {loaded && year.status === "open" && (
        <div className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="re-account">Retained earnings account</Label>
            {equityAccounts.length === 0 ? (
              <p className="text-sm text-destructive" role="alert">
                No postable, active equity account exists. Create one on the
                Chart of Accounts screen first.
              </p>
            ) : (
              <Select value={accountId} onValueChange={setAccountId}>
                <SelectTrigger id="re-account" className="w-full">
                  <SelectValue placeholder="Select an equity account" />
                </SelectTrigger>
                <SelectContent>
                  {equityAccounts.map((a) => (
                    <SelectItem key={a.id} value={String(a.id)}>
                      {a.code} — {a.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
            <p className="text-sm text-muted-foreground">
              The year&apos;s net income (or loss) is posted here.
            </p>
          </div>

          {closeError !== null && (
            <p className="text-sm text-destructive" role="alert">
              Failed to close the year: {closeError}
            </p>
          )}

          <div className="flex gap-2">
            <Button
              onClick={handleClose}
              disabled={closing || accountId === ""}
            >
              {closing ? "Closing…" : `Close ${year.name}`}
            </Button>
            <Button variant="outline" asChild>
              <Link to="/periods">Cancel</Link>
            </Button>
          </div>
        </div>
      )}
    </section>
  )
}
