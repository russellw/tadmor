import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import { listProducts, type Product } from "@/lib/api"
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

// Products are standalone catalog entities (their own SKU and name), so unlike
// customers/suppliers this is a single fetch with no organization join.
export function Products() {
  const [products, setProducts] = useState<Product[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    listProducts()
      .then((data) => {
        if (!cancelled) setProducts(data)
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
          <h1 className="text-2xl font-semibold tracking-tight">Products</h1>
          <p className="text-sm text-muted-foreground">
            The product / service catalog, ordered by SKU.
          </p>
        </div>
        <Button asChild>
          <Link to="/products/new">New product</Link>
        </Button>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load products: {error}
        </p>
      )}

      {error === null && products === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {products !== null && products.length === 0 && (
        <p className="text-sm text-muted-foreground">No products yet.</p>
      )}

      {products !== null && products.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>SKU</TableHead>
              <TableHead>Name</TableHead>
              <TableHead className="text-right">Unit Price</TableHead>
              <TableHead>Currency</TableHead>
              <TableHead>Tax Code</TableHead>
              <TableHead>Inventory</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="w-0"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {products.map((p) => (
              <TableRow key={p.id}>
                <TableCell className="font-mono">{p.sku}</TableCell>
                <TableCell className="font-medium">{p.name}</TableCell>
                <TableCell className="text-right font-mono tabular-nums">
                  {p.unit_price}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {p.currency_code ?? "—"}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {p.tax_code ?? "—"}
                </TableCell>
                <TableCell>
                  {p.track_inventory ? (
                    <Badge variant="secondary">Tracked</Badge>
                  ) : (
                    <span className="text-muted-foreground">—</span>
                  )}
                </TableCell>
                <TableCell>
                  <Badge variant={p.is_active ? "default" : "outline"}>
                    {p.is_active ? "Active" : "Inactive"}
                  </Badge>
                </TableCell>
                <TableCell className="text-right">
                  <Link
                    to={`/products/${p.id}`}
                    className="text-sm font-medium text-primary hover:underline"
                  >
                    Edit
                  </Link>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </section>
  )
}
