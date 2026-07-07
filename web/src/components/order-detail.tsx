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
  getPurchaseOrder,
  getPurchaseOrderLines,
  getSalesOrder,
  getSalesOrderLines,
  invoiceSalesOrder,
  listWarehouses,
  receivePurchaseOrder,
  shipSalesOrder,
  type Warehouse,
} from "@/lib/api"
import { AmountCell } from "@/components/amount-cell"
import {
  fetchCustomerNames,
  fetchSupplierNames,
} from "@/components/document-list"
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
  createDoc: (
    id: number,
    number: string,
    date: string,
    due: string | null,
  ) => Promise<number> // returns the new document id
  createMovements: (
    id: number,
    warehouseId: number,
    date: string | null,
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
        createDoc: async (id, number, date, due) =>
          (
            await invoiceSalesOrder(id, {
              invoice_number: number,
              invoice_date: date,
              due_date: due,
            })
          ).invoice_id,
        createMovements: async (id, warehouseId, date) =>
          (
            await shipSalesOrder(id, {
              warehouse_id: warehouseId,
              movement_date: date,
              reference: null,
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
        createDoc: async (id, number, date, due) =>
          (
            await billPurchaseOrder(id, {
              bill_number: number,
              bill_date: date,
              due_date: due,
            })
          ).bill_id,
        createMovements: async (id, warehouseId, date) =>
          (
            await receivePurchaseOrder(id, {
              warehouse_id: warehouseId,
              movement_date: date,
              reference: null,
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
  // Which inline fulfilment panel is open, and a note after a stock movement.
  const [panel, setPanel] = useState<"none" | "doc" | "move">("none")
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
          onCancel={() => setPanel("none")}
          onCreated={(docId) => navigate(`${config.docBasePath}/${docId}`)}
        />
      )}

      {panel === "move" && (
        <MovePanel
          config={config}
          orderId={orderId}
          warehouses={warehouses}
          onCancel={() => setPanel("none")}
          onCreated={(ids) => {
            setPanel("none")
            setMovementNote(ids)
            reload()
          }}
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

// Inline panel to create the whole remaining invoice/bill from the order.
function CreateDocPanel({
  config,
  orderId,
  onCancel,
  onCreated,
}: {
  config: OrderConfig
  orderId: number
  onCancel: () => void
  onCreated: (docId: number) => void
}) {
  const [number, setNumber] = useState("")
  const [date, setDate] = useState(today())
  const [due, setDue] = useState("")
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setErr(null)
    config
      .createDoc(orderId, number.trim(), date, due.trim() === "" ? null : due)
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
        Creates a draft {config.docLabel.toLowerCase()} for everything still
        outstanding on this order.
      </p>
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
      {err !== null && (
        <p className="text-sm text-destructive" role="alert">
          {err}
        </p>
      )}
      <div className="flex gap-2">
        <Button type="submit" disabled={busy}>
          {busy ? "Creating…" : `Create ${config.docLabel.toLowerCase()}`}
        </Button>
        <Button type="button" variant="outline" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  )
}

// Inline panel to ship/receive the remaining stocked lines into a warehouse.
function MovePanel({
  config,
  orderId,
  warehouses,
  onCancel,
  onCreated,
}: {
  config: OrderConfig
  orderId: number
  warehouses: Warehouse[]
  onCancel: () => void
  onCreated: (movementIds: number[]) => void
}) {
  const [warehouseId, setWarehouseId] = useState("")
  const [date, setDate] = useState(today())
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  function submit(e: FormEvent) {
    e.preventDefault()
    if (warehouseId === "") {
      setErr("Please choose a warehouse.")
      return
    }
    setBusy(true)
    setErr(null)
    config
      .createMovements(orderId, Number(warehouseId), date)
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
        movements for the stocked lines still outstanding. Non-stock lines
        (services) are skipped.
      </p>
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
      {err !== null && (
        <p className="text-sm text-destructive" role="alert">
          {err}
        </p>
      )}
      <div className="flex gap-2">
        <Button type="submit" disabled={busy}>
          {busy ? "Working…" : config.moveLabel}
        </Button>
        <Button type="button" variant="outline" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  )
}
