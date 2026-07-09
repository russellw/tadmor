import { useCallback, useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import {
  ApiError,
  billPurchaseOrder,
  cancelPurchaseOrder,
  cancelSalesOrder,
  closePurchaseOrder,
  closeSalesOrder,
  confirmPurchaseOrder,
  confirmSalesOrder,
  deletePurchaseOrder,
  deleteSalesOrder,
  getPurchaseOrder,
  getPurchaseOrderLines,
  getSalesOrder,
  getSalesOrderLines,
  invoiceSalesOrder,
  listWarehouses,
  receivePurchaseOrder,
  shipSalesOrder,
  type OrderLineQty,
  type Warehouse,
} from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
import {
  fetchCustomerNames,
  fetchSupplierNames,
} from "@/components/document-list"
import { EmailDocumentPanel } from "@/components/email-document"
import { FulfilmentBadge, OrderStatusBadge } from "@/components/order-list"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// Normalized shapes so one component renders both order kinds. The two fulfilment
// axes are flattened to a document axis (invoice/bill) and a stock axis
// (ship/receive).
interface OrderHeaderView {
  id: number
  order_number: string
  party_id: number
  order_date: string
  currency_code: string
  status: string
  total: string
  docStatus: string
  moveStatus: string
}

interface OrderLineView {
  line_no: number
  order_line_id: number
  description: string
  quantity: string
  unit_amount: string
  tax_code: string | null
  tax_amount: string
  line_total: string
  qtyDocd: string
  qtyToDoc: string
  qtyMoved: string
  qtyToMove: string
}

interface OrderConfig {
  kind: "sales" | "purchase"
  titlePrefix: string
  basePath: string
  unitLabel: string
  // Column labels for the two fulfilment axes.
  docDoneLabel: string // "Invoiced" | "Billed"
  docTodoLabel: string // "To invoice" | "To bill"
  moveDoneLabel: string // "Shipped" | "Received"
  moveTodoLabel: string // "To ship" | "To receive"
  // The document a fulfilment produces.
  docLabel: string // "Invoice" | "Bill"
  docBasePath: string // "/invoices" | "/bills"
  moveLabel: string // "Ship" | "Receive"
  fetchHeader: (id: number) => Promise<OrderHeaderView>
  fetchLines: (id: number) => Promise<OrderLineView[]>
  fetchPartyNames: () => Promise<Map<number, string>>
  confirm: (id: number) => Promise<unknown>
  close: (id: number) => Promise<unknown>
  cancel: (id: number) => Promise<unknown>
  deleteOrder: (id: number) => Promise<void>
  // lines carries the chosen per-line quantities; the backend caps each at the
  // line's outstanding amount, so full-remaining and partial both flow through.
  createDoc: (
    id: number,
    number: string,
    date: string,
    due: string | null,
    lines: OrderLineQty[],
  ) => Promise<number> // returns the new document id
  createMovements: (
    id: number,
    warehouseId: number,
    date: string | null,
    lines: OrderLineQty[],
  ) => Promise<number[]> // returns the created movement ids
}

export function SalesOrderDetail() {
  return (
    <OrderDetail
      config={{
        kind: "sales",
        titlePrefix: "Sales Order",
        basePath: "/sales-orders",
        unitLabel: "Unit Price",
        docDoneLabel: "Invoiced",
        docTodoLabel: "To invoice",
        moveDoneLabel: "Shipped",
        moveTodoLabel: "To ship",
        docLabel: "Invoice",
        docBasePath: "/invoices",
        moveLabel: "Ship",
        fetchHeader: async (id) => salesHeader(await getSalesOrder(id)),
        fetchLines: async (id) =>
          (await getSalesOrderLines(id)).map((l) => ({
            line_no: l.line_no,
            order_line_id: l.order_line_id,
            description: l.description,
            quantity: l.quantity,
            unit_amount: l.unit_price,
            tax_code: l.tax_code,
            tax_amount: l.tax_amount,
            line_total: l.line_total,
            qtyDocd: l.qty_invoiced,
            qtyToDoc: l.qty_to_invoice,
            qtyMoved: l.qty_shipped,
            qtyToMove: l.qty_to_ship,
          })),
        fetchPartyNames: fetchCustomerNames,
        confirm: confirmSalesOrder,
        close: closeSalesOrder,
        cancel: cancelSalesOrder,
        deleteOrder: deleteSalesOrder,
        createDoc: async (id, number, date, due, lines) =>
          (
            await invoiceSalesOrder(id, {
              invoice_number: number,
              invoice_date: date,
              due_date: due,
              lines,
            })
          ).invoice_id,
        createMovements: async (id, warehouseId, date, lines) =>
          (
            await shipSalesOrder(id, {
              warehouse_id: warehouseId,
              movement_date: date,
              reference: null,
              lines,
            })
          ).movement_ids,
      }}
    />
  )
}

export function PurchaseOrderDetail() {
  return (
    <OrderDetail
      config={{
        kind: "purchase",
        titlePrefix: "Purchase Order",
        basePath: "/purchase-orders",
        unitLabel: "Unit Cost",
        docDoneLabel: "Billed",
        docTodoLabel: "To bill",
        moveDoneLabel: "Received",
        moveTodoLabel: "To receive",
        docLabel: "Bill",
        docBasePath: "/bills",
        moveLabel: "Receive",
        fetchHeader: async (id) => purchaseHeader(await getPurchaseOrder(id)),
        fetchLines: async (id) =>
          (await getPurchaseOrderLines(id)).map((l) => ({
            line_no: l.line_no,
            order_line_id: l.order_line_id,
            description: l.description,
            quantity: l.quantity,
            unit_amount: l.unit_cost,
            tax_code: l.tax_code,
            tax_amount: l.tax_amount,
            line_total: l.line_total,
            qtyDocd: l.qty_billed,
            qtyToDoc: l.qty_to_bill,
            qtyMoved: l.qty_received,
            qtyToMove: l.qty_to_receive,
          })),
        fetchPartyNames: fetchSupplierNames,
        confirm: confirmPurchaseOrder,
        close: closePurchaseOrder,
        cancel: cancelPurchaseOrder,
        deleteOrder: deletePurchaseOrder,
        createDoc: async (id, number, date, due, lines) =>
          (
            await billPurchaseOrder(id, {
              bill_number: number,
              bill_date: date,
              due_date: due,
              lines,
            })
          ).bill_id,
        createMovements: async (id, warehouseId, date, lines) =>
          (
            await receivePurchaseOrder(id, {
              warehouse_id: warehouseId,
              movement_date: date,
              reference: null,
              lines,
            })
          ).movement_ids,
      }}
    />
  )
}

function salesHeader(o: {
  id: number
  order_number: string
  customer_id: number
  order_date: string
  currency_code: string
  status: string
  total: string
  invoiced_status: string
  shipped_status: string
}): OrderHeaderView {
  return {
    id: o.id,
    order_number: o.order_number,
    party_id: o.customer_id,
    order_date: o.order_date,
    currency_code: o.currency_code,
    status: o.status,
    total: o.total,
    docStatus: o.invoiced_status,
    moveStatus: o.shipped_status,
  }
}

function purchaseHeader(o: {
  id: number
  order_number: string
  supplier_id: number
  order_date: string
  currency_code: string
  status: string
  total: string
  billed_status: string
  received_status: string
}): OrderHeaderView {
  return {
    id: o.id,
    order_number: o.order_number,
    party_id: o.supplier_id,
    order_date: o.order_date,
    currency_code: o.currency_code,
    status: o.status,
    total: o.total,
    docStatus: o.billed_status,
    moveStatus: o.received_status,
  }
}

const today = () => new Date().toISOString().slice(0, 10)

// One outstanding order line offered for fulfilment in a panel.
interface FulfilLine {
  order_line_id: number
  line_no: number
  description: string
  remaining: string
}

// Drop the trailing zeros the database pads decimals with ("10.0000" -> "10",
// "2.5000" -> "2.5") so the pre-filled quantity inputs read naturally.
function trimZeros(s: string): string {
  return s.includes(".") ? s.replace(/\.?0+$/, "") : s
}

function OrderDetail({ config }: { config: OrderConfig }) {
  const { id } = useParams()
  const orderId = Number(id)
  const navigate = useNavigate()

  const [order, setOrder] = useState<OrderHeaderView | null>(null)
  const [lines, setLines] = useState<OrderLineView[] | null>(null)
  const [partyName, setPartyName] = useState<string | null>(null)
  const [warehouses, setWarehouses] = useState<Warehouse[]>([])
  const [error, setError] = useState<string | null>(null)
  const [acting, setActing] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)
  // Which inline panel is open (fulfilment or email), and a note after a stock
  // movement.
  const [panel, setPanel] = useState<"none" | "doc" | "move" | "email">("none")
  const [movementNote, setMovementNote] = useState<number[] | null>(null)

  const load = useCallback(async () => {
    const [hdr, lns, names] = await Promise.all([
      config.fetchHeader(orderId),
      config.fetchLines(orderId),
      config.fetchPartyNames(),
    ])
    return { hdr, lns, name: names.get(hdr.party_id) ?? `#${hdr.party_id}` }
  }, [orderId, config])

  useEffect(() => {
    if (!Number.isInteger(orderId) || orderId <= 0) {
      setError("Invalid id.")
      return
    }
    let cancelled = false
    Promise.all([load(), listWarehouses()])
      .then(([{ hdr, lns, name }, whs]) => {
        if (cancelled) return
        setOrder(hdr)
        setLines(lns)
        setPartyName(name)
        setWarehouses(whs.filter((w) => w.is_active))
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err))
      })
    return () => {
      cancelled = true
    }
  }, [orderId, load])

  function reload() {
    load()
      .then(({ hdr, lns, name }) => {
        setOrder(hdr)
        setLines(lns)
        setPartyName(name)
      })
      .catch((err: unknown) =>
        setActionError(err instanceof Error ? err.message : String(err)),
      )
  }

  function runLifecycle(action: (id: number) => Promise<unknown>) {
    setActing(true)
    setActionError(null)
    setMovementNote(null)
    action(orderId)
      .then(() => {
        setActing(false)
        setPanel("none")
        reload()
      })
      .catch((err: unknown) => {
        setActing(false)
        setActionError(err instanceof ApiError ? err.message : String(err))
      })
  }

  function runDelete() {
    if (
      !window.confirm(
        `Delete this draft ${config.titlePrefix.toLowerCase()}? This cannot be undone.`,
      )
    ) {
      return
    }
    setActing(true)
    setActionError(null)
    config
      .deleteOrder(orderId)
      .then(() => navigate(config.basePath))
      .catch((err: unknown) => {
        setActing(false)
        setActionError(err instanceof ApiError ? err.message : String(err))
      })
  }

  if (error !== null) {
    return (
      <section className="mx-auto w-full max-w-5xl p-6">
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to={config.basePath}>Back</Link>
          </Button>
        </div>
      </section>
    )
  }

  if (order === null || lines === null) {
    return (
      <section className="mx-auto w-full max-w-5xl p-6">
        <p className="text-sm text-muted-foreground">Loading…</p>
      </section>
    )
  }

  const isOpen = order.status === "open"
  const isDraft = order.status === "draft"

  // Lines still open on each axis, offered for (partial) fulfilment.
  const fulfilLine = (l: OrderLineView, remaining: string): FulfilLine => ({
    order_line_id: l.order_line_id,
    line_no: l.line_no,
    description: l.description,
    remaining,
  })
  const docLines = lines
    .filter((l) => Number(l.qtyToDoc) > 0)
    .map((l) => fulfilLine(l, l.qtyToDoc))
  const moveLines = lines
    .filter((l) => Number(l.qtyToMove) > 0)
    .map((l) => fulfilLine(l, l.qtyToMove))

  return (
    <section className="mx-auto w-full max-w-5xl p-6">
      <header className="mb-6 flex items-start justify-between gap-4">
        <div>
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="text-2xl font-semibold tracking-tight">
              {config.titlePrefix} {order.order_number}
            </h1>
            <OrderStatusBadge status={order.status} />
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {partyName} · {order.order_date} · {order.currency_code}
          </p>
          <p className="mt-2 flex items-center gap-2 text-sm">
            <span className="text-muted-foreground">{config.docDoneLabel}:</span>
            <FulfilmentBadge status={order.docStatus} />
            <span className="ml-2 text-muted-foreground">
              {config.moveDoneLabel}:
            </span>
            <FulfilmentBadge status={order.moveStatus} />
          </p>
        </div>
        <div className="flex flex-wrap justify-end gap-2">
          {isDraft && (
            <Button
              disabled={acting}
              onClick={() => runLifecycle(config.confirm)}
            >
              {acting ? "Working…" : "Confirm"}
            </Button>
          )}
          {isDraft && (
            <>
              <Button variant="outline" disabled={acting} asChild>
                <Link to={`${config.basePath}/${orderId}/edit`}>Edit</Link>
              </Button>
              <Button variant="outline" disabled={acting} onClick={runDelete}>
                Delete
              </Button>
            </>
          )}
          {isOpen && (
            <>
              <Button
                disabled={acting}
                onClick={() => {
                  setPanel(panel === "doc" ? "none" : "doc")
                  setActionError(null)
                  setMovementNote(null)
                }}
              >
                Create {config.docLabel}
              </Button>
              <Button
                variant="secondary"
                disabled={acting}
                onClick={() => {
                  setPanel(panel === "move" ? "none" : "move")
                  setActionError(null)
                  setMovementNote(null)
                }}
              >
                {config.moveLabel}
              </Button>
              <Button
                variant="outline"
                disabled={acting}
                onClick={() => runLifecycle(config.close)}
              >
                Close
              </Button>
            </>
          )}
          {(isDraft || isOpen) && (
            <Button
              variant="outline"
              disabled={acting}
              onClick={() => runLifecycle(config.cancel)}
            >
              Cancel order
            </Button>
          )}
          <Button variant="outline" asChild>
            <a
              href={`/api${config.basePath}/${orderId}/pdf`}
              target="_blank"
              rel="noopener"
            >
              PDF
            </a>
          </Button>
          <Button
            variant="outline"
            disabled={acting}
            onClick={() => {
              setPanel(panel === "email" ? "none" : "email")
              setActionError(null)
              setMovementNote(null)
            }}
          >
            Email
          </Button>
          <Button variant="outline" asChild>
            <Link to={config.basePath}>Back</Link>
          </Button>
        </div>
      </header>

      {actionError !== null && (
        <p className="mb-4 text-sm text-destructive" role="alert">
          {actionError}
        </p>
      )}

      {movementNote !== null && (
        <p className="mb-4 text-sm">
          Created{" "}
          {movementNote.map((mid, i) => (
            <span key={mid}>
              {i > 0 && ", "}
              <Link
                to={`/stock-movements/${mid}`}
                className="text-primary hover:underline"
              >
                movement #{mid}
              </Link>
            </span>
          ))}
          . Post {movementNote.length > 1 ? "them" : "it"} from the stock screen
          to hit the ledger.
        </p>
      )}

      {panel === "doc" && (
        <CreateDocPanel
          config={config}
          orderId={orderId}
          fulfilLines={docLines}
          onCancel={() => setPanel("none")}
          onCreated={(docId) => navigate(`${config.docBasePath}/${docId}`)}
        />
      )}

      {panel === "move" && (
        <MovePanel
          config={config}
          orderId={orderId}
          warehouses={warehouses}
          fulfilLines={moveLines}
          onCancel={() => setPanel("none")}
          onCreated={(ids) => {
            setPanel("none")
            setMovementNote(ids)
            reload()
          }}
        />
      )}

      {panel === "email" && (
        <EmailDocumentPanel
          collection={config.basePath.replace(/^\//, "")}
          documentId={orderId}
          label={config.titlePrefix}
          onClose={() => setPanel("none")}
        />
      )}

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-10">#</TableHead>
            <TableHead>Description</TableHead>
            <TableHead className="text-right">Ordered</TableHead>
            <TableHead className="text-right">{config.unitLabel}</TableHead>
            <TableHead className="text-right">Line Total</TableHead>
            <TableHead className="text-right">{config.docDoneLabel}</TableHead>
            <TableHead className="text-right">{config.docTodoLabel}</TableHead>
            <TableHead className="text-right">{config.moveDoneLabel}</TableHead>
            <TableHead className="text-right">{config.moveTodoLabel}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {lines.map((l) => (
            <TableRow key={l.line_no}>
              <TableCell className="text-muted-foreground">
                {l.line_no}
              </TableCell>
              <TableCell className="font-medium">{l.description}</TableCell>
              <AmountCell value={l.quantity} />
              <AmountCell value={l.unit_amount} />
              <AmountCell value={l.line_total} />
              <AmountCell value={l.qtyDocd} />
              <AmountCell value={l.qtyToDoc} />
              <AmountCell value={l.qtyMoved} />
              <AmountCell value={l.qtyToMove} />
            </TableRow>
          ))}
        </TableBody>
        <TableFooter>
          <TableRow>
            <TableCell colSpan={4}>Total</TableCell>
            <AmountCell value={order.total} />
            <TableCell colSpan={4} />
          </TableRow>
        </TableFooter>
      </Table>
    </section>
  )
}

// Per-line quantity state for a fulfilment panel. Each outstanding line starts
// pre-filled with its full remaining amount, so submitting unchanged fulfils
// everything; lowering (or clearing) a line fulfils part. chosen() returns the
// lines with a positive quantity, ready for the API.
function useFulfilQuantities(fulfilLines: FulfilLine[]) {
  const [qty, setQty] = useState<Record<number, string>>(() =>
    Object.fromEntries(
      fulfilLines.map((l) => [l.order_line_id, trimZeros(l.remaining)]),
    ),
  )
  const set = (orderLineId: number, value: string) =>
    setQty((q) => ({ ...q, [orderLineId]: value }))
  const chosen = (): OrderLineQty[] =>
    fulfilLines
      .map((l) => ({
        order_line_id: l.order_line_id,
        quantity: (qty[l.order_line_id] ?? "").trim(),
      }))
      .filter((x) => x.quantity !== "" && Number(x.quantity) > 0)
  return { qty, set, chosen }
}

// The editable outstanding-lines table shared by both fulfilment panels.
function FulfilLinesTable({
  fulfilLines,
  qty,
  onQty,
  idPrefix,
}: {
  fulfilLines: FulfilLine[]
  qty: Record<number, string>
  onQty: (orderLineId: number, value: string) => void
  idPrefix: string
}) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className="w-10">#</TableHead>
          <TableHead>Description</TableHead>
          <TableHead className="text-right">Outstanding</TableHead>
          <TableHead className="w-36 text-right">Qty</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {fulfilLines.map((l) => (
          <TableRow key={l.order_line_id}>
            <TableCell className="text-muted-foreground">{l.line_no}</TableCell>
            <TableCell className="font-medium">{l.description}</TableCell>
            <TableCell className="text-right font-mono tabular-nums">
              {trimZeros(l.remaining)}
            </TableCell>
            <TableCell className="text-right">
              <Input
                id={`${idPrefix}_qty_${l.order_line_id}`}
                inputMode="decimal"
                className="ml-auto w-28 text-right"
                value={qty[l.order_line_id] ?? ""}
                onChange={(e) => onQty(l.order_line_id, e.target.value)}
              />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

// Inline panel to invoice/bill outstanding lines from the order, in full or in
// part.
function CreateDocPanel({
  config,
  orderId,
  fulfilLines,
  onCancel,
  onCreated,
}: {
  config: OrderConfig
  orderId: number
  fulfilLines: FulfilLine[]
  onCancel: () => void
  onCreated: (docId: number) => void
}) {
  const [number, setNumber] = useState("")
  const [date, setDate] = useState(today())
  const [due, setDue] = useState("")
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const { qty, set, chosen } = useFulfilQuantities(fulfilLines)

  function submit(e: FormEvent) {
    e.preventDefault()
    const lines = chosen()
    if (lines.length === 0) {
      setErr("Enter a quantity on at least one line.")
      return
    }
    setBusy(true)
    setErr(null)
    config
      .createDoc(
        orderId,
        number.trim(),
        date,
        due.trim() === "" ? null : due,
        lines,
      )
      .then(onCreated)
      .catch((e: unknown) => {
        setBusy(false)
        setErr(e instanceof ApiError ? e.message : String(e))
      })
  }

  return (
    <form
      onSubmit={submit}
      className="mb-6 space-y-4 rounded-md border p-4"
      aria-label={`Create ${config.docLabel}`}
    >
      <p className="text-sm text-muted-foreground">
        Creates a draft {config.docLabel.toLowerCase()}. Quantities default to
        what is outstanding; lower them to {config.docLabel.toLowerCase()} part.
      </p>
      {fulfilLines.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Nothing left to {config.docLabel.toLowerCase()} on this order.
        </p>
      ) : (
        <>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
            <div className="space-y-1">
              <Label htmlFor="doc_number">{config.docLabel} #</Label>
              <Input
                id="doc_number"
                required
                value={number}
                onChange={(e) => setNumber(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="doc_date">Date</Label>
              <Input
                id="doc_date"
                type="date"
                required
                value={date}
                onChange={(e) => setDate(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="doc_due">Due Date</Label>
              <Input
                id="doc_due"
                type="date"
                value={due}
                onChange={(e) => setDue(e.target.value)}
              />
            </div>
          </div>
          <FulfilLinesTable
            fulfilLines={fulfilLines}
            qty={qty}
            onQty={set}
            idPrefix="doc"
          />
        </>
      )}
      {err !== null && (
        <p className="text-sm text-destructive" role="alert">
          {err}
        </p>
      )}
      <div className="flex gap-2">
        {fulfilLines.length > 0 && (
          <Button type="submit" disabled={busy}>
            {busy ? "Creating…" : `Create ${config.docLabel.toLowerCase()}`}
          </Button>
        )}
        <Button type="button" variant="outline" onClick={onCancel}>
          {fulfilLines.length > 0 ? "Cancel" : "Close"}
        </Button>
      </div>
    </form>
  )
}

// Inline panel to ship/receive outstanding stocked lines into a warehouse, in
// full or in part. Only stocked lines appear (services carry no stock).
function MovePanel({
  config,
  orderId,
  warehouses,
  fulfilLines,
  onCancel,
  onCreated,
}: {
  config: OrderConfig
  orderId: number
  warehouses: Warehouse[]
  fulfilLines: FulfilLine[]
  onCancel: () => void
  onCreated: (movementIds: number[]) => void
}) {
  const [warehouseId, setWarehouseId] = useState("")
  const [date, setDate] = useState(today())
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const { qty, set, chosen } = useFulfilQuantities(fulfilLines)

  function submit(e: FormEvent) {
    e.preventDefault()
    if (warehouseId === "") {
      setErr("Please choose a warehouse.")
      return
    }
    const lines = chosen()
    if (lines.length === 0) {
      setErr("Enter a quantity on at least one line.")
      return
    }
    setBusy(true)
    setErr(null)
    config
      .createMovements(orderId, Number(warehouseId), date, lines)
      .then(onCreated)
      .catch((e: unknown) => {
        setBusy(false)
        setErr(e instanceof ApiError ? e.message : String(e))
      })
  }

  return (
    <form
      onSubmit={submit}
      className="mb-6 space-y-4 rounded-md border p-4"
      aria-label={config.moveLabel}
    >
      <p className="text-sm text-muted-foreground">
        Creates draft {config.moveLabel === "Ship" ? "issue" : "receipt"}{" "}
        movements for the stocked lines below. Quantities default to what is
        outstanding; lower them to {config.moveLabel.toLowerCase()} part.
      </p>
      {fulfilLines.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Nothing left to {config.moveLabel.toLowerCase()} on this order.
        </p>
      ) : (
        <>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
            <div className="space-y-1">
              <Label htmlFor="move_warehouse">Warehouse</Label>
              <Select value={warehouseId} onValueChange={setWarehouseId}>
                <SelectTrigger id="move_warehouse" className="w-full">
                  <SelectValue placeholder="Select a warehouse" />
                </SelectTrigger>
                <SelectContent>
                  {warehouses.map((w) => (
                    <SelectItem key={w.id} value={String(w.id)}>
                      {w.code} — {w.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label htmlFor="move_date">Date</Label>
              <Input
                id="move_date"
                type="date"
                required
                value={date}
                onChange={(e) => setDate(e.target.value)}
              />
            </div>
          </div>
          <FulfilLinesTable
            fulfilLines={fulfilLines}
            qty={qty}
            onQty={set}
            idPrefix="move"
          />
        </>
      )}
      {err !== null && (
        <p className="text-sm text-destructive" role="alert">
          {err}
        </p>
      )}
      <div className="flex gap-2">
        {fulfilLines.length > 0 && (
          <Button type="submit" disabled={busy}>
            {busy ? "Working…" : config.moveLabel}
          </Button>
        )}
        <Button type="button" variant="outline" onClick={onCancel}>
          {fulfilLines.length > 0 ? "Cancel" : "Close"}
        </Button>
      </div>
    </form>
  )
}
