import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import { formatAmount, lineAmounts, sumAmounts, trimAmount } from "@/lib/amount"
import {
  ApiError,
  createPurchaseBill,
  getPurchaseBill,
  getPurchaseBillLines,
  listAccounts,
  listOrganizations,
  listProducts,
  listSuppliers,
  listTaxCodes,
  updatePurchaseBill,
  type Account,
  type Product,
  type PurchaseBillInput,
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

interface LineState {
  productId: string // NONE = free-form line
  description: string
  quantity: string
  unitCost: string
  taxCode: string
  taxRate: string
  expenseAccountId: string // NONE = fall back to the product's inventory account
}

const blankLine: LineState = {
  productId: NONE,
  description: "",
  quantity: "1",
  unitCost: "",
  taxCode: NONE,
  taxRate: "0",
  expenseAccountId: NONE,
}

interface SupplierOption {
  id: number
  name: string
  currency: string | null
}

function emptyToNull(s: string): string | null {
  const t = s.trim()
  return t === "" ? null : t
}

// Preview of one line's money, mirroring the backend's empty-input defaults
// (quantity 1, cost 0, rate 0) so the on-screen total matches what the
// database will compute.
function previewLine(l: LineState) {
  return lineAmounts(
    l.quantity.trim() === "" ? "1" : l.quantity,
    l.unitCost.trim() === "" ? "0" : l.unitCost,
    l.taxRate.trim() === "" ? "0" : l.taxRate,
  )
}

// Purchase-bill form: header fields plus dynamic lines, mirroring the
// invoice form on the purchasing side. Picking a product prefills the
// description and tax from the catalog but not the cost (a product's price is
// its sales price); at posting time a product line without an explicit
// expense account debits the product's inventory account, so the account
// select defaults to "Product's". The bill is created as a draft — posting to
// the GL happens from the bill page. In edit mode the form loads an existing
// draft and rewrites it in place; posted and order-linked bills are refused
// up front.
export function BillForm({ mode = "create" }: { mode?: "create" | "edit" }) {
  const navigate = useNavigate()
  const { id } = useParams()
  const billId = mode === "edit" ? Number(id) : null

  const [suppliers, setSuppliers] = useState<SupplierOption[]>([])
  const [products, setProducts] = useState<Product[]>([])
  const [taxCodes, setTaxCodes] = useState<TaxCode[]>([])
  const [accounts, setAccounts] = useState<Account[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loaded, setLoaded] = useState(false)

  const [supplierId, setSupplierId] = useState("")
  const [billNumber, setBillNumber] = useState("")
  const [billDate, setBillDate] = useState(new Date().toISOString().slice(0, 10))
  const [dueDate, setDueDate] = useState("")
  const [currencyCode, setCurrencyCode] = useState("")
  const [reference, setReference] = useState("")
  const [memo, setMemo] = useState("")
  const [lines, setLines] = useState<LineState[]>([blankLine])

  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([
      listSuppliers(),
      listOrganizations(),
      listProducts(),
      listTaxCodes(),
      listAccounts(),
      billId !== null ? getPurchaseBill(billId) : null,
      billId !== null ? getPurchaseBillLines(billId) : null,
    ])
      .then(([supps, orgs, prods, taxes, accts, doc, docLines]) => {
        if (cancelled) return
        const orgNames = new Map(orgs.map((o) => [o.id, o.name]))
        setSuppliers(
          supps
            .filter((s) => s.is_active)
            .map((s) => ({
              id: s.id,
              name: orgNames.get(s.organization_id) ?? `#${s.id}`,
              currency: s.currency_code,
            })),
        )
        setProducts(prods.filter((p) => p.is_active))
        setTaxCodes(taxes)
        setAccounts(accts)
        if (doc !== null && docLines !== null) {
          if (doc.status !== "draft") {
            setError("Only draft bills can be edited.")
            return
          }
          if (docLines.some((l) => l.order_line_id !== null)) {
            setError(
              "This bill was created from a purchase order and cannot be edited. Delete it and bill the order again instead.",
            )
            return
          }
          setSupplierId(String(doc.party_id))
          setBillNumber(doc.number)
          setBillDate(doc.date)
          setDueDate(doc.due_date ?? "")
          setCurrencyCode(doc.currency_code)
          setReference(doc.reference ?? "")
          setMemo(doc.memo ?? "")
          setLines(
            docLines.map((l) => ({
              productId: l.product_id !== null ? String(l.product_id) : NONE,
              description: l.description,
              quantity: trimAmount(l.quantity),
              unitCost: trimAmount(l.unit_cost),
              taxCode: l.tax_code ?? NONE,
              taxRate: trimAmount(l.tax_rate),
              expenseAccountId:
                l.expense_account_id !== null
                  ? String(l.expense_account_id)
                  : NONE,
            })),
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
  }, [billId])

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
      description: product.name,
      taxCode: product.tax_code ?? NONE,
      taxRate: rate,
    })
  }

  function chooseTaxCode(index: number, value: string) {
    const rate =
      value === NONE ? "0" : (taxCodes.find((t) => t.code === value)?.rate ?? "0")
    setLine(index, { taxCode: value, taxRate: rate })
  }

  function chooseSupplier(value: string) {
    setSupplierId(value)
    const s = suppliers.find((s) => String(s.id) === value)
    if (s?.currency && currencyCode.trim() === "") {
      setCurrencyCode(s.currency)
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
    if (supplierId === "") {
      setSaveError("Please choose a supplier.")
      return
    }
    const input: PurchaseBillInput = {
      bill_number: billNumber.trim(),
      supplier_id: Number(supplierId),
      bill_date: billDate,
      due_date: emptyToNull(dueDate),
      currency_code: currencyCode.trim().toUpperCase(),
      reference: emptyToNull(reference),
      memo: emptyToNull(memo),
      lines: lines.map((l) => ({
        product_id: l.productId === NONE ? null : Number(l.productId),
        description: l.description.trim(),
        quantity: l.quantity.trim(),
        unit_cost: l.unitCost.trim(),
        expense_account_id:
          l.expenseAccountId === NONE ? null : Number(l.expenseAccountId),
        tax_code: l.taxCode === NONE ? null : l.taxCode,
        tax_rate: l.taxRate.trim(),
      })),
    }
    setSaving(true)
    setSaveError(null)
    const save =
      billId !== null
        ? updatePurchaseBill(billId, input).then(() => billId)
        : createPurchaseBill(input).then(({ id }) => id)
    save
      .then((docId) => navigate(`/bills/${docId}`))
      .catch((err: unknown) => {
        setSaving(false)
        setSaveError(err instanceof ApiError ? err.message : String(err))
      })
  }

  return (
    <section className="mx-auto w-full max-w-5xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">
          {billId !== null ? "Edit Bill" : "New Bill"}
        </h1>
        <p className="text-sm text-muted-foreground">
          {billId !== null
            ? "Rewrites the draft — it stays unposted."
            : "Creates a draft — post it to the ledger from the bill page."}
        </p>
      </header>

      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/bills">Back to bills</Link>
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
              <Label htmlFor="supplier">Supplier</Label>
              <Select value={supplierId} onValueChange={chooseSupplier}>
                <SelectTrigger id="supplier" className="w-full">
                  <SelectValue placeholder="Select a supplier" />
                </SelectTrigger>
                <SelectContent>
                  {suppliers.map((s) => (
                    <SelectItem key={s.id} value={String(s.id)}>
                      {s.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="bill_number">Bill #</Label>
              <Input
                id="bill_number"
                required
                value={billNumber}
                onChange={(e) => setBillNumber(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="bill_date">Bill Date</Label>
              <Input
                id="bill_date"
                type="date"
                required
                value={billDate}
                onChange={(e) => setBillDate(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="due_date">Due Date</Label>
              <Input
                id="due_date"
                type="date"
                value={dueDate}
                onChange={(e) => setDueDate(e.target.value)}
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
                    <Label htmlFor={`line_${i}_unit_cost`}>Unit Cost</Label>
                    <Input
                      id={`line_${i}_unit_cost`}
                      inputMode="decimal"
                      value={line.unitCost}
                      onChange={(e) => setLine(i, { unitCost: e.target.value })}
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
                    <Label htmlFor={`line_${i}_expense_account`}>
                      Expense Account
                    </Label>
                    <Select
                      value={line.expenseAccountId}
                      onValueChange={(v) =>
                        setLine(i, { expenseAccountId: v })
                      }
                    >
                      <SelectTrigger
                        id={`line_${i}_expense_account`}
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
              {saving ? "Saving…" : billId !== null ? "Save changes" : "Create draft"}
            </Button>
            <Button type="button" variant="outline" asChild>
              <Link to={billId !== null ? `/bills/${billId}` : "/bills"}>
                Cancel
              </Link>
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}
