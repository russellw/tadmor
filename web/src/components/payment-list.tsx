import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import { isZeroAmount } from "@/lib/amount"
import {
  listCustomerPayments,
  listSupplierPayments,
  type Payment,
} from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
import {
  fetchCustomerNames,
  fetchSupplierNames,
  StatusBadge,
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

// Customer and supplier payments share the row shape (api.Payment), so both
// lists render through one parameterized component, like document-list.tsx.

export function CustomerPayments() {
  return (
    <PaymentList
      title="Customer Payments"
      description="Money received from customers, newest first."
      partyLabel="Customer"
      newLabel="New payment"
      basePath="/customer-payments"
      emptyMessage="No customer payments yet."
      fetchPayments={listCustomerPayments}
      fetchPartyNames={fetchCustomerNames}
    />
  )
}

export function SupplierPayments() {
  return (
    <PaymentList
      title="Supplier Payments"
      description="Money paid to suppliers, newest first."
      partyLabel="Supplier"
      newLabel="New payment"
      basePath="/supplier-payments"
      emptyMessage="No supplier payments yet."
      fetchPayments={listSupplierPayments}
      fetchPartyNames={fetchSupplierNames}
    />
  )
}

// How much of a posted payment has found its documents; drafts show nothing
// (they can't be applied yet). Shared with the detail screen.
export function AppliedBadge({ payment }: { payment: Payment }) {
  if (payment.status !== "posted") return null
  if (isZeroAmount(payment.amount_applied)) {
    return <Badge variant="outline">unapplied</Badge>
  }
  if (isZeroAmount(payment.unapplied)) {
    return <Badge variant="default">applied</Badge>
  }
  return <Badge variant="secondary">partial</Badge>
}

function PaymentList({
  title,
  description,
  partyLabel,
  newLabel,
  basePath,
  emptyMessage,
  fetchPayments,
  fetchPartyNames,
}: {
  title: string
  description: string
  partyLabel: string
  newLabel: string
  basePath: string
  emptyMessage: string
  fetchPayments: () => Promise<Payment[]>
  fetchPartyNames: () => Promise<Map<number, string>>
}) {
  const [payments, setPayments] = useState<Payment[] | null>(null)
  const [partyNames, setPartyNames] = useState<Map<number, string>>(new Map())
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([fetchPayments(), fetchPartyNames()])
      .then(([rows, names]) => {
        if (cancelled) return
        setPartyNames(names)
        setPayments(rows)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [fetchPayments, fetchPartyNames])

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

      {error === null && payments === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {payments !== null && payments.length === 0 && (
        <p className="text-sm text-muted-foreground">{emptyMessage}</p>
      )}

      {payments !== null && payments.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-16">#</TableHead>
              <TableHead>{partyLabel}</TableHead>
              <TableHead>Date</TableHead>
              <TableHead>Method</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Applied</TableHead>
              <TableHead className="text-right">Amount</TableHead>
              <TableHead className="text-right">Unapplied</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {payments.map((p) => (
              <TableRow key={p.id}>
                <TableCell className="font-mono">
                  <Link
                    to={`${basePath}/${p.id}`}
                    className="font-medium text-primary hover:underline"
                  >
                    {p.id}
                  </Link>
                </TableCell>
                <TableCell className="font-medium">
                  {partyNames.get(p.party_id) ?? `#${p.party_id}`}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {p.date}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {p.method ?? "—"}
                </TableCell>
                <TableCell>
                  <StatusBadge status={p.status} />
                </TableCell>
                <TableCell>
                  <AppliedBadge payment={p} />
                </TableCell>
                <AmountCell value={p.amount} />
                <AmountCell value={p.unapplied} />
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </section>
  )
}
