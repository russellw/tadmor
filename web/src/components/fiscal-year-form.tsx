import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import {
  ApiError,
  createFiscalYear,
  getFiscalYear,
  listFiscalYears,
  updateFiscalYear,
} from "@/lib/api"
import { dayAfter, endOfYearFrom } from "@/lib/dates"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

type Mode = "create" | "edit"

// No status field: open/closed is owned by the year-end close workflow on the
// Periods screen, not by edits here.
interface FormState {
  name: string
  startDate: string
  endDate: string
}

// In create mode the form prefills the year after the latest existing fiscal
// year (or the current calendar year when there are none), since "add the next
// year" is what this form is almost always opened for.
function nextYearDefaults(latestEnd: string | undefined): FormState {
  const start =
    latestEnd !== undefined
      ? dayAfter(latestEnd)
      : `${new Date().getFullYear()}-01-01`
  return {
    name: `FY${start.slice(0, 4)}`,
    startDate: start,
    endDate: endOfYearFrom(start),
  }
}

export function FiscalYearForm({ mode }: { mode: Mode }) {
  const { id } = useParams()
  const fiscalYearId = Number(id)
  const navigate = useNavigate()

  const [form, setForm] = useState<FormState | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      try {
        if (mode === "edit") {
          if (!Number.isInteger(fiscalYearId) || fiscalYearId <= 0) {
            setError("Invalid fiscal year id.")
            return
          }
          const fy = await getFiscalYear(fiscalYearId)
          if (cancelled) return
          setForm({
            name: fy.name,
            startDate: fy.start_date,
            endDate: fy.end_date,
          })
        } else {
          // Years come back ordered by start_date, so the last is the latest.
          const years = await listFiscalYears()
          if (cancelled) return
          setForm(nextYearDefaults(years.at(-1)?.end_date))
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
  }, [mode, fiscalYearId])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form) return
    if (form.name.trim() === "") {
      setSaveError("Name is required.")
      return
    }
    const input = {
      name: form.name.trim(),
      start_date: form.startDate,
      end_date: form.endDate,
    }
    setSaving(true)
    setSaveError(null)
    const action =
      mode === "create"
        ? createFiscalYear(input).then(() => undefined)
        : updateFiscalYear(fiscalYearId, input)
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
          {creating ? "New Fiscal Year" : "Edit Fiscal Year"}
        </h1>
        <p className="text-sm text-muted-foreground">
          {creating
            ? "Add a reporting year, then create its accounting periods."
            : "Update the fiscal year's details."}
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
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              placeholder="e.g. FY2027"
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

          {saveError !== null && (
            <p className="text-sm text-destructive" role="alert">
              Failed to save: {saveError}
            </p>
          )}

          <div className="flex gap-2">
            <Button type="submit" disabled={saving}>
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
