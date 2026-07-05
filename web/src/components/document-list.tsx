import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import {
  listCustomers,
  listOrganizations,
  listPurchaseBills,
  listSalesInvoices,
  listSuppliers,
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

// Sales invoices and purchase bills share the balance-view row shape
// (api.DocumentBalance), so both lists render through one parameterized
// component; only the fetches, labels, and routes differ.

export function Invoices() {
  return (
    <DocumentList
      title="Sales Invoices"
      description="Draft and posted invoices, newest first."
      partyLabel="Customer"
      newLabel="New invoice"
      basePath="/invoices"
      emptyMessage="No invoices yet."
      fetchDocuments={listSalesInvoices}
      fetchPartyNames={fetchCustomerNames}
    />
  )
}

export function Bills() {
  return (
    <DocumentList
      title="Purchase Bills"
      description="Draft and posted bills, newest first."
      partyLabel="Supplier"
      newLabel="New bill"
      basePath="/bills"
      emptyMessage="No bills yet."
      fetchDocuments={listPurchaseBills}
      fetchPartyNames={fetchSupplierNames}
    />
  )
}

// A document names its party only by id; the display name lives on the
// organization behind the customer/supplier role, so join client-side.
export async function fetchCustomerNames(): Promise<Map<number, string>> {
  const [customers, orgs] = await Promise.all([
    listCustomers(),
    listOrganizations(),
  ])
  const orgNames = new Map(orgs.map((o) => [o.id, o.name]))
  return new Map(
    customers.map((c) => [c.id, orgNames.get(c.organization_id) ?? `#${c.id}`]),
  )
}

export async function fetchSupplierNames(): Promise<Map<number, string>> {
  const [suppliers, orgs] = await Promise.all([
    listSuppliers(),
    listOrganizations(),
  ])
  const orgNames = new Map(orgs.map((o) => [o.id, o.name]))
  return new Map(
    suppliers.map((s) => [s.id, orgNames.get(s.organization_id) ?? `#${s.id}`]),
  )
}

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

function DocumentList({
  title,
  description,
  partyLabel,
  newLabel,
  basePath,
  emptyMessage,
  fetchDocuments,
  fetchPartyNames,
}: {
  title: string
  description: string
  partyLabel: string
  newLabel: string
  basePath: string
  emptyMessage: string
  fetchDocuments: () => Promise<DocumentBalance[]>
  fetchPartyNames: () => Promise<Map<number, string>>
}) {
  const [documents, setDocuments] = useState<DocumentBalance[] | null>(null)
  const [partyNames, setPartyNames] = useState<Map<number, string>>(new Map())
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([fetchDocuments(), fetchPartyNames()])
      .then(([docs, names]) => {
        if (cancelled) return
        setPartyNames(names)
        setDocuments(docs)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [fetchDocuments, fetchPartyNames])

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

      {error === null && documents === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {documents !== null && documents.length === 0 && (
        <p className="text-sm text-muted-foreground">{emptyMessage}</p>
      )}

      {documents !== null && documents.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Number</TableHead>
              <TableHead>{partyLabel}</TableHead>
              <TableHead>Date</TableHead>
              <TableHead>Due</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Payment</TableHead>
              <TableHead className="text-right">Total</TableHead>
              <TableHead className="text-right">Balance</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {documents.map((doc) => (
              <TableRow key={doc.id}>
                <TableCell className="font-mono">
                  <Link
                    to={`${basePath}/${doc.id}`}
                    className="font-medium text-primary hover:underline"
                  >
                    {doc.number}
                  </Link>
                </TableCell>
                <TableCell className="font-medium">
                  {partyNames.get(doc.party_id) ?? `#${doc.party_id}`}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {doc.date}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {doc.due_date ?? "—"}
                </TableCell>
                <TableCell>
                  <StatusBadge status={doc.status} />
                </TableCell>
                <TableCell>
                  <PaymentBadge status={doc.payment_status} />
                </TableCell>
                <AmountCell value={doc.total} />
                <AmountCell value={doc.balance} />
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </section>
  )
}
