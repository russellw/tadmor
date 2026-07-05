import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import {
  ApiError,
  createAccountingPeriod,
  getAccountingPeriod,
  listAccountingPeriods,
  listFiscalYears,
  updateAccountingPeriod,
  type FiscalYear,
} from "@/lib/api"
import { dayAfter, endOfMonth, yearMonth } from "@/lib/dates"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

type Mode = "create" | "edit"

interface FormState {
  fiscalYearId: string
  name: string
  startDate: string
  endDate: string
  status: string
}

// In create mode the form prefills the month after the latest existing period
// — the "open next month so documents can post in it" chore this screen exists
// for. With no periods yet it offers the current month instead.
function nextMonthDefaults(
  latestEnd: string | undefined,
  years: FiscalYear[],
): FormState {
  const start =
    latestEnd !== undefined
      ? dayAfter(latestEnd)
      : `${new Date().toISOString().slice(0, 8)}01`
  // The fiscal year whose range covers the new period, or the latest one as a
  // stand-in (the user may need to create the next fiscal year first).
  const covering = years.find(
    (fy) => fy.start_date <= start && start <= fy.end_date,
  )
  const fy = covering ?? years.at(-1)
  return {
    fiscalYearId: fy !== undefined ? String(fy.id) : "",
    name: yearMonth(start),
    startDate: start,
    endDate: endOfMonth(start),
    status: "open",
  }
}

export function PeriodForm({ mode }: { mode: Mode }) {
  const { id } = useParams()
  const periodId = Number(id)
  const navigate = useNavigate()

  const [years, setYears] = useState<FiscalYear[]>([])
  const [form, setForm] = useState<FormState | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      try {
        if (mode === "edit") {
          if (!Number.isInteger(periodId) || periodId <= 0) {
            setError("Invalid period id.")
            return
          }
          const [p, fy] = await Promise.all([
            getAccountingPeriod(periodId),
            listFiscalYears(),
          ])
          if (cancelled) return
          setYears(fy)
          setForm({
            fiscalYearId: String(p.fiscal_year_id),
            name: p.name,
            startDate: p.start_date,
            endDate: p.end_date,
            status: p.status,
          })
        } else {
          // Both lists come back ordered by start_date, so the last period is
          // the latest.
          const [fy, periods] = await Promise.all([
            listFiscalYears(),
            listAccountingPeriods(),
          ])
          if (cancelled) return
          setYears(fy)
          setForm(nextMonthDefaults(periods.at(-1)?.end_date, fy))
        }
      } catch (err: unknown) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [mode, periodId])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form) return
    if (form.fiscalYearId === "") {
      setSaveError("Please choose a fiscal year.")
      return
    }
    if (form.name.trim() === "") {
      setSaveError("Name is required.")
      return
    }
    const input = {
      fiscal_year_id: Number(form.fiscalYearId),
      name: form.name.trim(),
      start_date: form.startDate,
      end_date: form.endDate,
      status: form.status,
    }
    setSaving(true)
    setSaveError(null)
    const action =
      mode === "create"
        ? createAccountingPeriod(input).then(() => undefined)
        : updateAccountingPeriod(periodId, input)
    action
      .then(() => navigate("/periods"))
      .catch((err: unknown) => {
        setSaving(false)
        setSaveError(err instanceof ApiError ? err.message : String(err))
      })
  }

  const creating = mode === "create"

  return (
    <section className="mx-auto w-full max-w-2xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">
          {creating ? "New Period" : "Edit Period"}
        </h1>
        <p className="text-sm text-muted-foreground">
          {creating
            ? "Open an accounting period so documents can post in it."
            : "Update the period's details."}
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

      {error === null && form === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {form !== null && (
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="fiscal_year">Fiscal Year</Label>
            {years.length > 0 ? (
              <Select
                value={form.fiscalYearId}
                onValueChange={(fiscalYearId) =>
                  setForm({ ...form, fiscalYearId })
                }
              >
                <SelectTrigger id="fiscal_year" className="w-full">
                  <SelectValue placeholder="Select a fiscal year" />
                </SelectTrigger>
                <SelectContent>
                  {years.map((fy) => (
                    <SelectItem key={fy.id} value={String(fy.id)}>
                      {fy.name} ({fy.start_date} → {fy.end_date})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <p className="text-sm text-muted-foreground">
                No fiscal years yet. Create a fiscal year first.
              </p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              placeholder="e.g. 2026-08"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="start_date">Start</Label>
              <Input
                id="start_date"
                type="date"
                required
                value={form.startDate}
                onChange={(e) =>
                  setForm({ ...form, startDate: e.target.value })
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="end_date">End</Label>
              <Input
                id="end_date"
                type="date"
                required
                value={form.endDate}
                onChange={(e) => setForm({ ...form, endDate: e.target.value })}
              />
            </div>
          </div>

          {!creating && (
            <div className="space-y-2">
              <Label htmlFor="status">Status</Label>
              <Select
                value={form.status}
                onValueChange={(status) => setForm({ ...form, status })}
              >
                <SelectTrigger id="status" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="open">open</SelectItem>
                  <SelectItem value="closed">closed</SelectItem>
                </SelectContent>
              </Select>
            </div>
          )}

          {saveError !== null && (
            <p className="text-sm text-destructive" role="alert">
              Failed to save: {saveError}
            </p>
          )}

          <div className="flex gap-2">
            <Button type="submit" disabled={saving || years.length === 0}>
              {saving ? "Saving…" : creating ? "Create" : "Save"}
            </Button>
            <Button type="button" variant="outline" asChild>
              <Link to="/periods">Cancel</Link>
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}
