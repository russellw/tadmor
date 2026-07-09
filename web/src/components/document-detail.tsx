import { useCallback, useEffect, useState } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import { isZeroAmount, sumAmounts } from "@/lib/amount"
import { useCurrentUser } from "@/lib/current-user"
import {
  ApiError,
  applyPurchaseCreditNote,
  applySalesCreditNote,
  deletePurchaseBill,
  deletePurchaseCreditNote,
  deleteSalesCreditNote,
  deleteSalesInvoice,
  getPurchaseBill,
  getPurchaseBillLines,
  getPurchaseCreditNote,
  getPurchaseCreditNoteApplications,
  getPurchaseCreditNoteLines,
  getSalesCreditNote,
  getSalesCreditNoteApplications,
  getSalesCreditNoteLines,
  getSalesInvoice,
  getSalesInvoiceLines,
  postPurchaseBill,
  postPurchaseCreditNote,
  postSalesCreditNote,
  postSalesInvoice,
  unpostPurchaseBill,
  unpostPurchaseCreditNote,
  unpostSalesCreditNote,
  unpostSalesInvoice,
  type DocumentBalance,
  type PaymentApplication,
} from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
import {
  fetchCustomerNames,
  fetchSupplierNames,
  PaymentBadge,
  StatusBadge,
} from "@/components/document-list"
import { EmailDocumentPanel } from "@/components/email-document"
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
// order_line_id marks lines produced by order fulfilment: such documents are
// deletable while draft but not editable.
interface LineView {
  line_no: number
  description: string
  quantity: string
  unit_amount: string
  tax_code: string | null
  tax_amount: string
  line_subtotal: string
  line_total: string
  order_line_id: number | null
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

async function fetchCreditNoteLines(id: number): Promise<LineView[]> {
  return (await getSalesCreditNoteLines(id)).map((l) => ({
    ...l,
    unit_amount: l.unit_price,
  }))
}

async function fetchSupplierCreditLines(id: number): Promise<LineView[]> {
  return (await getPurchaseCreditNoteLines(id)).map((l) => ({
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
      deleteDocument={deleteSalesInvoice}
      pdfBasePath="/api/sales-invoices"
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
      deleteDocument={deletePurchaseBill}
      pdfBasePath="/api/purchase-bills"
    />
  )
}

export function CreditNoteDetail() {
  return (
    <DocumentDetail
      titlePrefix="Credit Note"
      backPath="/credit-notes"
      unitLabel="Unit Price"
      balanceLabel="Unapplied"
      fetchDocument={getSalesCreditNote}
      fetchLines={fetchCreditNoteLines}
      fetchPartyNames={fetchCustomerNames}
      post={postSalesCreditNote}
      unpost={unpostSalesCreditNote}
      deleteDocument={deleteSalesCreditNote}
      apply={applySalesCreditNote}
      fetchApplications={getSalesCreditNoteApplications}
      appliedDocLabel="Invoice"
      appliedDocBasePath="/invoices"
      applyHint="Apply allocates the unapplied credit to the customer's open invoices, oldest first."
      pdfBasePath="/api/sales-credit-notes"
    />
  )
}

export function SupplierCreditDetail() {
  return (
    <DocumentDetail
      titlePrefix="Supplier Credit"
      backPath="/supplier-credits"
      unitLabel="Unit Cost"
      balanceLabel="Unapplied"
      fetchDocument={getPurchaseCreditNote}
      fetchLines={fetchSupplierCreditLines}
      fetchPartyNames={fetchSupplierNames}
      post={postPurchaseCreditNote}
      unpost={unpostPurchaseCreditNote}
      deleteDocument={deletePurchaseCreditNote}
      apply={applyPurchaseCreditNote}
      fetchApplications={getPurchaseCreditNoteApplications}
      appliedDocLabel="Bill"
      appliedDocBasePath="/bills"
      applyHint="Apply allocates the unapplied credit to the supplier's open bills, oldest first."
      pdfBasePath="/api/purchase-credit-notes"
    />
  )
}

// One document: header, database-computed lines, and the lifecycle actions.
// Post writes the journal entry; Unpost reverses it and returns the document
// to draft. Drafts can be edited (unless order-linked) or deleted. Credit
// notes additionally pass apply/fetchApplications: Apply allocates the
// unapplied credit to open documents oldest-first, and the allocations render
// in an "Applied to" table.
function DocumentDetail({
  titlePrefix,
  backPath,
  unitLabel,
  balanceLabel = "Balance due",
  fetchDocument,
  fetchLines,
  fetchPartyNames,
  post,
  unpost,
  deleteDocument,
  apply,
  fetchApplications,
  appliedDocLabel = "",
  appliedDocBasePath = "",
  applyHint = "",
  pdfBasePath,
}: {
  titlePrefix: string
  backPath: string
  unitLabel: string
  balanceLabel?: string
  fetchDocument: (id: number) => Promise<DocumentBalance>
  fetchLines: (id: number) => Promise<LineView[]>
  fetchPartyNames: () => Promise<Map<number, string>>
  post: (id: number) => Promise<unknown>
  unpost: (id: number) => Promise<unknown>
  deleteDocument: (id: number) => Promise<void>
  apply?: (id: number) => Promise<unknown>
  fetchApplications?: (id: number) => Promise<PaymentApplication[]>
  appliedDocLabel?: string
  appliedDocBasePath?: string
  applyHint?: string
  /** API base path for a printable PDF; when set, a PDF button opens
   *  `${pdfBasePath}/${id}/pdf` in a new tab. */
  pdfBasePath?: string
}) {
  const { id } = useParams()
  const navigate = useNavigate()
  const documentId = Number(id)

  const [document, setDocument] = useState<DocumentBalance | null>(null)
  const [lines, setLines] = useState<LineView[] | null>(null)
  const [applications, setApplications] = useState<PaymentApplication[]>([])
  const [partyName, setPartyName] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [acting, setActing] = useState(false)
  const currentUser = useCurrentUser()
  const [actionError, setActionError] = useState<string | null>(null)
  const [emailOpen, setEmailOpen] = useState(false)
  // The email endpoint shares the PDF endpoint's collection segment (drop the
  // "/api/" the PDF href carries): the same six documents are printable and
  // emailable.
  const emailCollection = pdfBasePath?.replace(/^\/api\//, "")

  const load = useCallback(async () => {
    const [doc, lns, apps, names] = await Promise.all([
      fetchDocument(documentId),
      fetchLines(documentId),
      fetchApplications?.(documentId) ?? Promise.resolve([]),
      fetchPartyNames(),
    ])
    return {
      doc,
      lns,
      apps,
      name: names.get(doc.party_id) ?? `#${doc.party_id}`,
    }
  }, [documentId, fetchDocument, fetchLines, fetchApplications, fetchPartyNames])

  useEffect(() => {
    if (!Number.isInteger(documentId) || documentId <= 0) {
      setError("Invalid id.")
      return
    }
    let cancelled = false
    load()
      .then(({ doc, lns, apps, name }) => {
        if (cancelled) return
        setDocument(doc)
        setLines(lns)
        setApplications(apps)
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
      .then(({ doc, lns, apps, name }) => {
        setDocument(doc)
        setLines(lns)
        setApplications(apps)
        setPartyName(name)
        setActing(false)
      })
      .catch((err: unknown) => {
        setActing(false)
        setActionError(err instanceof ApiError ? err.message : String(err))
      })
  }

  function runDelete() {
    if (
      !window.confirm(
        `Delete this draft ${titlePrefix.toLowerCase()}? This cannot be undone.`,
      )
    ) {
      return
    }
    setActing(true)
    setActionError(null)
    deleteDocument(documentId)
      .then(() => navigate(backPath))
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
              {document.status === "draft" &&
                !lines.some((l) => l.order_line_id !== null) && (
                  <Button variant="outline" disabled={acting} asChild>
                    <Link to={`${backPath}/${documentId}/edit`}>Edit</Link>
                  </Button>
                )}
              {document.status === "draft" && (
                <Button
                  variant="outline"
                  disabled={acting}
                  onClick={runDelete}
                >
                  Delete
                </Button>
              )}
              {apply !== undefined &&
                document.status === "posted" &&
                !isZeroAmount(document.balance) && (
                  <Button disabled={acting} onClick={() => runAction(apply)}>
                    {acting ? "Applying…" : "Apply"}
                  </Button>
                )}
              {pdfBasePath !== undefined && (
                <Button variant="outline" asChild>
                  <a
                    href={`${pdfBasePath}/${documentId}/pdf`}
                    target="_blank"
                    rel="noopener"
                  >
                    PDF
                  </a>
                </Button>
              )}
              {emailCollection !== undefined && (
                <Button
                  variant="outline"
                  disabled={acting}
                  onClick={() => setEmailOpen((open) => !open)}
                >
                  Email
                </Button>
              )}
              {document.status === "posted" && currentUser.is_admin && (
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

          {emailOpen && emailCollection !== undefined && (
            <EmailDocumentPanel
              collection={emailCollection}
              documentId={documentId}
              label={titlePrefix}
              onClose={() => setEmailOpen(false)}
            />
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
                  <TableCell colSpan={6}>{balanceLabel}</TableCell>
                  <AmountCell value={document.balance} />
                </TableRow>
              </TableFooter>
            </Table>
          )}

          {fetchApplications !== undefined && (
            <div className="mt-8">
              <h2 className="mb-3 text-sm font-semibold">Applied to</h2>
              {applications.length === 0 && (
                <p className="text-sm text-muted-foreground">
                  Not applied to any {appliedDocLabel.toLowerCase()} yet.
                  {document.status === "posted" && ` ${applyHint}`}
                </p>
              )}
              {applications.length > 0 && (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{appliedDocLabel}</TableHead>
                      <TableHead className="text-right">
                        Amount Applied
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {applications.map((a) => (
                      <TableRow key={a.document_id}>
                        <TableCell className="font-mono">
                          <Link
                            to={`${appliedDocBasePath}/${a.document_id}`}
                            className="font-medium text-primary hover:underline"
                          >
                            {a.document_number}
                          </Link>
                        </TableCell>
                        <AmountCell value={a.amount_applied} />
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </div>
          )}
        </>
      )}
    </section>
  )
}
