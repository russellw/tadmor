import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import { trimAmount } from "@/lib/amount"
import {
  ApiError,
  createCustomerPayment,
  createSupplierPayment,
  getCustomerPayment,
  getSupplierPayment,
  listAccounts,
  listCustomers,
  listOrganizations,
  listSuppliers,
  PAYMENT_METHODS,
  updateCustomerPayment,
  updateSupplierPayment,
  type Account,
  type Payment,
} from "@/lib/api"
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

// Radix Select reserves "" as a value, so nullable selects use this sentinel.
const NONE = "__none__"

interface PartyOption {
  id: number
  name: string
  currency: string | null
}

// The common fields of both payment kinds; the wrappers below map them onto
// the customer/supplier input shapes.
interface PaymentFields {
  partyId: number
  date: string
  currency: string
  amount: string
  method: string | null
  reference: string | null
  accountId: number | null
}

async function customerOptions(): Promise<PartyOption[]> {
  const [customers, orgs] = await Promise.all([
    listCustomers(),
    listOrganizations(),
  ])
  const orgNames = new Map(orgs.map((o) => [o.id, o.name]))
  return customers
    .filter((c) => c.is_active)
    .map((c) => ({
      id: c.id,
      name: orgNames.get(c.organization_id) ?? `#${c.id}`,
      currency: c.currency_code,
    }))
}

async function supplierOptions(): Promise<PartyOption[]> {
  const [suppliers, orgs] = await Promise.all([
    listSuppliers(),
    listOrganizations(),
  ])
  const orgNames = new Map(orgs.map((o) => [o.id, o.name]))
  return suppliers
    .filter((s) => s.is_active)
    .map((s) => ({
      id: s.id,
      name: orgNames.get(s.organization_id) ?? `#${s.id}`,
      currency: s.currency_code,
    }))
}

// Save maps the common fields onto the customer/supplier input shape, creating
// when editId is null and updating otherwise; it resolves to the payment id.
function saveCustomerFields(
  editId: number | null,
  f: PaymentFields,
): Promise<number> {
  const input = {
    customer_id: f.partyId,
    payment_date: f.date,
    currency_code: f.currency,
    amount: f.amount,
    method: f.method,
    reference: f.reference,
    deposit_account_id: f.accountId,
  }
  return editId !== null
    ? updateCustomerPayment(editId, input).then(() => editId)
    : createCustomerPayment(input).then(({ id }) => id)
}

function saveSupplierFields(
  editId: number | null,
  f: PaymentFields,
): Promise<number> {
  const input = {
    supplier_id: f.partyId,
    payment_date: f.date,
    currency_code: f.currency,
    amount: f.amount,
    method: f.method,
    reference: f.reference,
    payment_account_id: f.accountId,
  }
  return editId !== null
    ? updateSupplierPayment(editId, input).then(() => editId)
    : createSupplierPayment(input).then(({ id }) => id)
}

export function CustomerPaymentForm({
  mode = "create",
}: {
  mode?: "create" | "edit"
}) {
  return (
    <PaymentForm
      mode={mode}
      title={mode === "edit" ? "Edit Customer Payment" : "New Customer Payment"}
      description="Records money received — post it to the ledger from the payment page, then apply it to open invoices."
      partyLabel="Customer"
      accountLabel="Deposit Account"
      basePath="/customer-payments"
      fetchParties={customerOptions}
      fetchPayment={getCustomerPayment}
      save={saveCustomerFields}
    />
  )
}

export function SupplierPaymentForm({
  mode = "create",
}: {
  mode?: "create" | "edit"
}) {
  return (
    <PaymentForm
      mode={mode}
      title={mode === "edit" ? "Edit Supplier Payment" : "New Supplier Payment"}
      description="Records money paid out — post it to the ledger from the payment page, then apply it to open bills."
      partyLabel="Supplier"
      accountLabel="Payment Account"
      basePath="/supplier-payments"
      fetchParties={supplierOptions}
      fetchPayment={getSupplierPayment}
      save={saveSupplierFields}
    />
  )
}

function emptyToNull(s: string): string | null {
  const t = s.trim()
  return t === "" ? null : t
}

function PaymentForm({
  mode,
  title,
  description,
  partyLabel,
  accountLabel,
  basePath,
  fetchParties,
  fetchPayment,
  save,
}: {
  mode: "create" | "edit"
  title: string
  description: string
  partyLabel: string
  accountLabel: string
  basePath: string
  fetchParties: () => Promise<PartyOption[]>
  fetchPayment: (id: number) => Promise<Payment>
  save: (editId: number | null, fields: PaymentFields) => Promise<number>
}) {
  const navigate = useNavigate()
  const { id } = useParams()
  const editId = mode === "edit" ? Number(id) : null

  const [parties, setParties] = useState<PartyOption[]>([])
  const [accounts, setAccounts] = useState<Account[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loaded, setLoaded] = useState(false)

  const [partyId, setPartyId] = useState("")
  const [date, setDate] = useState(new Date().toISOString().slice(0, 10))
  const [amount, setAmount] = useState("")
  const [currency, setCurrency] = useState("")
  const [method, setMethod] = useState(NONE)
  const [reference, setReference] = useState("")
  const [accountId, setAccountId] = useState(NONE)

  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([
      fetchParties(),
      listAccounts(),
      editId !== null ? fetchPayment(editId) : null,
    ])
      .then(([partyOptions, accts, payment]) => {
        if (cancelled) return
        setParties(partyOptions)
        setAccounts(accts)
        if (payment !== null) {
          if (payment.status !== "draft") {
            setError("Only draft payments can be edited.")
            return
          }
          setPartyId(String(payment.party_id))
          setDate(payment.date)
          setAmount(trimAmount(payment.amount))
          setCurrency(payment.currency_code)
          setMethod(payment.method ?? NONE)
          setReference(payment.reference ?? "")
          setAccountId(
            payment.account_id !== null ? String(payment.account_id) : NONE,
          )
        }
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
  }, [fetchParties, fetchPayment, editId])

  function chooseParty(value: string) {
    setPartyId(value)
    const p = parties.find((p) => String(p.id) === value)
    if (p?.currency && currency.trim() === "") {
      setCurrency(p.currency)
    }
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (partyId === "") {
      setSaveError(`Please choose a ${partyLabel.toLowerCase()}.`)
      return
    }
    setSaving(true)
    setSaveError(null)
    save(editId, {
      partyId: Number(partyId),
      date,
      currency: currency.trim().toUpperCase(),
      amount: amount.trim(),
      method: method === NONE ? null : method,
      reference: emptyToNull(reference),
      accountId: accountId === NONE ? null : Number(accountId),
    })
      .then((docId) => navigate(`${basePath}/${docId}`))
      .catch((err: unknown) => {
        setSaving(false)
        setSaveError(err instanceof ApiError ? err.message : String(err))
      })
  }

  return (
    <section className="mx-auto w-full max-w-2xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        <p className="text-sm text-muted-foreground">{description}</p>
      </header>

      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to={basePath}>Back</Link>
          </Button>
        </div>
      )}

      {error === null && !loaded && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {error === null && loaded && (
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="party">{partyLabel}</Label>
            <Select value={partyId} onValueChange={chooseParty}>
              <SelectTrigger id="party" className="w-full">
                <SelectValue
                  placeholder={`Select a ${partyLabel.toLowerCase()}`}
                />
              </SelectTrigger>
              <SelectContent>
                {parties.map((p) => (
                  <SelectItem key={p.id} value={String(p.id)}>
                    {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="date">Payment Date</Label>
              <Input
                id="date"
                type="date"
                required
                value={date}
                onChange={(e) => setDate(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="amount">Amount</Label>
              <Input
                id="amount"
                inputMode="decimal"
                required
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="currency">Currency</Label>
              <Input
                id="currency"
                required
                maxLength={3}
                placeholder="e.g. USD"
                value={currency}
                onChange={(e) => setCurrency(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="method">Method</Label>
              <Select value={method} onValueChange={setMethod}>
                <SelectTrigger id="method" className="w-full">
                  <SelectValue placeholder="None" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NONE}>None</SelectItem>
                  {PAYMENT_METHODS.map((m) => (
                    <SelectItem key={m} value={m}>
                      {m}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="reference">Reference</Label>
            <Input
              id="reference"
              value={reference}
              onChange={(e) => setReference(e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="account">{accountLabel}</Label>
            <Select value={accountId} onValueChange={setAccountId}>
              <SelectTrigger id="account" className="w-full">
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
              Required before the payment can be posted.
            </p>
          </div>

          {saveError !== null && (
            <p className="text-sm text-destructive" role="alert">
              Failed to save: {saveError}
            </p>
          )}

          <div className="flex gap-2">
            <Button type="submit" disabled={saving}>
              {saving ? "Saving…" : editId !== null ? "Save changes" : "Create draft"}
            </Button>
            <Button type="button" variant="outline" asChild>
              <Link to={editId !== null ? `${basePath}/${editId}` : basePath}>
                Cancel
              </Link>
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}
