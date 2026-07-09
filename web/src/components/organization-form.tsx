import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import {
  ApiError,
  createOrganization,
  getOrganization,
  updateOrganization,
  type OrganizationInput,
} from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

type Mode = "create" | "edit"

// Organizations are standalone (their own name), so the form needs no joins.
// country_code and default_currency are free-text codes, matching the currency
// inputs on the customer/supplier forms (there are no reference endpoints).
interface FormState {
  name: string
  legalName: string
  taxId: string
  countryCode: string
  defaultCurrency: string
  isSelf: boolean
}

const blankForm: FormState = {
  name: "",
  legalName: "",
  taxId: "",
  countryCode: "",
  defaultCurrency: "",
  isSelf: false,
}

function emptyToNull(s: string): string | null {
  const t = s.trim()
  return t === "" ? null : t
}

export function OrganizationForm({ mode }: { mode: Mode }) {
  const { id } = useParams()
  const organizationId = Number(id)
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
          if (!Number.isInteger(organizationId) || organizationId <= 0) {
            setError("Invalid organization id.")
            return
          }
          const org = await getOrganization(organizationId)
          if (cancelled) return
          setForm({
            name: org.name,
            legalName: org.legal_name ?? "",
            taxId: org.tax_id ?? "",
            countryCode: org.country_code ?? "",
            defaultCurrency: org.default_currency ?? "",
            isSelf: org.is_self,
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
  }, [mode, organizationId])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form) return
    if (form.name.trim() === "") {
      setSaveError("Name is required.")
      return
    }
    const country = form.countryCode.trim().toUpperCase()
    const currency = form.defaultCurrency.trim().toUpperCase()
    const input: OrganizationInput = {
      name: form.name.trim(),
      legal_name: emptyToNull(form.legalName),
      tax_id: emptyToNull(form.taxId),
      country_code: country === "" ? null : country,
      default_currency: currency === "" ? null : currency,
      is_self: form.isSelf,
    }
    setSaving(true)
    setSaveError(null)
    const action =
      mode === "create"
        ? createOrganization(input).then(() => undefined)
        : updateOrganization(organizationId, input)
    action
      .then(() => navigate("/organizations"))
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
          {creating ? "New Organization" : "Edit Organization"}
        </h1>
        <p className="text-sm text-muted-foreground">
          {creating
            ? "Add an organization that customers or suppliers can attach to."
            : "Update the organization's details."}
        </p>
      </header>

      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/organizations">Back to organizations</Link>
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
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="legal_name">Legal Name</Label>
            <Input
              id="legal_name"
              placeholder="Registered name, if different"
              value={form.legalName}
              onChange={(e) => setForm({ ...form, legalName: e.target.value })}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="tax_id">Tax ID</Label>
            <Input
              id="tax_id"
              placeholder="VAT/EIN/etc."
              value={form.taxId}
              onChange={(e) => setForm({ ...form, taxId: e.target.value })}
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="country_code">Country</Label>
              <Input
                id="country_code"
                maxLength={2}
                placeholder="e.g. US"
                value={form.countryCode}
                onChange={(e) =>
                  setForm({ ...form, countryCode: e.target.value })
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="default_currency">Default Currency</Label>
              <Input
                id="default_currency"
                maxLength={3}
                placeholder="e.g. USD"
                value={form.defaultCurrency}
                onChange={(e) =>
                  setForm({ ...form, defaultCurrency: e.target.value })
                }
              />
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Checkbox
              id="is_self"
              checked={form.isSelf}
              onCheckedChange={(c) => setForm({ ...form, isSelf: c === true })}
            />
            <Label htmlFor="is_self">
              This is our own company (its name, tax ID, and address appear as
              the issuer on printed documents; only one organization can be it)
            </Label>
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
              <Link to="/organizations">Cancel</Link>
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}
