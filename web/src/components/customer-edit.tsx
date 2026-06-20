import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import {
  ApiError,
  getCustomer,
  listAccounts,
  listOrganizations,
  listTaxCodes,
  updateCustomer,
  type Account,
  type CustomerInput,
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

// Radix Select reserves "" as a value, so nullable selects use this sentinel.
const NONE = "__none__"

// Form state mirrors CustomerInput but holds everything the inputs need as
// strings; conversion back to CustomerInput happens on submit. organizationId is
// kept but not editable — a customer's organization is its identity here.
interface FormState {
  organizationId: number
  organizationName: string
  customerNumber: string
  arAccountId: string
  paymentTermsCode: string
  currencyCode: string
  taxCode: string
  creditLimit: string
  isActive: boolean
}

function emptyToNull(s: string): string | null {
  const t = s.trim()
  return t === "" ? null : t
}

export function CustomerEdit() {
  const { id } = useParams()
  const customerId = Number(id)
  const navigate = useNavigate()

  const [form, setForm] = useState<FormState | null>(null)
  const [accounts, setAccounts] = useState<Account[]>([])
  const [taxCodes, setTaxCodes] = useState<TaxCode[]>([])
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    if (!Number.isInteger(customerId) || customerId <= 0) {
      setError("Invalid customer id.")
      return
    }
    let cancelled = false
    Promise.all([
      getCustomer(customerId),
      listOrganizations(),
      listAccounts(),
      listTaxCodes(),
    ])
      .then(([customer, organizations, accts, taxes]) => {
        if (cancelled) return
        const org = organizations.find((o) => o.id === customer.organization_id)
        setAccounts(accts)
        setTaxCodes(taxes)
        setForm({
          organizationId: customer.organization_id,
          organizationName: org?.name ?? `#${customer.organization_id}`,
          customerNumber: customer.customer_number ?? "",
          arAccountId:
            customer.ar_account_id != null
              ? String(customer.ar_account_id)
              : NONE,
          paymentTermsCode: customer.payment_terms_code ?? "",
          currencyCode: customer.currency_code ?? "",
          taxCode: customer.tax_code ?? NONE,
          creditLimit: customer.credit_limit ?? "",
          isActive: customer.is_active,
        })
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [customerId])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form) return
    const currency = form.currencyCode.trim().toUpperCase()
    const input: CustomerInput = {
      organization_id: form.organizationId,
      customer_number: emptyToNull(form.customerNumber),
      ar_account_id: form.arAccountId === NONE ? null : Number(form.arAccountId),
      payment_terms_code: emptyToNull(form.paymentTermsCode),
      currency_code: currency === "" ? null : currency,
      tax_code: form.taxCode === NONE ? null : form.taxCode,
      credit_limit: emptyToNull(form.creditLimit),
      is_active: form.isActive,
    }
    setSaving(true)
    setSaveError(null)
    updateCustomer(customerId, input)
      .then(() => navigate("/customers"))
      .catch((err: unknown) => {
        setSaving(false)
        setSaveError(err instanceof ApiError ? err.message : String(err))
      })
  }

  return (
    <section className="mx-auto w-full max-w-2xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">Edit Customer</h1>
        <p className="text-sm text-muted-foreground">
          Update the customer's terms and accounts.
        </p>
      </header>

      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/customers">Back to customers</Link>
          </Button>
        </div>
      )}

      {error === null && form === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {form !== null && (
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="space-y-1.5">
            <Label>Organization</Label>
            <p className="text-sm font-medium">{form.organizationName}</p>
            <p className="text-xs text-muted-foreground">
              A customer's organization can't be reassigned here.
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="customer_number">Customer #</Label>
            <Input
              id="customer_number"
              value={form.customerNumber}
              onChange={(e) =>
                setForm({ ...form, customerNumber: e.target.value })
              }
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="ar_account">AR Account</Label>
            <Select
              value={form.arAccountId}
              onValueChange={(v) => setForm({ ...form, arAccountId: v })}
            >
              <SelectTrigger id="ar_account" className="w-full">
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

          <div className="space-y-2">
            <Label htmlFor="credit_limit">Credit Limit</Label>
            <Input
              id="credit_limit"
              inputMode="decimal"
              value={form.creditLimit}
              onChange={(e) =>
                setForm({ ...form, creditLimit: e.target.value })
              }
            />
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
              {saving ? "Saving…" : "Save"}
            </Button>
            <Button type="button" variant="outline" asChild>
              <Link to="/customers">Cancel</Link>
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}
