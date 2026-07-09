import { useEffect, useState, type FormEvent } from "react"
import { Link } from "react-router-dom"

import {
  ApiError,
  getSettings,
  listAccounts,
  updateSettings,
  type Account,
  type Settings,
} from "@/lib/api"
import { useCurrentUser } from "@/lib/current-user"
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

// Radix Select items cannot carry an empty value, so "no account" is a
// sentinel (same convention as the tax-code and product forms).
const NONE = "__none__"

// Ledger settings: the base (functional) currency every report is stated in,
// and the account realized exchange gains/losses post to. Editing is
// admin-only; the base currency is additionally frozen by the database once
// any journal entry exists, so a change is only accepted on an empty ledger.
export function SettingsScreen() {
  const currentUser = useCurrentUser()

  const [baseCurrency, setBaseCurrency] = useState("")
  const [fxAccountId, setFxAccountId] = useState(NONE)
  const [accounts, setAccounts] = useState<Account[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loaded, setLoaded] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    let cancelled = false
    Promise.all([getSettings(), listAccounts()])
      .then(([s, accts]) => {
        if (cancelled) return
        setBaseCurrency(s.base_currency)
        setFxAccountId(
          s.fx_gain_loss_account_id != null
            ? String(s.fx_gain_loss_account_id)
            : NONE,
        )
        setAccounts(accts)
        setLoaded(true)
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

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (baseCurrency.trim().length !== 3) {
      setSaveError("Base currency must be a 3-letter code.")
      return
    }
    const input: Settings = {
      base_currency: baseCurrency.trim().toUpperCase(),
      fx_gain_loss_account_id: fxAccountId === NONE ? null : Number(fxAccountId),
    }
    setSaving(true)
    setSaveError(null)
    setSaved(false)
    updateSettings(input)
      .then(() => {
        setSaving(false)
        setSaved(true)
      })
      .catch((err: unknown) => {
        setSaving(false)
        setSaveError(err instanceof ApiError ? err.message : String(err))
      })
  }

  const readOnly = !currentUser.is_admin

  return (
    <section className="mx-auto w-full max-w-2xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">
          The ledger's base currency and its exchange gain/loss account.
        </p>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load settings: {error}
        </p>
      )}

      {error === null && !loaded && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {loaded && (
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="base_currency">Base Currency</Label>
            <Input
              id="base_currency"
              maxLength={3}
              className="max-w-40"
              disabled={readOnly}
              value={baseCurrency}
              onChange={(e) => setBaseCurrency(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              The functional currency every report is stated in. It can only be
              changed while the ledger has no journal entries.
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="fx_account">Exchange Gain/Loss Account</Label>
            <Select
              value={fxAccountId}
              onValueChange={setFxAccountId}
              disabled={readOnly}
            >
              <SelectTrigger id="fx_account" className="w-full">
                <SelectValue placeholder="None" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NONE}>None</SelectItem>
                {accounts.map((a) => (
                  <SelectItem key={a.id} value={String(a.id)}>
                    {a.code} — {a.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              Where realized exchange differences post when a payment or credit
              note settles a document booked at a different rate.
            </p>
          </div>

          {readOnly && (
            <p className="text-sm text-muted-foreground">
              Only administrators can change these settings.
            </p>
          )}

          {saveError !== null && (
            <p className="text-sm text-destructive" role="alert">
              Failed to save: {saveError}
            </p>
          )}

          {saved && (
            <p className="text-sm text-muted-foreground" role="status">
              Settings saved.
            </p>
          )}

          {!readOnly && (
            <div className="flex gap-2">
              <Button type="submit" disabled={saving}>
                {saving ? "Saving…" : "Save"}
              </Button>
              <Button type="button" variant="outline" asChild>
                <Link to="/">Cancel</Link>
              </Button>
            </div>
          )}
        </form>
      )}
    </section>
  )
}
