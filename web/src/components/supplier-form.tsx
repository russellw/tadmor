import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import {
  ApiError,
  createSupplier,
  getSupplier,
  listAccounts,
  listOrganizations,
  listSuppliers,
  listTaxCodes,
  updateSupplier,
  type Account,
  type Organization,
  type SupplierInput,
  type TaxCode,
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

// Radix Select reserves "" as a value, so nullable selects use this sentinel.
const NONE = "__none__"

// Mirrors customer-form.tsx: a supplier is a role on an organization, the column
// is UNIQUE (one supplier per organization), so organizationId is a select when
// creating and read-only when editing.
interface FormState {
  organizationId: string
  supplierNumber: string
  apAccountId: string
  paymentTermsCode: string
  currencyCode: string
  taxCode: string
  isActive: boolean
}

const blankForm: FormState = {
  organizationId: "",
  supplierNumber: "",
  apAccountId: NONE,
  paymentTermsCode: "",
  currencyCode: "",
  taxCode: NONE,
  isActive: true,
}

function emptyToNull(s: string): string | null {
  const t = s.trim()
  return t === "" ? null : t
}

export function SupplierForm({ mode }: { mode: Mode }) {
  const { id } = useParams()
  const supplierId = Number(id)
  const navigate = useNavigate()

  const [form, setForm] = useState<FormState | null>(null)
  // In create mode, only organizations without an existing supplier (the UNIQUE
  // constraint); in edit mode, just the supplier's own organization (read-only).
  const [orgOptions, setOrgOptions] = useState<Organization[]>([])
  const [accounts, setAccounts] = useState<Account[]>([])
  const [taxCodes, setTaxCodes] = useState<TaxCode[]>([])
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      try {
        if (mode === "edit") {
          if (!Number.isInteger(supplierId) || supplierId <= 0) {
            setError("Invalid supplier id.")
            return
          }
          const [supplier, organizations, accts, taxes] = await Promise.all([
            getSupplier(supplierId),
            listOrganizations(),
            listAccounts(),
            listTaxCodes(),
          ])
          if (cancelled) return
          const org = organizations.find((o) => o.id === supplier.organization_id)
          setOrgOptions(org ? [org] : [])
          setAccounts(accts)
          setTaxCodes(taxes)
          setForm({
            organizationId: String(supplier.organization_id),
            supplierNumber: supplier.supplier_number ?? "",
            apAccountId:
              supplier.ap_account_id != null
                ? String(supplier.ap_account_id)
                : NONE,
            paymentTermsCode: supplier.payment_terms_code ?? "",
            currencyCode: supplier.currency_code ?? "",
            taxCode: supplier.tax_code ?? NONE,
            isActive: supplier.is_active,
          })
        } else {
          const [organizations, suppliers, accts, taxes] = await Promise.all([
            listOrganizations(),
            listSuppliers(),
            listAccounts(),
            listTaxCodes(),
          ])
          if (cancelled) return
          const taken = new Set(suppliers.map((s) => s.organization_id))
          setOrgOptions(organizations.filter((o) => !taken.has(o.id)))
          setAccounts(accts)
          setTaxCodes(taxes)
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
  }, [mode, supplierId])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form) return
    if (mode === "create" && form.organizationId === "") {
      setSaveError("Please choose an organization.")
      return
    }
    const currency = form.currencyCode.trim().toUpperCase()
    const input: SupplierInput = {
      organization_id: Number(form.organizationId),
      supplier_number: emptyToNull(form.supplierNumber),
      ap_account_id: form.apAccountId === NONE ? null : Number(form.apAccountId),
      payment_terms_code: emptyToNull(form.paymentTermsCode),
      currency_code: currency === "" ? null : currency,
      tax_code: form.taxCode === NONE ? null : form.taxCode,
      is_active: form.isActive,
    }
    setSaving(true)
    setSaveError(null)
    const action =
      mode === "create"
        ? createSupplier(input).then(() => undefined)
        : updateSupplier(supplierId, input)
    action
      .then(() => navigate("/suppliers"))
      .catch((err: unknown) => {
        setSaving(false)
        setSaveError(err instanceof ApiError ? err.message : String(err))
      })
  }

  const creating = mode === "create"
  const orgName =
    orgOptions.find((o) => String(o.id) === form?.organizationId)?.name ??
    (form ? `#${form.organizationId}` : "")

  return (
    <section className="mx-auto w-full max-w-2xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">
          {creating ? "New Supplier" : "Edit Supplier"}
        </h1>
        <p className="text-sm text-muted-foreground">
          {creating
            ? "Give an organization a supplier role."
            : "Update the supplier's terms and accounts."}
        </p>
      </header>

      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/suppliers">Back to suppliers</Link>
          </Button>
        </div>
      )}

      {error === null && form === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {form !== null && (
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="organization">Organization</Label>
            {creating ? (
              orgOptions.length > 0 ? (
                <Select
                  value={form.organizationId}
                  onValueChange={(v) =>
                    setForm({ ...form, organizationId: v })
                  }
                >
                  <SelectTrigger id="organization" className="w-full">
                    <SelectValue placeholder="Select an organization" />
                  </SelectTrigger>
                  <SelectContent>
                    {orgOptions.map((o) => (
                      <SelectItem key={o.id} value={String(o.id)}>
                        {o.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <p className="text-sm text-muted-foreground">
                  Every organization already has a supplier. Create an
                  organization first.
                </p>
              )
            ) : (
              <>
                <p className="text-sm font-medium">{orgName}</p>
                <p className="text-xs text-muted-foreground">
                  A supplier's organization can't be reassigned here.
                </p>
              </>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="supplier_number">Supplier #</Label>
            <Input
              id="supplier_number"
              value={form.supplierNumber}
              onChange={(e) =>
                setForm({ ...form, supplierNumber: e.target.value })
              }
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="ap_account">AP Account</Label>
            <Select
              value={form.apAccountId}
              onValueChange={(v) => setForm({ ...form, apAccountId: v })}
            >
              <SelectTrigger id="ap_account" className="w-full">
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
          </div>

          <div className="space-y-2">
            <Label htmlFor="tax_code">Tax Code</Label>
            <Select
              value={form.taxCode}
              onValueChange={(v) => setForm({ ...form, taxCode: v })}
            >
              <SelectTrigger id="tax_code" className="w-full">
                <SelectValue placeholder="None" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NONE}>None</SelectItem>
                {taxCodes.map((t) => (
                  <SelectItem key={t.code} value={t.code}>
                    {t.code} — {t.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="payment_terms">Payment Terms</Label>
              <Input
                id="payment_terms"
                value={form.paymentTermsCode}
                onChange={(e) =>
                  setForm({ ...form, paymentTermsCode: e.target.value })
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="currency">Currency</Label>
              <Input
                id="currency"
                maxLength={3}
                placeholder="e.g. USD"
                value={form.currencyCode}
                onChange={(e) =>
                  setForm({ ...form, currencyCode: e.target.value })
                }
              />
            </div>
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
            <Button
              type="submit"
              disabled={saving || (creating && orgOptions.length === 0)}
            >
              {saving ? "Saving…" : creating ? "Create" : "Save"}
            </Button>
            <Button type="button" variant="outline" asChild>
              <Link to="/suppliers">Cancel</Link>
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}
