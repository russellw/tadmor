import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import {
  ApiError,
  createWarehouse,
  getWarehouse,
  updateWarehouse,
  type WarehouseInput,
} from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

type Mode = "create" | "edit"

interface FormState {
  code: string
  name: string
  // There is no addresses screen or endpoint yet, so the form never edits
  // address_id — it only carries the loaded value back on save (PUT is a full
  // replace and must not drop it).
  addressId: number | null
  isActive: boolean
}

const blankForm: FormState = {
  code: "",
  name: "",
  addressId: null,
  isActive: true,
}

export function WarehouseForm({ mode }: { mode: Mode }) {
  const { id } = useParams()
  const warehouseId = Number(id)
  const navigate = useNavigate()

  const [form, setForm] = useState<FormState | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      try {
        if (mode === "edit") {
          if (!Number.isInteger(warehouseId) || warehouseId <= 0) {
            setError("Invalid warehouse id.")
            return
          }
          const w = await getWarehouse(warehouseId)
          if (cancelled) return
          setForm({
            code: w.code,
            name: w.name,
            addressId: w.address_id,
            isActive: w.is_active,
          })
        } else {
          setForm(blankForm)
        }
      } catch (err: unknown) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [mode, warehouseId])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form) return
    if (form.code.trim() === "" || form.name.trim() === "") {
      setSaveError("Code and name are required.")
      return
    }
    const input: WarehouseInput = {
      code: form.code.trim().toUpperCase(),
      name: form.name.trim(),
      address_id: form.addressId,
      is_active: form.isActive,
    }
    setSaving(true)
    setSaveError(null)
    const action =
      mode === "create"
        ? createWarehouse(input).then(() => undefined)
        : updateWarehouse(warehouseId, input)
    action
      .then(() => navigate("/warehouses"))
      .catch((err: unknown) => {
        setSaving(false)
        setSaveError(err instanceof ApiError ? err.message : String(err))
      })
  }

  const creating = mode === "create"

  return (
    <section className="mx-auto w-full max-w-2xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">
          {creating ? "New Warehouse" : "Edit Warehouse"}
        </h1>
        <p className="text-sm text-muted-foreground">
          {creating
            ? "Add a stock location for movements to happen against."
            : "Update the warehouse's details."}
        </p>
      </header>

      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/warehouses">Back to warehouses</Link>
          </Button>
        </div>
      )}

      {error === null && form === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {form !== null && (
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="code">Code</Label>
            <Input
              id="code"
              placeholder="e.g. MAIN"
              className="max-w-40"
              value={form.code}
              onChange={(e) => setForm({ ...form, code: e.target.value })}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              placeholder="e.g. Main warehouse"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </div>

          <div className="flex items-center gap-2">
            <Checkbox
              id="is_active"
              checked={form.isActive}
              onCheckedChange={(c) => setForm({ ...form, isActive: c === true })}
            />
            <Label htmlFor="is_active">Active</Label>
          </div>

          {saveError !== null && (
            <p className="text-sm text-destructive" role="alert">
              Failed to save: {saveError}
            </p>
          )}

          <div className="flex gap-2">
            <Button type="submit" disabled={saving}>
              {saving ? "Saving…" : creating ? "Create" : "Save"}
            </Button>
            <Button type="button" variant="outline" asChild>
              <Link to="/warehouses">Cancel</Link>
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}
