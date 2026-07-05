import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import {
  ApiError,
  createPaymentTerm,
  getPaymentTerm,
  updatePaymentTerm,
  type PaymentTermInput,
} from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

type Mode = "create" | "edit"

interface FormState {
  code: string
  name: string
  dueDays: string
}

const blankForm: FormState = {
  code: "",
  name: "",
  dueDays: "0",
}

export function PaymentTermForm({ mode }: { mode: Mode }) {
  const { code } = useParams()
  const termCode = code ?? ""
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
          if (termCode === "") {
            setError("Invalid payment term code.")
            return
          }
          const t = await getPaymentTerm(termCode)
          if (cancelled) return
          setForm({
            code: t.code,
            name: t.name,
            dueDays: String(t.due_days),
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
  }, [mode, termCode])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form) return
    if (form.code.trim() === "" || form.name.trim() === "") {
      setSaveError("Code and name are required.")
      return
    }
    const dueDays = Number(form.dueDays.trim() === "" ? "0" : form.dueDays)
    if (!Number.isInteger(dueDays) || dueDays < 0) {
      setSaveError("Due days must be a whole number of days, 0 or more.")
      return
    }
    const input: PaymentTermInput = {
      code: form.code.trim().toUpperCase(),
      name: form.name.trim(),
      due_days: dueDays,
    }
    setSaving(true)
    setSaveError(null)
    const action =
      mode === "create"
        ? createPaymentTerm(input).then(() => undefined)
        : updatePaymentTerm(termCode, input)
    action
      .then(() => navigate("/payment-terms"))
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
          {creating ? "New Payment Term" : "Edit Payment Term"}
        </h1>
        <p className="text-sm text-muted-foreground">
          {creating
            ? "Add a payment term customers and suppliers can carry."
            : "Update the payment term's name and due days."}
        </p>
      </header>

      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/payment-terms">Back to payment terms</Link>
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
                placeholder="e.g. NET30"
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
              placeholder="e.g. Net 30"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="due_days">Due Days</Label>
            <Input
              id="due_days"
              inputMode="numeric"
              className="max-w-40"
              value={form.dueDays}
              onChange={(e) => setForm({ ...form, dueDays: e.target.value })}
            />
            <p className="text-xs text-muted-foreground">
              Days from the document date until payment is due; 0 means due on
              receipt.
            </p>
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
              <Link to="/payment-terms">Cancel</Link>
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}
