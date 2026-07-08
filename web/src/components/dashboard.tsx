import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import {
  listCustomers,
  listOrganizations,
  listPurchaseBills,
  listPurchaseOrders,
  listSalesInvoices,
  listSalesOrders,
  listSuppliers,
  type DocumentBalance,
  type PurchaseOrderSummary,
  type SalesOrderSummary,
} from "@/lib/api"
import { formatAmount, isZeroAmount, sumAmounts } from "@/lib/amount"
import { daysAfter, today } from "@/lib/dates"
import { AmountCell } from "@/components/amount-cell"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { cn } from "@/lib/utils"

// The landing screen: outstanding balances, documents that need chasing or
// paying, and one-click starts for the everyday workflows. Everything here is
// derived client-side from the ordinary list endpoints; amounts are grouped
// per currency rather than summed across currencies (unlike the aging views,
// which mix currencies in one total).

const ATTENTION_ROWS = 5
const BILL_HORIZON_DAYS = 14

interface DashboardData {
  invoices: DocumentBalance[]
  bills: DocumentBalance[]
  salesOrders: SalesOrderSummary[]
  purchaseOrders: PurchaseOrderSummary[]
  customerNames: Map<number, string>
  supplierNames: Map<number, string>
}

// Posted documents still carrying a positive balance — the ones that are
// really outstanding. Void and draft documents never count.
function outstanding(docs: DocumentBalance[]): DocumentBalance[] {
  return docs.filter(
    (d) =>
      d.status === "posted" &&
      !isZeroAmount(d.balance) &&
      !d.balance.startsWith("-"),
  )
}

/** Sum balances per currency, alphabetical by currency code. */
function byCurrency(
  docs: DocumentBalance[],
): { currency: string; amount: string }[] {
  const groups = new Map<string, string[]>()
  for (const d of docs) {
    const list = groups.get(d.currency_code) ?? []
    list.push(d.balance)
    groups.set(d.currency_code, list)
  }
  return [...groups.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([currency, balances]) => ({ currency, amount: sumAmounts(balances) }))
}

export function Dashboard() {
  const [data, setData] = useState<DashboardData | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([
      listSalesInvoices(),
      listPurchaseBills(),
      listSalesOrders(),
      listPurchaseOrders(),
      listCustomers(),
      listSuppliers(),
      listOrganizations(),
    ])
      .then(([invoices, bills, salesOrders, purchaseOrders, customers, suppliers, orgs]) => {
        if (cancelled) return
        const orgNames = new Map(orgs.map((o) => [o.id, o.name]))
        setData({
          invoices,
          bills,
          salesOrders,
          purchaseOrders,
          customerNames: new Map(
            customers.map((c) => [
              c.id,
              orgNames.get(c.organization_id) ?? `#${c.id}`,
            ]),
          ),
          supplierNames: new Map(
            suppliers.map((s) => [
              s.id,
              orgNames.get(s.organization_id) ?? `#${s.id}`,
            ]),
          ),
        })
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
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">Home</h1>
        <p className="text-sm text-muted-foreground">
          Outstanding balances and work in progress across the business.
        </p>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load: {error}
        </p>
      )}

      {error === null && data === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {data !== null && <DashboardBody data={data} />}
    </section>
  )
}

function DashboardBody({ data }: { data: DashboardData }) {
  const todayStr = today()
  const billHorizon = daysAfter(todayStr, BILL_HORIZON_DAYS)

  const openInvoices = outstanding(data.invoices)
  const openBills = outstanding(data.bills)
  const overdueInvoices = openInvoices
    .filter((d) => d.due_date !== null && d.due_date < todayStr)
    .sort((a, b) => (a.due_date! < b.due_date! ? -1 : 1))
  const billsComingDue = openBills
    .filter((d) => d.due_date !== null && d.due_date <= billHorizon)
    .sort((a, b) => (a.due_date! < b.due_date! ? -1 : 1))

  const openSales = data.salesOrders.filter((o) => o.status === "open").length
  const openPurchases = data.purchaseOrders.filter(
    (o) => o.status === "open",
  ).length
  const draftInvoices = data.invoices.filter(
    (d) => d.status === "draft",
  ).length
  const draftBills = data.bills.filter((d) => d.status === "draft").length

  return (
    <div className="space-y-6">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          label="Receivables outstanding"
          amounts={byCurrency(openInvoices)}
          overdue={byCurrency(overdueInvoices)}
        />
        <StatCard
          label="Payables outstanding"
          amounts={byCurrency(openBills)}
          overdue={byCurrency(
            openBills.filter(
              (d) => d.due_date !== null && d.due_date < todayStr,
            ),
          )}
        />
        <CountCard
          label="Open orders"
          count={openSales + openPurchases}
          detail={`${openSales} sales · ${openPurchases} purchase`}
        />
        <CountCard
          label="Drafts to post"
          count={draftInvoices + draftBills}
          detail={`${draftInvoices} invoices · ${draftBills} bills`}
        />
      </div>

      <div className="grid items-start gap-6 lg:grid-cols-3">
        <div className="space-y-6 lg:col-span-2">
          <AttentionList
            title="Overdue invoices"
            emptyMessage="Nothing overdue."
            partyLabel="Customer"
            basePath="/invoices"
            documents={overdueInvoices}
            partyNames={data.customerNames}
            today={todayStr}
            reportLabel="AR aging report"
            reportPath="/reports/ar-aging"
          />
          <AttentionList
            title={`Bills due within ${BILL_HORIZON_DAYS} days`}
            emptyMessage="Nothing coming due."
            partyLabel="Supplier"
            basePath="/bills"
            documents={billsComingDue}
            partyNames={data.supplierNames}
            today={todayStr}
            reportLabel="AP aging report"
            reportPath="/reports/ap-aging"
          />
        </div>

        <div className="rounded-lg border bg-card p-4">
          <h2 className="mb-3 text-sm font-medium">Quick actions</h2>
          <div className="flex flex-col gap-2">
            <QuickAction to="/invoices/new" label="New invoice" />
            <QuickAction
              to="/customer-payments/new"
              label="Record customer payment"
            />
            <QuickAction to="/bills/new" label="New bill" />
            <QuickAction to="/supplier-payments/new" label="Pay a supplier" />
            <QuickAction to="/sales-orders/new" label="New sales order" />
            <QuickAction to="/purchase-orders/new" label="New purchase order" />
          </div>
        </div>
      </div>
    </div>
  )
}

function StatCard({
  label,
  amounts,
  overdue,
}: {
  label: string
  amounts: { currency: string; amount: string }[]
  overdue: { currency: string; amount: string }[]
}) {
  return (
    <div className="rounded-lg border bg-card p-4">
      <p className="text-sm text-muted-foreground">{label}</p>
      <div className="mt-1 space-y-0.5">
        {amounts.length === 0 && (
          <p className="text-2xl font-semibold tabular-nums text-muted-foreground">
            0.00
          </p>
        )}
        {amounts.map(({ currency, amount }) => (
          <p key={currency} className="text-2xl font-semibold tabular-nums">
            {formatAmount(amount)}{" "}
            <span className="text-sm font-normal text-muted-foreground">
              {currency}
            </span>
          </p>
        ))}
      </div>
      {overdue.length > 0 ? (
        <p className="mt-1 text-sm text-destructive">
          Overdue:{" "}
          {overdue
            .map(({ currency, amount }) => `${formatAmount(amount)} ${currency}`)
            .join(" · ")}
        </p>
      ) : (
        <p className="mt-1 text-sm text-muted-foreground">Nothing overdue</p>
      )}
    </div>
  )
}

function CountCard({
  label,
  count,
  detail,
}: {
  label: string
  count: number
  detail: string
}) {
  return (
    <div className="rounded-lg border bg-card p-4">
      <p className="text-sm text-muted-foreground">{label}</p>
      <p
        className={cn(
          "mt-1 text-2xl font-semibold tabular-nums",
          count === 0 && "text-muted-foreground",
        )}
      >
        {count}
      </p>
      <p className="mt-1 text-sm text-muted-foreground">{detail}</p>
    </div>
  )
}

function AttentionList({
  title,
  emptyMessage,
  partyLabel,
  basePath,
  documents,
  partyNames,
  today,
  reportLabel,
  reportPath,
}: {
  title: string
  emptyMessage: string
  partyLabel: string
  basePath: string
  documents: DocumentBalance[]
  partyNames: Map<number, string>
  today: string
  reportLabel: string
  reportPath: string
}) {
  const shown = documents.slice(0, ATTENTION_ROWS)
  return (
    <div className="rounded-lg border bg-card">
      <div className="flex items-baseline justify-between gap-4 px-4 pt-4">
        <h2 className="text-sm font-medium">{title}</h2>
        <Link
          to={reportPath}
          className="text-sm text-muted-foreground hover:text-foreground hover:underline"
        >
          {reportLabel}
        </Link>
      </div>
      {shown.length === 0 ? (
        <p className="px-4 pb-4 pt-2 text-sm text-muted-foreground">
          {emptyMessage}
        </p>
      ) : (
        <div className="p-2">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Number</TableHead>
                <TableHead>{partyLabel}</TableHead>
                <TableHead>Due</TableHead>
                <TableHead className="text-right">Balance</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {shown.map((doc) => (
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
                  <TableCell
                    className={cn(
                      doc.due_date !== null && doc.due_date < today
                        ? "text-destructive"
                        : "text-muted-foreground",
                    )}
                  >
                    {doc.due_date}
                  </TableCell>
                  <AmountCell value={doc.balance} />
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {documents.length > shown.length && (
            <p className="px-2 pb-2 pt-1 text-sm text-muted-foreground">
              And {documents.length - shown.length} more — see the{" "}
              <Link to={reportPath} className="hover:underline">
                {reportLabel.toLowerCase()}
              </Link>
              .
            </p>
          )}
        </div>
      )}
    </div>
  )
}

function QuickAction({ to, label }: { to: string; label: string }) {
  return (
    <Button asChild variant="outline" className="justify-start">
      <Link to={to}>{label}</Link>
    </Button>
  )
}
