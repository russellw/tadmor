import { useCallback, useEffect, useState } from "react"
import { Link, useParams } from "react-router-dom"

import { formatAmount } from "@/lib/amount"
import { useCurrentUser } from "@/lib/current-user"
import {
  ApiError,
  getStockMovement,
  listAccounts,
  listProducts,
  listWarehouses,
  postStockMovement,
  unpostStockMovement,
  type Account,
  type StockMovement,
} from "@/lib/api"
import {
  MovementGLBadge,
  POSTABLE_MOVEMENT_TYPES,
} from "@/components/stock-movements"
import { Badge } from "@/components/ui/badge"
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

// Radix Select reserves "" as a value, so the nullable select uses this.
const NONE = "__none__"

// One stock movement: its facts plus the GL actions. Posting needs a currency
// (movements carry none of their own); a receipt additionally needs the
// clearing account it credits (typically Goods Received Not Invoiced), which
// the matching purchase bill later debits. An issue posts COGS against
// inventory. Unpost reverses the journal entry.
export function StockMovementDetail() {
  const { id } = useParams()
  const movementId = Number(id)

  const [movement, setMovement] = useState<StockMovement | null>(null)
  const [productName, setProductName] = useState<string>("")
  const [warehouseName, setWarehouseName] = useState<string>("")
  const [accounts, setAccounts] = useState<Account[]>([])
  const [error, setError] = useState<string | null>(null)

  const [currency, setCurrency] = useState("")
  const [creditAccountId, setCreditAccountId] = useState(NONE)
  const [acting, setActing] = useState(false)
  const currentUser = useCurrentUser()
  const [actionError, setActionError] = useState<string | null>(null)

  const load = useCallback(async () => {
    const [m, products, warehouses, accts] = await Promise.all([
      getStockMovement(movementId),
      listProducts(),
      listWarehouses(),
      listAccounts(),
    ])
    const product = products.find((p) => p.id === m.product_id)
    const warehouse = warehouses.find((w) => w.id === m.warehouse_id)
    return {
      m,
      productName: product ? `${product.sku} — ${product.name}` : `#${m.product_id}`,
      productCurrency: product?.currency_code ?? null,
      warehouseName: warehouse
        ? `${warehouse.code} — ${warehouse.name}`
        : `#${m.warehouse_id}`,
      accts,
    }
  }, [movementId])

  useEffect(() => {
    if (!Number.isInteger(movementId) || movementId <= 0) {
      setError("Invalid id.")
      return
    }
    let cancelled = false
    load()
      .then((r) => {
        if (cancelled) return
        setMovement(r.m)
        setProductName(r.productName)
        setWarehouseName(r.warehouseName)
        setAccounts(r.accts)
        setCurrency((c) => (c === "" ? (r.productCurrency ?? "") : c))
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [movementId, load])

  function refresh() {
    load().then((r) => {
      setMovement(r.m)
      setActing(false)
    })
  }

  function handlePost() {
    if (!movement) return
    setActing(true)
    setActionError(null)
    postStockMovement(
      movementId,
      currency.trim().toUpperCase(),
      creditAccountId === NONE ? null : Number(creditAccountId),
    )
      .then(refresh)
      .catch((err: unknown) => {
        setActing(false)
        setActionError(err instanceof ApiError ? err.message : String(err))
      })
  }

  function handleUnpost() {
    setActing(true)
    setActionError(null)
    unpostStockMovement(movementId)
      .then(refresh)
      .catch((err: unknown) => {
        setActing(false)
        setActionError(err instanceof ApiError ? err.message : String(err))
      })
  }

  const posted = movement?.journal_entry_id != null
  const postable =
    movement !== null &&
    !posted &&
    POSTABLE_MOVEMENT_TYPES.has(movement.movement_type)

  return (
    <section className="mx-auto w-full max-w-2xl p-6">
      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/stock-movements">Back</Link>
          </Button>
        </div>
      )}

      {error === null && movement === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {movement !== null && (
        <>
          <header className="mb-6 flex items-start justify-between gap-4">
            <div>
              <div className="flex items-center gap-3">
                <h1 className="text-2xl font-semibold tracking-tight">
                  Stock Movement #{movement.id}
                </h1>
                <Badge variant="secondary">{movement.movement_type}</Badge>
                <MovementGLBadge movement={movement} />
              </div>
              <p className="text-sm text-muted-foreground">
                {productName} · {warehouseName} · {movement.date}
                {movement.reference !== null && ` · ${movement.reference}`}
                {movement.journal_entry_id !== null && (
                  <>
                    {" · "}
                    <Link
                      to={`/journal-entries/${movement.journal_entry_id}`}
                      className="text-primary hover:underline"
                    >
                      journal entry #{movement.journal_entry_id}
                    </Link>
                  </>
                )}
              </p>
            </div>
            <Button variant="outline" asChild>
              <Link to="/stock-movements">Back</Link>
            </Button>
          </header>

          <dl className="mb-6 grid grid-cols-3 gap-4 text-sm">
            {(
              [
                ["Quantity", movement.quantity],
                ["Unit Cost", movement.unit_cost],
                ["Total Cost", movement.total_cost],
              ] as const
            ).map(([label, value]) => (
              <div key={label}>
                <dt className="text-muted-foreground">{label}</dt>
                <dd className="font-mono text-base tabular-nums">
                  {formatAmount(value)}
                </dd>
              </div>
            ))}
          </dl>

          {movement.notes !== null && (
            <p className="mb-6 text-sm text-muted-foreground">
              {movement.notes}
            </p>
          )}

          {actionError !== null && (
            <p className="mb-4 text-sm text-destructive" role="alert">
              {actionError}
            </p>
          )}

          {postable && (
            <div className="space-y-4 rounded-md border p-4">
              <div>
                <h2 className="text-sm font-semibold">Post to ledger</h2>
                <p className="text-sm text-muted-foreground">
                  {movement.movement_type === "receipt"
                    ? "Debits the product's inventory account and credits the clearing account below (typically Goods Received Not Invoiced)."
                    : "Debits the product's COGS account and credits its inventory account."}
                </p>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="currency">Currency</Label>
                  <Input
                    id="currency"
                    maxLength={3}
                    placeholder="e.g. USD"
                    value={currency}
                    onChange={(e) => setCurrency(e.target.value)}
                  />
                </div>
                {movement.movement_type === "receipt" && (
                  <div className="space-y-2">
                    <Label htmlFor="credit_account">Credit Account</Label>
                    <Select
                      value={creditAccountId}
                      onValueChange={setCreditAccountId}
                    >
                      <SelectTrigger id="credit_account" className="w-full">
                        <SelectValue placeholder="Select an account" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={NONE}>None</SelectItem>
                        {accounts.map((a) => (
                          <SelectItem key={a.id} value={String(a.id)}>
                            {a.code} — {a.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                )}
              </div>
              <Button disabled={acting} onClick={handlePost}>
                {acting ? "Posting…" : "Post to ledger"}
              </Button>
            </div>
          )}

          {posted && currentUser.is_admin && (
            <Button variant="outline" disabled={acting} onClick={handleUnpost}>
              {acting ? "Unposting…" : "Unpost"}
            </Button>
          )}

          {!postable && !posted && (
            <p className="text-sm text-muted-foreground">
              This movement type does not post to the ledger.
            </p>
          )}
        </>
      )}
    </section>
  )
}
