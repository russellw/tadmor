import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import {
  listProducts,
  listStockMovements,
  listWarehouses,
  type StockMovement,
} from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// The movement types that post to the GL (an issue posts COGS, a receipt
// posts inventory against a clearing account); the rest only move stock.
export const POSTABLE_MOVEMENT_TYPES = new Set(["receipt", "issue"])

// GL state of a movement: posted iff it has a journal entry; draft only makes
// sense for postable types. Shared with the detail screen.
export function MovementGLBadge({ movement }: { movement: StockMovement }) {
  if (movement.journal_entry_id !== null) {
    return <Badge variant="default">posted</Badge>
  }
  if (POSTABLE_MOVEMENT_TYPES.has(movement.movement_type)) {
    return <Badge variant="outline">draft</Badge>
  }
  return <span className="text-sm text-muted-foreground">—</span>
}

// The stock-movement list from GET /api/stock-movements, joined client-side
// with products and warehouses for display names.
export function StockMovements() {
  const [movements, setMovements] = useState<StockMovement[] | null>(null)
  const [productNames, setProductNames] = useState<Map<number, string>>(
    new Map(),
  )
  const [warehouseNames, setWarehouseNames] = useState<Map<number, string>>(
    new Map(),
  )
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([listStockMovements(), listProducts(), listWarehouses()])
      .then(([rows, products, warehouses]) => {
        if (cancelled) return
        setProductNames(
          new Map(products.map((p) => [p.id, `${p.sku} — ${p.name}`])),
        )
        setWarehouseNames(new Map(warehouses.map((w) => [w.id, w.code])))
        setMovements(rows)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <section className="mx-auto w-full max-w-5xl p-6">
      <header className="mb-6 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Stock Movements
          </h1>
          <p className="text-sm text-muted-foreground">
            Inventory in and out, newest first. Receipts and issues post to the
            ledger; other types only move stock.
          </p>
        </div>
        <Button asChild>
          <Link to="/stock-movements/new">New movement</Link>
        </Button>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load: {error}
        </p>
      )}

      {error === null && movements === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {movements !== null && movements.length === 0 && (
        <p className="text-sm text-muted-foreground">No stock movements yet.</p>
      )}

      {movements !== null && movements.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-16">#</TableHead>
              <TableHead>Date</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>Product</TableHead>
              <TableHead>Warehouse</TableHead>
              <TableHead>GL</TableHead>
              <TableHead className="text-right">Qty</TableHead>
              <TableHead className="text-right">Unit Cost</TableHead>
              <TableHead className="text-right">Total Cost</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {movements.map((m) => (
              <TableRow key={m.id}>
                <TableCell className="font-mono">
                  <Link
                    to={`/stock-movements/${m.id}`}
                    className="font-medium text-primary hover:underline"
                  >
                    {m.id}
                  </Link>
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {m.date}
                </TableCell>
                <TableCell>
                  <Badge variant="secondary">{m.movement_type}</Badge>
                </TableCell>
                <TableCell className="font-medium">
                  {productNames.get(m.product_id) ?? `#${m.product_id}`}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {warehouseNames.get(m.warehouse_id) ?? `#${m.warehouse_id}`}
                </TableCell>
                <TableCell>
                  <MovementGLBadge movement={m} />
                </TableCell>
                <AmountCell value={m.quantity} />
                <AmountCell value={m.unit_cost} />
                <AmountCell value={m.total_cost} />
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </section>
  )
}
