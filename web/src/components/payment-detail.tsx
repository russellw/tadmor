import { useCallback, useEffect, useState } from "react"
import { Link, useParams } from "react-router-dom"

import { isZeroAmount } from "@/lib/amount"
import { useCurrentUser } from "@/lib/current-user"
import {
  ApiError,
  applyCustomerPayment,
  applySupplierPayment,
  getCustomerPayment,
  getCustomerPaymentApplications,
  getSupplierPayment,
  getSupplierPaymentApplications,
  postCustomerPayment,
  postSupplierPayment,
  unpostCustomerPayment,
  unpostSupplierPayment,
  type Payment,
  type PaymentApplication,
} from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
import {
  fetchCustomerNames,
  fetchSupplierNames,
  StatusBadge,
} from "@/components/document-list"
import { AppliedBadge } from "@/components/payment-list"
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

export function CustomerPaymentDetail() {
  return (
    <PaymentDetail
      titlePrefix="Customer Payment"
      basePath="/customer-payments"
      documentBasePath="/invoices"
      documentLabel="Invoice"
      applyHint="Apply allocates the unapplied remainder to the customer's open invoices, oldest first."
      fetchPayment={getCustomerPayment}
      fetchApplications={getCustomerPaymentApplications}
      fetchPartyNames={fetchCustomerNames}
      post={postCustomerPayment}
      unpost={unpostCustomerPayment}
      apply={applyCustomerPayment}
    />
  )
}

export function SupplierPaymentDetail() {
  return (
    <PaymentDetail
      titlePrefix="Supplier Payment"
      basePath="/supplier-payments"
      documentBasePath="/bills"
      documentLabel="Bill"
      applyHint="Apply allocates the unapplied remainder to the supplier's open bills, oldest first."
      fetchPayment={getSupplierPayment}
      fetchApplications={getSupplierPaymentApplications}
      fetchPartyNames={fetchSupplierNames}
      post={postSupplierPayment}
      unpost={unpostSupplierPayment}
      apply={applySupplierPayment}
    />
  )
}

// One payment: header, what it has been applied to, and the lifecycle
// actions. Post writes the journal entry; Apply allocates the remainder to
// open documents oldest-first; Unpost reverses the entry, deletes any
// applications, and returns the payment to draft.
function PaymentDetail({
  titlePrefix,
  basePath,
  documentBasePath,
  documentLabel,
  applyHint,
  fetchPayment,
  fetchApplications,
  fetchPartyNames,
  post,
  unpost,
  apply,
}: {
  titlePrefix: string
  basePath: string
  documentBasePath: string
  documentLabel: string
  applyHint: string
  fetchPayment: (id: number) => Promise<Payment>
  fetchApplications: (id: number) => Promise<PaymentApplication[]>
  fetchPartyNames: () => Promise<Map<number, string>>
  post: (id: number) => Promise<unknown>
  unpost: (id: number) => Promise<unknown>
  apply: (id: number) => Promise<unknown>
}) {
  const { id } = useParams()
  const paymentId = Number(id)

  const [payment, setPayment] = useState<Payment | null>(null)
  const [applications, setApplications] = useState<PaymentApplication[] | null>(
    null,
  )
  const [partyName, setPartyName] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [acting, setActing] = useState(false)
  const currentUser = useCurrentUser()
  const [actionError, setActionError] = useState<string | null>(null)

  const load = useCallback(async () => {
    const [p, apps, names] = await Promise.all([
      fetchPayment(paymentId),
      fetchApplications(paymentId),
      fetchPartyNames(),
    ])
    return { p, apps, name: names.get(p.party_id) ?? `#${p.party_id}` }
  }, [paymentId, fetchPayment, fetchApplications, fetchPartyNames])

  useEffect(() => {
    if (!Number.isInteger(paymentId) || paymentId <= 0) {
      setError("Invalid id.")
      return
    }
    let cancelled = false
    load()
      .then(({ p, apps, name }) => {
        if (cancelled) return
        setPayment(p)
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
  }, [paymentId, load])

  function runAction(action: (id: number) => Promise<unknown>) {
    setActing(true)
    setActionError(null)
    action(paymentId)
      .then(load)
      .then(({ p, apps, name }) => {
        setPayment(p)
        setApplications(apps)
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
            <Link to={basePath}>Back</Link>
          </Button>
        </div>
      )}

      {error === null && (payment === null || applications === null) && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {payment !== null && applications !== null && (
        <>
          <header className="mb-6 flex items-start justify-between gap-4">
            <div>
              <div className="flex items-center gap-3">
                <h1 className="text-2xl font-semibold tracking-tight">
                  {titlePrefix} #{payment.id}
                </h1>
                <StatusBadge status={payment.status} />
                <AppliedBadge payment={payment} />
              </div>
              <p className="text-sm text-muted-foreground">
                {partyName} · {payment.date}
                {payment.method !== null && ` · ${payment.method}`}
                {payment.reference !== null && ` · ${payment.reference}`}
                {" · "}
                {payment.currency_code}
                {payment.journal_entry_id !== null && (
                  <>
                    {" · "}
                    <Link
                      to={`/journal-entries/${payment.journal_entry_id}`}
                      className="text-primary hover:underline"
                    >
                      journal entry #{payment.journal_entry_id}
                    </Link>
                  </>
                )}
              </p>
            </div>
            <div className="flex gap-2">
              {payment.status === "draft" && (
                <Button disabled={acting} onClick={() => runAction(post)}>
                  {acting ? "Posting…" : "Post to ledger"}
                </Button>
              )}
              {payment.status === "posted" &&
                !isZeroAmount(payment.unapplied) && (
                  <Button disabled={acting} onClick={() => runAction(apply)}>
                    {acting ? "Applying…" : "Apply"}
                  </Button>
                )}
              {payment.status === "posted" && currentUser.is_admin && (
                <Button
                  variant="outline"
                  disabled={acting}
                  onClick={() => runAction(unpost)}
                >
                  {acting ? "Unposting…" : "Unpost"}
                </Button>
              )}
              <Button variant="outline" asChild>
                <Link to={basePath}>Back</Link>
              </Button>
            </div>
          </header>

          {actionError !== null && (
            <p className="mb-4 text-sm text-destructive" role="alert">
              {actionError}
            </p>
          )}

          {applications.length === 0 && (
            <p className="mb-4 text-sm text-muted-foreground">
              Not applied to any {documentLabel.toLowerCase()} yet.
              {payment.status === "posted" && ` ${applyHint}`}
            </p>
          )}

          <Table>
            {applications.length > 0 && (
              <>
                <TableHeader>
                  <TableRow>
                    <TableHead>{documentLabel}</TableHead>
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
                          to={`${documentBasePath}/${a.document_id}`}
                          className="font-medium text-primary hover:underline"
                        >
                          {a.document_number}
                        </Link>
                      </TableCell>
                      <AmountCell value={a.amount_applied} />
                    </TableRow>
                  ))}
                </TableBody>
              </>
            )}
            <TableFooter>
              <TableRow>
                <TableCell>Amount</TableCell>
                <AmountCell value={payment.amount} />
              </TableRow>
              <TableRow>
                <TableCell>Applied</TableCell>
                <AmountCell value={payment.amount_applied} />
              </TableRow>
              <TableRow>
                <TableCell>Unapplied</TableCell>
                <AmountCell value={payment.unapplied} />
              </TableRow>
            </TableFooter>
          </Table>
        </>
      )}
    </section>
  )
}
