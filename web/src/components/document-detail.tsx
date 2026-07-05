import { useCallback, useEffect, useState } from "react"
import { Link, useParams } from "react-router-dom"

import { sumAmounts } from "@/lib/amount"
import {
  ApiError,
  getPurchaseBill,
  getPurchaseBillLines,
  getSalesInvoice,
  getSalesInvoiceLines,
  postPurchaseBill,
  postSalesInvoice,
  unpostPurchaseBill,
  unpostSalesInvoice,
  type DocumentBalance,
} from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
import {
  fetchCustomerNames,
  fetchSupplierNames,
  PaymentBadge,
  StatusBadge,
} from "@/components/document-list"
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

// Invoice and bill lines differ only in the name of the per-unit money field
// (unit_price vs unit_cost), so the detail screen renders this common view.
interface LineView {
  line_no: number
  description: string
  quantity: string
  unit_amount: string
  tax_code: string | null
  tax_amount: string
  line_subtotal: string
  line_total: string
}

// Module-level so their identity is stable across renders (they are effect
// dependencies inside DocumentDetail).
async function fetchInvoiceLines(id: number): Promise<LineView[]> {
  return (await getSalesInvoiceLines(id)).map((l) => ({
    ...l,
    unit_amount: l.unit_price,
  }))
}

async function fetchBillLines(id: number): Promise<LineView[]> {
  return (await getPurchaseBillLines(id)).map((l) => ({
    ...l,
    unit_amount: l.unit_cost,
  }))
}

export function InvoiceDetail() {
  return (
    <DocumentDetail
      titlePrefix="Invoice"
      backPath="/invoices"
      unitLabel="Unit Price"
      fetchDocument={getSalesInvoice}
      fetchLines={fetchInvoiceLines}
      fetchPartyNames={fetchCustomerNames}
      post={postSalesInvoice}
      unpost={unpostSalesInvoice}
    />
  )
}

export function BillDetail() {
  return (
    <DocumentDetail
      titlePrefix="Bill"
      backPath="/bills"
      unitLabel="Unit Cost"
      fetchDocument={getPurchaseBill}
      fetchLines={fetchBillLines}
      fetchPartyNames={fetchSupplierNames}
      post={postPurchaseBill}
      unpost={unpostPurchaseBill}
    />
  )
}

// One document: header, database-computed lines, and the lifecycle actions.
// Post writes the journal entry; Unpost reverses it and returns the document
// to draft.
function DocumentDetail({
  titlePrefix,
  backPath,
  unitLabel,
  fetchDocument,
  fetchLines,
  fetchPartyNames,
  post,
  unpost,
}: {
  titlePrefix: string
  backPath: string
  unitLabel: string
  fetchDocument: (id: number) => Promise<DocumentBalance>
  fetchLines: (id: number) => Promise<LineView[]>
  fetchPartyNames: () => Promise<Map<number, string>>
  post: (id: number) => Promise<unknown>
  unpost: (id: number) => Promise<unknown>
}) {
  const { id } = useParams()
  const documentId = Number(id)

  const [document, setDocument] = useState<DocumentBalance | null>(null)
  const [lines, setLines] = useState<LineView[] | null>(null)
  const [partyName, setPartyName] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [acting, setActing] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)

  const load = useCallback(async () => {
    const [doc, lns, names] = await Promise.all([
      fetchDocument(documentId),
      fetchLines(documentId),
      fetchPartyNames(),
    ])
    return { doc, lns, name: names.get(doc.party_id) ?? `#${doc.party_id}` }
  }, [documentId, fetchDocument, fetchLines, fetchPartyNames])

  useEffect(() => {
    if (!Number.isInteger(documentId) || documentId <= 0) {
      setError("Invalid id.")
      return
    }
    let cancelled = false
    load()
      .then(({ doc, lns, name }) => {
        if (cancelled) return
        setDocument(doc)
        setLines(lns)
        setPartyName(name)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [documentId, load])

  function runAction(action: (id: number) => Promise<unknown>) {
    setActing(true)
    setActionError(null)
    action(documentId)
      .then(load)
      .then(({ doc, lns, name }) => {
        setDocument(doc)
        setLines(lns)
        setPartyName(name)
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
            <Link to={backPath}>Back</Link>
          </Button>
        </div>
      )}

      {error === null && (document === null || lines === null) && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {document !== null && lines !== null && (
        <>
          <header className="mb-6 flex items-start justify-between gap-4">
            <div>
              <div className="flex items-center gap-3">
                <h1 className="text-2xl font-semibold tracking-tight">
                  {titlePrefix} {document.number}
                </h1>
                <StatusBadge status={document.status} />
                <PaymentBadge status={document.payment_status} />
              </div>
              <p className="text-sm text-muted-foreground">
                {partyName} · {document.date}
                {document.due_date !== null && ` · due ${document.due_date}`}
                {" · "}
                {document.currency_code}
                {document.journal_entry_id !== null && (
                  <>
                    {" · "}
                    <Link
                      to={`/journal-entries/${document.journal_entry_id}`}
                      className="text-primary hover:underline"
                    >
                      journal entry #{document.journal_entry_id}
                    </Link>
                  </>
                )}
              </p>
            </div>
            <div className="flex gap-2">
              {document.status === "draft" && (
                <Button disabled={acting} onClick={() => runAction(post)}>
                  {acting ? "Posting…" : "Post to ledger"}
                </Button>
              )}
              {document.status === "posted" && (
                <Button
                  variant="outline"
                  disabled={acting}
                  onClick={() => runAction(unpost)}
                >
                  {acting ? "Unposting…" : "Unpost"}
                </Button>
              )}
              <Button variant="outline" asChild>
                <Link to={backPath}>Back</Link>
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
              This document has no lines.
            </p>
          )}

          {lines.length > 0 && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10">#</TableHead>
                  <TableHead>Description</TableHead>
                  <TableHead className="text-right">Qty</TableHead>
                  <TableHead className="text-right">{unitLabel}</TableHead>
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
                    <AmountCell value={l.unit_amount} />
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
                  <AmountCell value={document.total} />
                </TableRow>
                <TableRow>
                  <TableCell colSpan={6}>Applied</TableCell>
                  <AmountCell value={document.amount_applied} />
                </TableRow>
                <TableRow>
                  <TableCell colSpan={6}>Balance due</TableCell>
                  <AmountCell value={document.balance} />
                </TableRow>
              </TableFooter>
            </Table>
          )}
        </>
      )}
    </section>
  )
}
