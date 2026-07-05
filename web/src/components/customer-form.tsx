import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import {
  ApiError,
  createCustomer,
  getCustomer,
  listAccounts,
  listCustomers,
  listOrganizations,
  listPaymentTerms,
  listTaxCodes,
  updateCustomer,
  type Account,
  type CustomerInput,
  type Organization,
  type PaymentTerm,
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

// Form state holds everything the inputs need as strings; conversion back to
// CustomerInput happens on submit. organizationId is a select when creating and
// read-only when editing (a customer's organization is its identity, and the
// column is UNIQUE — one customer per organization).
interface FormState {
  organizationId: string
  customerNumber: string
  arAccountId: string
  paymentTermsCode: string
  currencyCode: string
  taxCode: string
  creditLimit: string
  isActive: boolean
}

const blankForm: FormState = {
  organizationId: "",
  customerNumber: "",
  arAccountId: NONE,
  paymentTermsCode: NONE,
  currencyCode: "",
  taxCode: NONE,
  creditLimit: "",
  isActive: true,
}

function emptyToNull(s: string): string | null {
  const t = s.trim()
  return t === "" ? null : t
}

export function CustomerForm({ mode }: { mode: Mode }) {
  const { id } = useParams()
  const customerId = Number(id)
  const navigate = useNavigate()

  const [form, setForm] = useState<FormState | null>(null)
  // Organizations selectable for this form: in create mode, only those without
  // an existing customer (the UNIQUE constraint); in edit mode, just the
  // customer's own organization (shown read-only).
  const [orgOptions, setOrgOptions] = useState<Organization[]>([])
  const [accounts, setAccounts] = useState<Account[]>([])
  const [taxCodes, setTaxCodes] = useState<TaxCode[]>([])
  const [paymentTerms, setPaymentTerms] = useState<PaymentTerm[]>([])
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      try {
        if (mode === "edit") {
          if (!Number.isInteger(customerId) || customerId <= 0) {
            setError("Invalid customer id.")
            return
          }
          const [customer, organizations, accts, taxes, terms] =
            await Promise.all([
              getCustomer(customerId),
              listOrganizations(),
              listAccounts(),
              listTaxCodes(),
              listPaymentTerms(),
            ])
          if (cancelled) return
          const org = organizations.find((o) => o.id === customer.organization_id)
          setOrgOptions(org ? [org] : [])
          setAccounts(accts)
          setTaxCodes(taxes)
          setPaymentTerms(terms)
          setForm({
            organizationId: String(customer.organization_id),
            customerNumber: customer.customer_number ?? "",
            arAccountId:
              customer.ar_account_id != null
                ? String(customer.ar_account_id)
                : NONE,
            paymentTermsCode: customer.payment_terms_code ?? NONE,
            currencyCode: customer.currency_code ?? "",
            taxCode: customer.tax_code ?? NONE,
            creditLimit: customer.credit_limit ?? "",
            isActive: customer.is_active,
          })
        } else {
          const [organizations, customers, accts, taxes, terms] =
            await Promise.all([
              listOrganizations(),
              listCustomers(),
              listAccounts(),
              listTaxCodes(),
              listPaymentTerms(),
            ])
          if (cancelled) return
          const taken = new Set(customers.map((c) => c.organization_id))
          setOrgOptions(organizations.filter((o) => !taken.has(o.id)))
          setAccounts(accts)
          setTaxCodes(taxes)
          setPaymentTerms(terms)
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
  }, [mode, customerId])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form) return
    if (mode === "create" && form.organizationId === "") {
      setSaveError("Please choose an organization.")
      return
    }
    const currency = form.currencyCode.trim().toUpperCase()
    const input: CustomerInput = {
      organization_id: Number(form.organizationId),
      customer_number: emptyToNull(form.customerNumber),
      ar_account_id: form.arAccountId === NONE ? null : Number(form.arAccountId),
      payment_terms_code:
        form.paymentTermsCode === NONE ? null : form.paymentTermsCode,
      currency_code: currency === "" ? null : currency,
      tax_code: form.taxCode === NONE ? null : form.taxCode,
      credit_limit: emptyToNull(form.creditLimit),
      is_active: form.isActive,
    }
    setSaving(true)
    setSaveError(null)
    const action =
      mode === "create"
        ? createCustomer(input).then(() => undefined)
        : updateCustomer(customerId, input)
    action
      .then(() => navigate("/customers"))
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
          {creating ? "New Customer" : "Edit Customer"}
        </h1>
        <p className="text-sm text-muted-foreground">
          {creating
            ? "Give an organization a customer role."
            : "Update the customer's terms and accounts."}
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
                  Every organization already has a customer. Create an
                  organization first.
                </p>
              )
            ) : (
              <>
                <p className="text-sm font-medium">{orgName}</p>
                <p className="text-xs text-muted-foreground">
                  A customer's organization can't be reassigned here.
                </p>
              </>
            )}
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
              <Select
                value={form.paymentTermsCode}
                onValueChange={(v) => setForm({ ...form, paymentTermsCode: v })}
              >
                <SelectTrigger id="payment_terms" className="w-full">
                  <SelectValue placeholder="None" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NONE}>None</SelectItem>
                  {paymentTerms.map((t) => (
                    <SelectItem key={t.code} value={t.code}>
                      {t.code} — {t.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
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
            <Button
              type="submit"
              disabled={saving || (creating && orgOptions.length === 0)}
            >
              {saving ? "Saving…" : creating ? "Create" : "Save"}
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
