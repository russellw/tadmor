import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import {
  listCustomers,
  listOrganizations,
  listSalesInvoices,
  type DocumentBalance,
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

// Document lifecycle badge (draft → posted, or void), shared with the detail
// screen.
export function StatusBadge({ status }: { status: string }) {
  const variant =
    status === "posted"
      ? ("default" as const)
      : status === "void"
        ? ("destructive" as const)
        : ("outline" as const)
  return <Badge variant={variant}>{status}</Badge>
}

// Payment progress badge (unpaid → partial → paid), shared with the detail
// screen.
export function PaymentBadge({ status }: { status: string }) {
  const variant =
    status === "paid"
      ? ("default" as const)
      : status === "partial"
        ? ("secondary" as const)
        : ("outline" as const)
  return <Badge variant={variant}>{status}</Badge>
}

// The sales-invoice list: every invoice's balance view from
// GET /api/sales-invoices, joined client-side with customers → organizations
// for the display name (an invoice carries only customer_id).
export function Invoices() {
  const [invoices, setInvoices] = useState<DocumentBalance[] | null>(null)
  const [customerNames, setCustomerNames] = useState<Map<number, string>>(
    new Map(),
  )
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([listSalesInvoices(), listCustomers(), listOrganizations()])
      .then(([invs, customers, orgs]) => {
        if (cancelled) return
        const orgNames = new Map(orgs.map((o) => [o.id, o.name]))
        setCustomerNames(
          new Map(
            customers.map((c) => [
              c.id,
              orgNames.get(c.organization_id) ?? `#${c.id}`,
            ]),
          ),
        )
        setInvoices(invs)
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
            Sales Invoices
          </h1>
          <p className="text-sm text-muted-foreground">
            Draft and posted invoices, newest first.
          </p>
        </div>
        <Button asChild>
          <Link to="/invoices/new">New invoice</Link>
        </Button>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load invoices: {error}
        </p>
      )}

      {error === null && invoices === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {invoices !== null && invoices.length === 0 && (
        <p className="text-sm text-muted-foreground">No invoices yet.</p>
      )}

      {invoices !== null && invoices.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Number</TableHead>
              <TableHead>Customer</TableHead>
              <TableHead>Date</TableHead>
              <TableHead>Due</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Payment</TableHead>
              <TableHead className="text-right">Total</TableHead>
              <TableHead className="text-right">Balance</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {invoices.map((inv) => (
              <TableRow key={inv.id}>
                <TableCell className="font-mono">
                  <Link
                    to={`/invoices/${inv.id}`}
                    className="font-medium text-primary hover:underline"
                  >
                    {inv.number}
                  </Link>
                </TableCell>
                <TableCell className="font-medium">
                  {customerNames.get(inv.party_id) ?? `#${inv.party_id}`}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {inv.date}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {inv.due_date ?? "—"}
                </TableCell>
                <TableCell>
                  <StatusBadge status={inv.status} />
                </TableCell>
                <TableCell>
                  <PaymentBadge status={inv.payment_status} />
                </TableCell>
                <AmountCell value={inv.total} />
                <AmountCell value={inv.balance} />
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </section>
  )
}
