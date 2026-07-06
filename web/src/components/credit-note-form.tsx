import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate } from "react-router-dom"

import { formatAmount, lineAmounts, sumAmounts } from "@/lib/amount"
import {
  ApiError,
  createPurchaseCreditNote,
  createSalesCreditNote,
  listAccounts,
  listCustomers,
  listOrganizations,
  listProducts,
  listSuppliers,
  listTaxCodes,
  type Account,
  type Product,
  type TaxCode,
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

// Sales and purchase credit notes differ only in the party (customer vs
// supplier), the per-unit money field, and which account a line books to, so
// both forms render through one parameterized component — the same
// consolidation the list and detail screens use, applied at the form level.

interface LineState {
  productId: string // NONE = free-form line
  description: string
  quantity: string
  unitAmount: string
  taxCode: string
  taxRate: string
  accountId: string // NONE = fall back to the product's account
}

const blankLine: LineState = {
  productId: NONE,
  description: "",
  quantity: "1",
  unitAmount: "",
  taxCode: NONE,
  taxRate: "0",
  accountId: NONE,
}

interface PartyOption {
  id: number
  name: string
  currency: string | null
}

interface HeaderState {
  partyId: number
  number: string
  date: string
  currency: string
  reference: string | null
  memo: string | null
}

interface LineValues {
  product_id: number | null
  description: string
  quantity: string
  unit_amount: string
  account_id: number | null
  tax_code: string | null
  tax_rate: string
}

function emptyToNull(s: string): string | null {
  const t = s.trim()
  return t === "" ? null : t
}

// Preview of one line's money, mirroring the backend's empty-input defaults
// (quantity 1, amount 0, rate 0) so the on-screen total matches what the
// database will compute.
function previewLine(l: LineState) {
  return lineAmounts(
    l.quantity.trim() === "" ? "1" : l.quantity,
    l.unitAmount.trim() === "" ? "0" : l.unitAmount,
    l.taxRate.trim() === "" ? "0" : l.taxRate,
  )
}

async function fetchCustomerOptions(): Promise<PartyOption[]> {
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

async function fetchSupplierOptions(): Promise<PartyOption[]> {
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

export function CreditNoteForm() {
  return (
    <GenericCreditNoteForm
      title="New Credit Note"
      partyLabel="Customer"
      unitLabel="Unit Price"
      accountLabel="Revenue Account"
      basePath="/credit-notes"
      backLabel="Cancel"
      fetchParties={fetchCustomerOptions}
      // Picking a product prefills the line the way the invoice form does:
      // the credit usually reverses what was invoiced.
      prefillFromProduct={(p) => ({
        description: p.name,
        unitAmount: p.unit_price,
        accountId:
          p.revenue_account_id != null ? String(p.revenue_account_id) : NONE,
      })}
      create={(header, lines) =>
        createSalesCreditNote({
          credit_note_number: header.number,
          customer_id: header.partyId,
          credit_note_date: header.date,
          currency_code: header.currency,
          reference: header.reference,
          memo: header.memo,
          lines: lines.map((l) => ({
            product_id: l.product_id,
            description: l.description,
            quantity: l.quantity,
            unit_price: l.unit_amount,
            revenue_account_id: l.account_id,
            tax_code: l.tax_code,
            tax_rate: l.tax_rate,
          })),
        })
      }
    />
  )
}

export function SupplierCreditForm() {
  return (
    <GenericCreditNoteForm
      title="New Supplier Credit"
      partyLabel="Supplier"
      unitLabel="Unit Cost"
      accountLabel="Expense Account"
      basePath="/supplier-credits"
      backLabel="Cancel"
      fetchParties={fetchSupplierOptions}
      // Costs come from the supplier's document, so only the description and
      // tax treatment prefill — the same choice the bill form makes.
      prefillFromProduct={(p) => ({ description: p.name })}
      create={(header, lines) =>
        createPurchaseCreditNote({
          credit_note_number: header.number,
          supplier_id: header.partyId,
          credit_note_date: header.date,
          currency_code: header.currency,
          reference: header.reference,
          memo: header.memo,
          lines: lines.map((l) => ({
            product_id: l.product_id,
            description: l.description,
            quantity: l.quantity,
            unit_cost: l.unit_amount,
            expense_account_id: l.account_id,
            tax_code: l.tax_code,
            tax_rate: l.tax_rate,
          })),
        })
      }
    />
  )
}

function GenericCreditNoteForm({
  title,
  partyLabel,
  unitLabel,
  accountLabel,
  basePath,
  backLabel,
  fetchParties,
  prefillFromProduct,
  create,
}: {
  title: string
  partyLabel: string
  unitLabel: string
  accountLabel: string
  basePath: string
  backLabel: string
  fetchParties: () => Promise<PartyOption[]>
  prefillFromProduct: (p: Product) => Partial<LineState>
  create: (header: HeaderState, lines: LineValues[]) => Promise<{ id: number }>
}) {
  const navigate = useNavigate()

  const [parties, setParties] = useState<PartyOption[]>([])
  const [products, setProducts] = useState<Product[]>([])
  const [taxCodes, setTaxCodes] = useState<TaxCode[]>([])
  const [accounts, setAccounts] = useState<Account[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loaded, setLoaded] = useState(false)

  const [partyId, setPartyId] = useState("")
  const [number, setNumber] = useState("")
  const [date, setDate] = useState(new Date().toISOString().slice(0, 10))
  const [currencyCode, setCurrencyCode] = useState("")
  const [reference, setReference] = useState("")
  const [memo, setMemo] = useState("")
  const [lines, setLines] = useState<LineState[]>([blankLine])

  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([fetchParties(), listProducts(), listTaxCodes(), listAccounts()])
      .then(([partyOpts, prods, taxes, accts]) => {
        if (cancelled) return
        setParties(partyOpts)
        setProducts(prods.filter((p) => p.is_active))
        setTaxCodes(taxes)
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
    // fetchParties is a module-level function on both call sites; effect runs once.
  }, [fetchParties])

  function setLine(index: number, patch: Partial<LineState>) {
    setLines((ls) => ls.map((l, i) => (i === index ? { ...l, ...patch } : l)))
  }

  function chooseProduct(index: number, value: string) {
    const product = products.find((p) => String(p.id) === value)
    if (!product) {
      setLine(index, { productId: NONE })
      return
    }
    const rate = product.tax_code
      ? (taxCodes.find((t) => t.code === product.tax_code)?.rate ?? "0")
      : "0"
    setLine(index, {
      productId: value,
      taxCode: product.tax_code ?? NONE,
      taxRate: rate,
      ...prefillFromProduct(product),
    })
  }

  function chooseTaxCode(index: number, value: string) {
    const rate =
      value === NONE ? "0" : (taxCodes.find((t) => t.code === value)?.rate ?? "0")
    setLine(index, { taxCode: value, taxRate: rate })
  }

  function chooseParty(value: string) {
    setPartyId(value)
    const p = parties.find((p) => String(p.id) === value)
    if (p?.currency && currencyCode.trim() === "") {
      setCurrencyCode(p.currency)
    }
  }

  const previews = lines.map(previewLine)
  const totals = previews.every((p) => p !== null)
    ? {
        subtotal: sumAmounts(previews.map((p) => p!.subtotal)),
        tax: sumAmounts(previews.map((p) => p!.tax)),
        total: sumAmounts(previews.map((p) => p!.total)),
      }
    : null

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (partyId === "") {
      setSaveError(`Please choose a ${partyLabel.toLowerCase()}.`)
      return
    }
    const header: HeaderState = {
      partyId: Number(partyId),
      number: number.trim(),
      date,
      currency: currencyCode.trim().toUpperCase(),
      reference: emptyToNull(reference),
      memo: emptyToNull(memo),
    }
    const lineValues: LineValues[] = lines.map((l) => ({
      product_id: l.productId === NONE ? null : Number(l.productId),
      description: l.description.trim(),
      quantity: l.quantity.trim(),
      unit_amount: l.unitAmount.trim(),
      account_id: l.accountId === NONE ? null : Number(l.accountId),
      tax_code: l.taxCode === NONE ? null : l.taxCode,
      tax_rate: l.taxRate.trim(),
    }))
    setSaving(true)
    setSaveError(null)
    create(header, lineValues)
      .then(({ id }) => navigate(`${basePath}/${id}`))
      .catch((err: unknown) => {
        setSaving(false)
        setSaveError(err instanceof ApiError ? err.message : String(err))
      })
  }

  return (
    <section className="mx-auto w-full max-w-5xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        <p className="text-sm text-muted-foreground">
          Creates a draft — post it to the ledger from the credit note page,
          then apply it to open documents.
        </p>
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
        <form onSubmit={handleSubmit} className="space-y-6">
          <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
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
            <div className="space-y-2">
              <Label htmlFor="credit_note_number">Credit Note #</Label>
              <Input
                id="credit_note_number"
                required
                value={number}
                onChange={(e) => setNumber(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="credit_note_date">Date</Label>
              <Input
                id="credit_note_date"
                type="date"
                required
                value={date}
                onChange={(e) => setDate(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="currency">Currency</Label>
              <Input
                id="currency"
                required
                maxLength={3}
                placeholder="e.g. USD"
                value={currencyCode}
                onChange={(e) => setCurrencyCode(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="reference">Reference</Label>
              <Input
                id="reference"
                placeholder="e.g. the invoice credited"
                value={reference}
                onChange={(e) => setReference(e.target.value)}
              />
            </div>
            <div className="col-span-2 space-y-2">
              <Label htmlFor="memo">Memo</Label>
              <Input
                id="memo"
                value={memo}
                onChange={(e) => setMemo(e.target.value)}
              />
            </div>
          </div>

          <div className="space-y-3">
            <h2 className="text-sm font-semibold">Lines</h2>
            {lines.map((line, i) => (
              <div key={i} className="space-y-3 rounded-md border p-3">
                <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
                  <div className="space-y-1">
                    <Label htmlFor={`line_${i}_product`}>Product</Label>
                    <Select
                      value={line.productId}
                      onValueChange={(v) => chooseProduct(i, v)}
                    >
                      <SelectTrigger
                        id={`line_${i}_product`}
                        className="w-full"
                      >
                        <SelectValue placeholder="Free-form" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={NONE}>Free-form</SelectItem>
                        {products.map((p) => (
                          <SelectItem key={p.id} value={String(p.id)}>
                            {p.sku} — {p.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-1 md:col-span-2">
                    <Label htmlFor={`line_${i}_description`}>Description</Label>
                    <Input
                      id={`line_${i}_description`}
                      required
                      value={line.description}
                      onChange={(e) =>
                        setLine(i, { description: e.target.value })
                      }
                    />
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-3 md:grid-cols-5">
                  <div className="space-y-1">
                    <Label htmlFor={`line_${i}_quantity`}>Qty</Label>
                    <Input
                      id={`line_${i}_quantity`}
                      inputMode="decimal"
                      value={line.quantity}
                      onChange={(e) => setLine(i, { quantity: e.target.value })}
                    />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor={`line_${i}_unit_amount`}>{unitLabel}</Label>
                    <Input
                      id={`line_${i}_unit_amount`}
                      inputMode="decimal"
                      value={line.unitAmount}
                      onChange={(e) =>
                        setLine(i, { unitAmount: e.target.value })
                      }
                    />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor={`line_${i}_tax_code`}>Tax Code</Label>
                    <Select
                      value={line.taxCode}
                      onValueChange={(v) => chooseTaxCode(i, v)}
                    >
                      <SelectTrigger
                        id={`line_${i}_tax_code`}
                        className="w-full"
                      >
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
                  <div className="space-y-1">
                    <Label htmlFor={`line_${i}_tax_rate`}>Tax Rate %</Label>
                    <Input
                      id={`line_${i}_tax_rate`}
                      inputMode="decimal"
                      value={line.taxRate}
                      onChange={(e) => setLine(i, { taxRate: e.target.value })}
                    />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor={`line_${i}_account`}>{accountLabel}</Label>
                    <Select
                      value={line.accountId}
                      onValueChange={(v) => setLine(i, { accountId: v })}
                    >
                      <SelectTrigger
                        id={`line_${i}_account`}
                        className="w-full"
                      >
                        <SelectValue placeholder="Product's" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={NONE}>Product&apos;s</SelectItem>
                        {accounts.map((a) => (
                          <SelectItem key={a.id} value={String(a.id)}>
                            {a.code} — {a.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <div className="flex items-center justify-between">
                  <p className="text-sm text-muted-foreground">
                    Line total:{" "}
                    <span className="font-mono tabular-nums">
                      {previews[i] ? formatAmount(previews[i].total) : "—"}
                    </span>
                  </p>
                  {lines.length > 1 && (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() =>
                        setLines((ls) => ls.filter((_, j) => j !== i))
                      }
                    >
                      Remove
                    </Button>
                  )}
                </div>
              </div>
            ))}
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setLines((ls) => [...ls, blankLine])}
            >
              Add line
            </Button>
          </div>

          <div className="ml-auto w-full max-w-xs space-y-1 text-sm">
            {(
              [
                ["Subtotal", totals?.subtotal],
                ["Tax", totals?.tax],
                ["Total", totals?.total],
              ] as const
            ).map(([label, value]) => (
              <div key={label} className="flex justify-between">
                <span
                  className={
                    label === "Total" ? "font-semibold" : "text-muted-foreground"
                  }
                >
                  {label}
                </span>
                <span className="font-mono tabular-nums">
                  {value != null ? formatAmount(value) : "—"}
                </span>
              </div>
            ))}
          </div>

          {saveError !== null && (
            <p className="text-sm text-destructive" role="alert">
              Failed to save: {saveError}
            </p>
          )}

          <div className="flex gap-2">
            <Button type="submit" disabled={saving}>
              {saving ? "Saving…" : "Create draft"}
            </Button>
            <Button type="button" variant="outline" asChild>
              <Link to={basePath}>{backLabel}</Link>
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}
