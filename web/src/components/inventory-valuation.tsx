import { useEffect, useState } from "react"

import { sumAmounts } from "@/lib/amount"
import { getInventoryValuation, type StockValuationRow } from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// Inventory valuation from GET /api/inventory/valuation: quantity and value on
// hand per tracked product, with the total stock value in the footer.
export function InventoryValuation() {
  const [rows, setRows] = useState<StockValuationRow[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    getInventoryValuation()
      .then((data) => {
        if (!cancelled) setRows(data)
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
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">
          Inventory Valuation
        </h1>
        <p className="text-sm text-muted-foreground">
          Quantity and value on hand per tracked product, across warehouses.
        </p>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load the valuation: {error}
        </p>
      )}

      {error === null && rows === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {rows !== null && rows.length === 0 && (
        <p className="text-sm text-muted-foreground">No stock on hand.</p>
      )}

      {rows !== null && rows.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-28">SKU</TableHead>
              <TableHead>Product</TableHead>
              <TableHead className="text-right">Qty on hand</TableHead>
              <TableHead className="text-right">Avg unit cost</TableHead>
              <TableHead className="text-right">Value on hand</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r) => (
              <TableRow key={r.product_id}>
                <TableCell className="font-mono">{r.sku}</TableCell>
                <TableCell className="font-medium">{r.name}</TableCell>
                <AmountCell value={r.qty_on_hand} />
                <AmountCell value={r.avg_unit_cost} />
                <AmountCell value={r.value_on_hand} />
              </TableRow>
            ))}
          </TableBody>
          <TableFooter>
            <TableRow>
              <TableCell colSpan={4}>Total</TableCell>
              <AmountCell
                value={sumAmounts(rows.map((r) => r.value_on_hand))}
              />
            </TableRow>
          </TableFooter>
        </Table>
      )}
    </section>
  )
}
