import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import { trimAmount } from "@/lib/amount"
import {
  ApiError,
  createStockMovement,
  getStockMovement,
  listProducts,
  listWarehouses,
  MOVEMENT_TYPES,
  updateStockMovement,
  type Product,
  type Warehouse,
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
import { Textarea } from "@/components/ui/textarea"

function emptyToNull(s: string): string | null {
  const t = s.trim()
  return t === "" ? null : t
}

// The schema requires the quantity's sign to agree with the movement type
// (receipt/transfer_in positive, issue/transfer_out negative, adjustment
// either). The form takes a magnitude and signs it from the type, except for
// adjustments, which pass through as typed.
function signQuantity(type: string, quantity: string): string {
  const magnitude = quantity.replace(/^-/, "")
  switch (type) {
    case "issue":
    case "transfer_out":
      return magnitude === "" ? "" : `-${magnitude}`
    case "adjustment":
      return quantity
    default:
      return magnitude
  }
}

// Stock-movement form. Creates the movement immediately (movements have
// no draft/posted status of their own); posting a receipt or issue to the GL
// happens from the movement page. In edit mode the form loads an existing
// movement and rewrites it; posted and order-fulfilment movements are refused
// up front.
export function StockMovementForm({
  mode = "create",
}: {
  mode?: "create" | "edit"
}) {
  const navigate = useNavigate()
  const { id } = useParams()
  const editId = mode === "edit" ? Number(id) : null

  const [products, setProducts] = useState<Product[]>([])
  const [warehouses, setWarehouses] = useState<Warehouse[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loaded, setLoaded] = useState(false)

  const [productId, setProductId] = useState("")
  const [warehouseId, setWarehouseId] = useState("")
  const [movementType, setMovementType] = useState("receipt")
  const [date, setDate] = useState(new Date().toISOString().slice(0, 10))
  const [quantity, setQuantity] = useState("")
  const [unitCost, setUnitCost] = useState("")
  const [reference, setReference] = useState("")
  const [notes, setNotes] = useState("")

  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([
      listProducts(),
      listWarehouses(),
      editId !== null ? getStockMovement(editId) : null,
    ])
      .then(([prods, whs, movement]) => {
        if (cancelled) return
        setProducts(prods.filter((p) => p.is_active && p.track_inventory))
        setWarehouses(whs.filter((w) => w.is_active))
        if (movement !== null) {
          if (movement.journal_entry_id !== null) {
            setError("Posted stock movements cannot be edited.")
            return
          }
          if (movement.source_type !== null) {
            setError(
              "This movement was created by order fulfilment and cannot be edited. Delete it and ship or receive the order again instead.",
            )
            return
          }
          setProductId(String(movement.product_id))
          setWarehouseId(String(movement.warehouse_id))
          setMovementType(movement.movement_type)
          setDate(movement.date)
          // The form takes a magnitude for signed types (adjustments pass
          // through as stored).
          setQuantity(
            movement.movement_type === "adjustment"
              ? trimAmount(movement.quantity)
              : trimAmount(movement.quantity).replace(/^-/, ""),
          )
          setUnitCost(trimAmount(movement.unit_cost))
          setReference(movement.reference ?? "")
          setNotes(movement.notes ?? "")
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
  }, [editId])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (productId === "" || warehouseId === "") {
      setSaveError("Please choose a product and a warehouse.")
      return
    }
    setSaving(true)
    setSaveError(null)
    const input = {
      product_id: Number(productId),
      warehouse_id: Number(warehouseId),
      movement_type: movementType,
      movement_date: date,
      quantity: signQuantity(movementType, quantity.trim()),
      unit_cost: unitCost.trim(),
      reference: emptyToNull(reference),
      notes: emptyToNull(notes),
    }
    const save =
      editId !== null
        ? updateStockMovement(editId, input).then(() => editId)
        : createStockMovement(input).then(({ id }) => id)
    save
      .then((docId) => navigate(`/stock-movements/${docId}`))
      .catch((err: unknown) => {
        setSaving(false)
        setSaveError(err instanceof ApiError ? err.message : String(err))
      })
  }

  return (
    <section className="mx-auto w-full max-w-2xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">
          {editId !== null ? "Edit Stock Movement" : "New Stock Movement"}
        </h1>
        <p className="text-sm text-muted-foreground">
          {editId !== null
            ? "Rewrites the movement — stock on hand adjusts immediately."
            : "Moves stock immediately. Receipts and issues can then be posted to the ledger from the movement page."}
        </p>
      </header>

      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/stock-movements">Back to stock movements</Link>
          </Button>
        </div>
      )}

      {error === null && !loaded && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {error === null && loaded && (
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="product">Product</Label>
            {products.length > 0 ? (
              <Select value={productId} onValueChange={setProductId}>
                <SelectTrigger id="product" className="w-full">
                  <SelectValue placeholder="Select a product" />
                </SelectTrigger>
                <SelectContent>
                  {products.map((p) => (
                    <SelectItem key={p.id} value={String(p.id)}>
                      {p.sku} — {p.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <p className="text-sm text-muted-foreground">
                No inventory-tracked products. Create a product with "track
                inventory" enabled first.
              </p>
            )}
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="warehouse">Warehouse</Label>
              <Select value={warehouseId} onValueChange={setWarehouseId}>
                <SelectTrigger id="warehouse" className="w-full">
                  <SelectValue placeholder="Select a warehouse" />
                </SelectTrigger>
                <SelectContent>
                  {warehouses.map((w) => (
                    <SelectItem key={w.id} value={String(w.id)}>
                      {w.code} — {w.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="movement_type">Type</Label>
              <Select value={movementType} onValueChange={setMovementType}>
                <SelectTrigger id="movement_type" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {MOVEMENT_TYPES.map((t) => (
                    <SelectItem key={t} value={t}>
                      {t}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="date">Date</Label>
              <Input
                id="date"
                type="date"
                required
                value={date}
                onChange={(e) => setDate(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="quantity">Quantity</Label>
              <Input
                id="quantity"
                inputMode="decimal"
                required
                value={quantity}
                onChange={(e) => setQuantity(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                {movementType === "adjustment"
                  ? "Signed: negative for shrinkage, positive for found stock."
                  : "Magnitude only; the type determines the direction."}
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="unit_cost">Unit Cost</Label>
              <Input
                id="unit_cost"
                inputMode="decimal"
                value={unitCost}
                onChange={(e) => setUnitCost(e.target.value)}
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
          </div>

          <div className="space-y-2">
            <Label htmlFor="notes">Notes</Label>
            <Textarea
              id="notes"
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
            />
          </div>

          {saveError !== null && (
            <p className="text-sm text-destructive" role="alert">
              Failed to save: {saveError}
            </p>
          )}

          <div className="flex gap-2">
            <Button
              type="submit"
              disabled={saving || products.length === 0}
            >
              {saving ? "Saving…" : editId !== null ? "Save changes" : "Create"}
            </Button>
            <Button type="button" variant="outline" asChild>
              <Link
                to={
                  editId !== null
                    ? `/stock-movements/${editId}`
                    : "/stock-movements"
                }
              >
                Cancel
              </Link>
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}
