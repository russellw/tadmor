import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import {
  listPurchaseOrders,
  listSalesOrders,
  type PurchaseOrderSummary,
  type SalesOrderSummary,
} from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
import {
  fetchCustomerNames,
  fetchSupplierNames,
} from "@/components/document-list"
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

// A row common to both order kinds after flattening the two fulfilment axes to
// a pair of labelled statuses, so one table renders sales and purchase orders.
interface OrderRow {
  id: number
  order_number: string
  party_id: number
  order_date: string
  currency_code: string
  status: string
  total: string
  fulfilA: string // invoiced (sales) | billed (purchase)
  fulfilB: string // shipped  (sales) | received (purchase)
}

export function SalesOrders() {
  return (
    <OrderList
      title="Sales Orders"
      description="Customer orders, newest first."
      partyLabel="Customer"
      colA="Invoiced"
      colB="Shipped"
      newLabel="New sales order"
      basePath="/sales-orders"
      emptyMessage="No sales orders yet."
      fetchOrders={fetchSalesRows}
      fetchPartyNames={fetchCustomerNames}
    />
  )
}

export function PurchaseOrders() {
  return (
    <OrderList
      title="Purchase Orders"
      description="Orders placed with suppliers, newest first."
      partyLabel="Supplier"
      colA="Billed"
      colB="Received"
      newLabel="New purchase order"
      basePath="/purchase-orders"
      emptyMessage="No purchase orders yet."
      fetchOrders={fetchPurchaseRows}
      fetchPartyNames={fetchSupplierNames}
    />
  )
}

async function fetchSalesRows(): Promise<OrderRow[]> {
  return (await listSalesOrders()).map((o: SalesOrderSummary) => ({
    id: o.id,
    order_number: o.order_number,
    party_id: o.customer_id,
    order_date: o.order_date,
    currency_code: o.currency_code,
    status: o.status,
    total: o.total,
    fulfilA: o.invoiced_status,
    fulfilB: o.shipped_status,
  }))
}

async function fetchPurchaseRows(): Promise<OrderRow[]> {
  return (await listPurchaseOrders()).map((o: PurchaseOrderSummary) => ({
    id: o.id,
    order_number: o.order_number,
    party_id: o.supplier_id,
    order_date: o.order_date,
    currency_code: o.currency_code,
    status: o.status,
    total: o.total,
    fulfilA: o.billed_status,
    fulfilB: o.received_status,
  }))
}

// Order lifecycle badge: draft → open → closed, or cancelled. Open reads as
// active (default), cancelled as destructive, the rest as muted outlines.
export function OrderStatusBadge({ status }: { status: string }) {
  const variant =
    status === "open"
      ? ("default" as const)
      : status === "cancelled"
        ? ("destructive" as const)
        : ("outline" as const)
  return <Badge variant={variant}>{status}</Badge>
}

// Fulfilment progress badge: none → partial → complete (any of the terminal
// words invoiced/shipped/billed/received).
export function FulfilmentBadge({ status }: { status: string }) {
  const variant =
    status === "none"
      ? ("outline" as const)
      : status === "partial"
        ? ("secondary" as const)
        : ("default" as const)
  return <Badge variant={variant}>{status}</Badge>
}

function OrderList({
  title,
  description,
  partyLabel,
  colA,
  colB,
  newLabel,
  basePath,
  emptyMessage,
  fetchOrders,
  fetchPartyNames,
}: {
  title: string
  description: string
  partyLabel: string
  colA: string
  colB: string
  newLabel: string
  basePath: string
  emptyMessage: string
  fetchOrders: () => Promise<OrderRow[]>
  fetchPartyNames: () => Promise<Map<number, string>>
}) {
  const [orders, setOrders] = useState<OrderRow[] | null>(null)
  const [partyNames, setPartyNames] = useState<Map<number, string>>(new Map())
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([fetchOrders(), fetchPartyNames()])
      .then(([rows, names]) => {
        if (cancelled) return
        setPartyNames(names)
        setOrders(rows)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [fetchOrders, fetchPartyNames])

  return (
    <section className="mx-auto w-full max-w-5xl p-6">
      <header className="mb-6 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
          <p className="text-sm text-muted-foreground">{description}</p>
        </div>
        <Button asChild>
          <Link to={`${basePath}/new`}>{newLabel}</Link>
        </Button>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load: {error}
        </p>
      )}

      {error === null && orders === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {orders !== null && orders.length === 0 && (
        <p className="text-sm text-muted-foreground">{emptyMessage}</p>
      )}

      {orders !== null && orders.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Number</TableHead>
              <TableHead>{partyLabel}</TableHead>
              <TableHead>Date</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>{colA}</TableHead>
              <TableHead>{colB}</TableHead>
              <TableHead className="text-right">Total</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {orders.map((o) => (
              <TableRow key={o.id}>
                <TableCell className="font-mono">
                  <Link
                    to={`${basePath}/${o.id}`}
                    className="font-medium text-primary hover:underline"
                  >
                    {o.order_number}
                  </Link>
                </TableCell>
                <TableCell className="font-medium">
                  {partyNames.get(o.party_id) ?? `#${o.party_id}`}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {o.order_date}
                </TableCell>
                <TableCell>
                  <OrderStatusBadge status={o.status} />
                </TableCell>
                <TableCell>
                  <FulfilmentBadge status={o.fulfilA} />
                </TableCell>
                <TableCell>
                  <FulfilmentBadge status={o.fulfilB} />
                </TableCell>
                <AmountCell value={o.total} />
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </section>
  )
}
