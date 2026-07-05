import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import { listWarehouses, type Warehouse } from "@/lib/api"
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

export function Warehouses() {
  const [warehouses, setWarehouses] = useState<Warehouse[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    listWarehouses()
      .then((data) => {
        if (!cancelled) setWarehouses(data)
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
          <h1 className="text-2xl font-semibold tracking-tight">Warehouses</h1>
          <p className="text-sm text-muted-foreground">
            The stock locations movements happen against, ordered by code.
          </p>
        </div>
        <Button asChild>
          <Link to="/warehouses/new">New warehouse</Link>
        </Button>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load warehouses: {error}
        </p>
      )}

      {error === null && warehouses === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {warehouses !== null && warehouses.length === 0 && (
        <p className="text-sm text-muted-foreground">No warehouses yet.</p>
      )}

      {warehouses !== null && warehouses.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Code</TableHead>
              <TableHead>Name</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="w-0"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {warehouses.map((w) => (
              <TableRow key={w.id}>
                <TableCell className="font-mono font-medium">
                  {w.code}
                </TableCell>
                <TableCell>{w.name}</TableCell>
                <TableCell>
                  <Badge variant={w.is_active ? "default" : "outline"}>
                    {w.is_active ? "Active" : "Inactive"}
                  </Badge>
                </TableCell>
                <TableCell className="text-right">
                  <Link
                    to={`/warehouses/${w.id}`}
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
