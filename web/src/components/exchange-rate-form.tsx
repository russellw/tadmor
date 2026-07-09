import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import {
  ApiError,
  createExchangeRate,
  deleteExchangeRate,
  getSettings,
  listExchangeRates,
  updateExchangeRate,
  type ExchangeRateInput,
} from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

type Mode = "create" | "edit"

interface FormState {
  currency: string
  date: string
  rate: string
}

const blankForm: FormState = { currency: "", date: "", rate: "" }

// Create or edit one exchange rate. The currency + date are the natural key,
// so on edit they are fixed and only the rate itself changes.
export function ExchangeRateForm({ mode }: { mode: Mode }) {
  const params = useParams()
  const currency = params.currency ?? ""
  const date = params.date ?? ""
  const navigate = useNavigate()

  const [form, setForm] = useState<FormState | null>(null)
  const [baseCurrency, setBaseCurrency] = useState<string>("")
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      try {
        const settings = await getSettings()
        if (cancelled) return
        setBaseCurrency(settings.base_currency)

        if (mode === "edit") {
          if (currency === "" || date === "") {
            setError("Invalid exchange rate.")
            return
          }
          // There is no single-rate GET; find it in the list (small table).
          const rates = await listExchangeRates()
          if (cancelled) return
          const r = rates.find(
            (x) => x.currency_code === currency && x.rate_date === date,
          )
          if (r === undefined) {
            setError("Exchange rate not found.")
            return
          }
          setForm({ currency: r.currency_code, date: r.rate_date, rate: r.rate })
        } else {
          setForm(blankForm)
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
  }, [mode, currency, date])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form) return
    if (
      form.currency.trim() === "" ||
      form.date.trim() === "" ||
      form.rate.trim() === ""
    ) {
      setSaveError("Currency, date, and rate are required.")
      return
    }
    const input: ExchangeRateInput = {
      currency_code: form.currency.trim().toUpperCase(),
      rate_date: form.date.trim(),
      rate: form.rate.trim(),
    }
    setSaving(true)
    setSaveError(null)
    const action =
      mode === "create"
        ? createExchangeRate(input).then(() => undefined)
        : updateExchangeRate(currency, date, input)
    action
      .then(() => navigate("/exchange-rates"))
      .catch((err: unknown) => {
        setSaving(false)
        setSaveError(err instanceof ApiError ? err.message : String(err))
      })
  }

  function handleDelete() {
    if (!window.confirm("Delete this exchange rate?")) return
    setSaving(true)
    setSaveError(null)
    deleteExchangeRate(currency, date)
      .then(() => navigate("/exchange-rates"))
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
          {creating ? "New Exchange Rate" : "Edit Exchange Rate"}
        </h1>
        <p className="text-sm text-muted-foreground">
          {baseCurrency !== ""
            ? `How many ${baseCurrency} one unit of the currency buys on the date.`
            : "How many base-currency units one unit of the currency buys on the date."}
        </p>
      </header>

      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/exchange-rates">Back to exchange rates</Link>
          </Button>
        </div>
      )}

      {error === null && form === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {form !== null && (
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="currency">Currency</Label>
            {creating ? (
              <Input
                id="currency"
                maxLength={3}
                placeholder="e.g. EUR"
                className="max-w-40"
                value={form.currency}
                onChange={(e) => setForm({ ...form, currency: e.target.value })}
              />
            ) : (
              <p className="font-mono text-sm">{form.currency}</p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="date">Date</Label>
            {creating ? (
              <Input
                id="date"
                type="date"
                className="w-44"
                value={form.date}
                onChange={(e) => setForm({ ...form, date: e.target.value })}
              />
            ) : (
              <p className="font-mono text-sm">{form.date}</p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="rate">
              Rate{baseCurrency !== "" && ` (→ ${baseCurrency})`}
            </Label>
            <Input
              id="rate"
              inputMode="decimal"
              className="max-w-40"
              placeholder="e.g. 1.08"
              value={form.rate}
              onChange={(e) => setForm({ ...form, rate: e.target.value })}
            />
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
              <Link to="/exchange-rates">Cancel</Link>
            </Button>
            {!creating && (
              <Button
                type="button"
                variant="destructive"
                className="ml-auto"
                disabled={saving}
                onClick={handleDelete}
              >
                Delete
              </Button>
            )}
          </div>
        </form>
      )}
    </section>
  )
}
