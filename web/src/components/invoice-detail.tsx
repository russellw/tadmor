import { useCallback, useEffect, useState } from "react"
import { Link, useParams } from "react-router-dom"

import { sumAmounts } from "@/lib/amount"
import {
  ApiError,
  getSalesInvoice,
  getSalesInvoiceLines,
  listCustomers,
  listOrganizations,
  postSalesInvoice,
  unpostSalesInvoice,
  type DocumentBalance,
  type SalesInvoiceLine,
} from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
import { PaymentBadge, StatusBadge } from "@/components/invoices"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// One invoice: header, database-computed lines, and the lifecycle actions.
// Post writes the journal entry (Dr A/R, Cr revenue/tax); Unpost reverses it
// and returns the invoice to draft.
export function InvoiceDetail() {
  const { id } = useParams()
  const invoiceId = Number(id)

  const [invoice, setInvoice] = useState<DocumentBalance | null>(null)
  const [lines, setLines] = useState<SalesInvoiceLine[] | null>(null)
  const [customerName, setCustomerName] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [acting, setActing] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)

  const load = useCallback(async () => {
    const [inv, lns, customers, orgs] = await Promise.all([
      getSalesInvoice(invoiceId),
      getSalesInvoiceLines(invoiceId),
      listCustomers(),
      listOrganizations(),
    ])
    const customer = customers.find((c) => c.id === inv.party_id)
    const org = orgs.find((o) => o.id === customer?.organization_id)
    return { inv, lns, name: org?.name ?? `#${inv.party_id}` }
  }, [invoiceId])

  useEffect(() => {
    if (!Number.isInteger(invoiceId) || invoiceId <= 0) {
      setError("Invalid invoice id.")
      return
    }
    let cancelled = false
    load()
      .then(({ inv, lns, name }) => {
        if (cancelled) return
        setInvoice(inv)
        setLines(lns)
        setCustomerName(name)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [invoiceId, load])

  function runAction(action: (id: number) => Promise<unknown>) {
    setActing(true)
    setActionError(null)
    action(invoiceId)
      .then(load)
      .then(({ inv, lns, name }) => {
        setInvoice(inv)
        setLines(lns)
        setCustomerName(name)
        setActing(false)
      })
      .catch((err: unknown) => {
        setActing(false)
        setActionError(err instanceof ApiError ? err.message : String(err))
      })
  }

  return (
    <section className="mx-auto w-full max-w-5xl p-6">
      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/invoices">Back to invoices</Link>
          </Button>
        </div>
      )}

      {error === null && (invoice === null || lines === null) && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {invoice !== null && lines !== null && (
        <>
          <header className="mb-6 flex items-start justify-between gap-4">
            <div>
              <div className="flex items-center gap-3">
                <h1 className="text-2xl font-semibold tracking-tight">
                  Invoice {invoice.number}
                </h1>
                <StatusBadge status={invoice.status} />
                <PaymentBadge status={invoice.payment_status} />
              </div>
              <p className="text-sm text-muted-foreground">
                {customerName} · {invoice.date}
                {invoice.due_date !== null && ` · due ${invoice.due_date}`}
                {" · "}
                {invoice.currency_code}
              </p>
            </div>
            <div className="flex gap-2">
              {invoice.status === "draft" && (
                <Button
                  disabled={acting}
                  onClick={() => runAction(postSalesInvoice)}
                >
                  {acting ? "Posting…" : "Post to ledger"}
                </Button>
              )}
              {invoice.status === "posted" && (
                <Button
                  variant="outline"
                  disabled={acting}
                  onClick={() => runAction(unpostSalesInvoice)}
                >
                  {acting ? "Unposting…" : "Unpost"}
                </Button>
              )}
              <Button variant="outline" asChild>
                <Link to="/invoices">Back</Link>
              </Button>
            </div>
          </header>

          {actionError !== null && (
            <p className="mb-4 text-sm text-destructive" role="alert">
              {actionError}
            </p>
          )}

          {lines.length === 0 && (
            <p className="text-sm text-muted-foreground">
              This invoice has no lines.
            </p>
          )}

          {lines.length > 0 && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10">#</TableHead>
                  <TableHead>Description</TableHead>
                  <TableHead className="text-right">Qty</TableHead>
                  <TableHead className="text-right">Unit Price</TableHead>
                  <TableHead>Tax</TableHead>
                  <TableHead className="text-right">Tax Amount</TableHead>
                  <TableHead className="text-right">Line Total</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {lines.map((l) => (
                  <TableRow key={l.line_no}>
                    <TableCell className="text-muted-foreground">
                      {l.line_no}
                    </TableCell>
                    <TableCell className="font-medium">
                      {l.description}
                    </TableCell>
                    <AmountCell value={l.quantity} />
                    <AmountCell value={l.unit_price} />
                    <TableCell className="text-muted-foreground">
                      {l.tax_code ?? "—"}
                    </TableCell>
                    <AmountCell value={l.tax_amount} />
                    <AmountCell value={l.line_total} />
                  </TableRow>
                ))}
              </TableBody>
              <TableFooter>
                <TableRow>
                  <TableCell colSpan={6}>Subtotal</TableCell>
                  <AmountCell
                    value={sumAmounts(lines.map((l) => l.line_subtotal))}
                  />
                </TableRow>
                <TableRow>
                  <TableCell colSpan={6}>Tax</TableCell>
                  <AmountCell
                    value={sumAmounts(lines.map((l) => l.tax_amount))}
                  />
                </TableRow>
                <TableRow>
                  <TableCell colSpan={6}>Total</TableCell>
                  <AmountCell value={invoice.total} />
                </TableRow>
                <TableRow>
                  <TableCell colSpan={6}>Applied</TableCell>
                  <AmountCell value={invoice.amount_applied} />
                </TableRow>
                <TableRow>
                  <TableCell colSpan={6}>Balance due</TableCell>
                  <AmountCell value={invoice.balance} />
                </TableRow>
              </TableFooter>
            </Table>
          )}
        </>
      )}
    </section>
  )
}
