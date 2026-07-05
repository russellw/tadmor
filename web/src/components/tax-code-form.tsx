import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import {
  ApiError,
  createTaxCode,
  getTaxCode,
  listAccounts,
  updateTaxCode,
  type Account,
  type TaxCodeInput,
} from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
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

// Radix Select items cannot carry an empty value, so "no account" is a
// sentinel (same convention as the product form).
const NONE = "__none__"

interface FormState {
  code: string
  name: string
  rate: string
  taxAccountId: string
  isActive: boolean
}

const blankForm: FormState = {
  code: "",
  name: "",
  rate: "0",
  taxAccountId: NONE,
  isActive: true,
}

export function TaxCodeForm({ mode }: { mode: Mode }) {
  const { code } = useParams()
  const taxCode = code ?? ""
  const navigate = useNavigate()

  const [form, setForm] = useState<FormState | null>(null)
  const [accounts, setAccounts] = useState<Account[]>([])
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      try {
        const accts = await listAccounts()
        if (cancelled) return
        setAccounts(accts)

        if (mode === "edit") {
          if (taxCode === "") {
            setError("Invalid tax code.")
            return
          }
          const t = await getTaxCode(taxCode)
          if (cancelled) return
          setForm({
            code: t.code,
            name: t.name,
            rate: t.rate,
            taxAccountId:
              t.tax_account_id != null ? String(t.tax_account_id) : NONE,
            isActive: t.is_active,
          })
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
  }, [mode, taxCode])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form) return
    if (form.code.trim() === "" || form.name.trim() === "") {
      setSaveError("Code and name are required.")
      return
    }
    const input: TaxCodeInput = {
      code: form.code.trim().toUpperCase(),
      name: form.name.trim(),
      rate: form.rate.trim() === "" ? "0" : form.rate.trim(),
      tax_account_id:
        form.taxAccountId === NONE ? null : Number(form.taxAccountId),
      is_active: form.isActive,
    }
    setSaving(true)
    setSaveError(null)
    const action =
      mode === "create"
        ? createTaxCode(input).then(() => undefined)
        : updateTaxCode(taxCode, input)
    action
      .then(() => navigate("/tax-codes"))
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
          {creating ? "New Tax Code" : "Edit Tax Code"}
        </h1>
        <p className="text-sm text-muted-foreground">
          {creating
            ? "Add a tax rate that document lines can carry."
            : "Update the tax code's rate and account."}
        </p>
      </header>

      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/tax-codes">Back to tax codes</Link>
          </Button>
        </div>
      )}

      {error === null && form === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {form !== null && (
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="code">Code</Label>
            {creating ? (
              <Input
                id="code"
                placeholder="e.g. STD"
                value={form.code}
                onChange={(e) => setForm({ ...form, code: e.target.value })}
              />
            ) : (
              // The code is the row's identity (natural primary key), so it is
              // fixed after creation.
              <p className="font-mono text-sm">{form.code}</p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              placeholder="e.g. Standard sales tax"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="rate">Rate %</Label>
            <Input
              id="rate"
              inputMode="decimal"
              className="max-w-40"
              value={form.rate}
              onChange={(e) => setForm({ ...form, rate: e.target.value })}
            />
            <p className="text-xs text-muted-foreground">
              Percent, e.g. 8.25 for 8.25%.
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="tax_account">Tax Account</Label>
            <Select
              value={form.taxAccountId}
              onValueChange={(v) => setForm({ ...form, taxAccountId: v })}
            >
              <SelectTrigger id="tax_account" className="w-full">
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
              Where collected tax posts (typically a tax-payable liability
              account).
            </p>
          </div>

          <div className="flex items-center gap-2">
            <Checkbox
              id="is_active"
              checked={form.isActive}
              onCheckedChange={(c) => setForm({ ...form, isActive: c === true })}
            />
            <Label htmlFor="is_active">Active</Label>
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
              <Link to="/tax-codes">Cancel</Link>
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}
