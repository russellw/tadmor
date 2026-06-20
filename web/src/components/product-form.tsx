import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import {
  ApiError,
  createProduct,
  getProduct,
  listAccounts,
  listTaxCodes,
  updateProduct,
  type Account,
  type ProductInput,
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
import { Textarea } from "@/components/ui/textarea"

type Mode = "create" | "edit"

// Radix Select reserves "" as a value, so nullable selects use this sentinel.
const NONE = "__none__"

// Products are standalone catalog entities (no organization), so this form has
// no org field. It carries three nullable account FKs (revenue, inventory, cogs)
// rendered as dropdowns; SKU and name are required.
interface FormState {
  sku: string
  name: string
  description: string
  unitPrice: string
  currencyCode: string
  revenueAccountId: string
  taxCode: string
  trackInventory: boolean
  inventoryAccountId: string
  cogsAccountId: string
  isActive: boolean
}

const blankForm: FormState = {
  sku: "",
  name: "",
  description: "",
  unitPrice: "0",
  currencyCode: "",
  revenueAccountId: NONE,
  taxCode: NONE,
  trackInventory: false,
  inventoryAccountId: NONE,
  cogsAccountId: NONE,
  isActive: true,
}

function emptyToNull(s: string): string | null {
  const t = s.trim()
  return t === "" ? null : t
}

function accountId(v: string): number | null {
  return v === NONE ? null : Number(v)
}

// A nullable account dropdown, reused for the three account FKs. Module-level so
// it keeps a stable identity across the parent's re-renders (a nested component
// would remount the Select on every keystroke).
function AccountSelect({
  id,
  value,
  accounts,
  onChange,
}: {
  id: string
  value: string
  accounts: Account[]
  onChange: (v: string) => void
}) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger id={id} className="w-full">
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
  )
}

export function ProductForm({ mode }: { mode: Mode }) {
  const { id } = useParams()
  const productId = Number(id)
  const navigate = useNavigate()

  const [form, setForm] = useState<FormState | null>(null)
  const [accounts, setAccounts] = useState<Account[]>([])
  const [taxCodes, setTaxCodes] = useState<TaxCode[]>([])
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      try {
        const [accts, taxes] = await Promise.all([
          listAccounts(),
          listTaxCodes(),
        ])
        if (cancelled) return
        setAccounts(accts)
        setTaxCodes(taxes)

        if (mode === "edit") {
          if (!Number.isInteger(productId) || productId <= 0) {
            setError("Invalid product id.")
            return
          }
          const product = await getProduct(productId)
          if (cancelled) return
          setForm({
            sku: product.sku,
            name: product.name,
            description: product.description ?? "",
            unitPrice: product.unit_price,
            currencyCode: product.currency_code ?? "",
            revenueAccountId:
              product.revenue_account_id != null
                ? String(product.revenue_account_id)
                : NONE,
            taxCode: product.tax_code ?? NONE,
            trackInventory: product.track_inventory,
            inventoryAccountId:
              product.inventory_account_id != null
                ? String(product.inventory_account_id)
                : NONE,
            cogsAccountId:
              product.cogs_account_id != null
                ? String(product.cogs_account_id)
                : NONE,
            isActive: product.is_active,
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
  }, [mode, productId])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form) return
    if (form.sku.trim() === "" || form.name.trim() === "") {
      setSaveError("SKU and name are required.")
      return
    }
    const currency = form.currencyCode.trim().toUpperCase()
    const input: ProductInput = {
      sku: form.sku.trim(),
      name: form.name.trim(),
      description: emptyToNull(form.description),
      unit_price: form.unitPrice.trim() === "" ? "0" : form.unitPrice.trim(),
      currency_code: currency === "" ? null : currency,
      revenue_account_id: accountId(form.revenueAccountId),
      tax_code: form.taxCode === NONE ? null : form.taxCode,
      track_inventory: form.trackInventory,
      inventory_account_id: accountId(form.inventoryAccountId),
      cogs_account_id: accountId(form.cogsAccountId),
      is_active: form.isActive,
    }
    setSaving(true)
    setSaveError(null)
    const action =
      mode === "create"
        ? createProduct(input).then(() => undefined)
        : updateProduct(productId, input)
    action
      .then(() => navigate("/products"))
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
          {creating ? "New Product" : "Edit Product"}
        </h1>
        <p className="text-sm text-muted-foreground">
          {creating
            ? "Add a product or service to the catalog."
            : "Update the product's pricing and accounts."}
        </p>
      </header>

      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/products">Back to products</Link>
          </Button>
        </div>
      )}

      {error === null && form === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {form !== null && (
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="sku">SKU</Label>
              <Input
                id="sku"
                required
                value={form.sku}
                onChange={(e) => setForm({ ...form, sku: e.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="name">Name</Label>
              <Input
                id="name"
                required
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="description">Description</Label>
            <Textarea
              id="description"
              value={form.description}
              onChange={(e) =>
                setForm({ ...form, description: e.target.value })
              }
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="unit_price">Unit Price</Label>
              <Input
                id="unit_price"
                inputMode="decimal"
                value={form.unitPrice}
                onChange={(e) =>
                  setForm({ ...form, unitPrice: e.target.value })
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
            <Label htmlFor="revenue_account">Revenue Account</Label>
            <AccountSelect
              id="revenue_account"
              accounts={accounts}
              value={form.revenueAccountId}
              onChange={(v) => setForm({ ...form, revenueAccountId: v })}
            />
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

          <div className="flex items-center gap-2">
            <Checkbox
              id="track_inventory"
              checked={form.trackInventory}
              onCheckedChange={(c) =>
                setForm({ ...form, trackInventory: c === true })
              }
            />
            <Label htmlFor="track_inventory">Track inventory</Label>
          </div>

          {form.trackInventory && (
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="inventory_account">Inventory Account</Label>
                <AccountSelect
                  id="inventory_account"
                  accounts={accounts}
                  value={form.inventoryAccountId}
                  onChange={(v) =>
                    setForm({ ...form, inventoryAccountId: v })
                  }
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="cogs_account">COGS Account</Label>
                <AccountSelect
                  id="cogs_account"
                  accounts={accounts}
                  value={form.cogsAccountId}
                  onChange={(v) => setForm({ ...form, cogsAccountId: v })}
                />
              </div>
            </div>
          )}

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
              <Link to="/products">Cancel</Link>
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}
