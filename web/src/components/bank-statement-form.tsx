import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import { trimAmount } from "@/lib/amount"
import {
  ApiError,
  createBankStatement,
  getBankStatement,
  listAccounts,
  updateBankStatement,
  type Account,
} from "@/lib/api"
import { today } from "@/lib/dates"
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

type Mode = "create" | "edit"

interface FormState {
  accountId: string
  statementDate: string
  openingBalance: string
  closingBalance: string
  reference: string
}

export function BankStatementForm({ mode }: { mode: Mode }) {
  const { id } = useParams()
  const statementId = Number(id)
  const navigate = useNavigate()

  // Statements are drawn on cash accounts only (the backend enforces it);
  // offer only those.
  const [accounts, setAccounts] = useState<Account[]>([])
  const [form, setForm] = useState<FormState | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      try {
        const cashAccounts = (await listAccounts()).filter(
          (a) => a.is_cash && a.is_postable && a.is_active,
        )
        if (cancelled) return
        setAccounts(cashAccounts)
        if (mode === "edit") {
          if (!Number.isInteger(statementId) || statementId <= 0) {
            setError("Invalid statement id.")
            return
          }
          const s = await getBankStatement(statementId)
          if (cancelled) return
          setForm({
            accountId: String(s.account_id),
            statementDate: s.statement_date,
            openingBalance: trimAmount(s.opening_balance),
            closingBalance: trimAmount(s.closing_balance),
            reference: s.reference ?? "",
          })
        } else {
          setForm({
            accountId:
              cashAccounts.length === 1 ? String(cashAccounts[0].id) : "",
            statementDate: today(),
            openingBalance: "0",
            closingBalance: "0",
            reference: "",
          })
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
  }, [mode, statementId])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form) return
    if (form.accountId === "") {
      setSaveError("Please choose a bank account.")
      return
    }
    const input = {
      account_id: Number(form.accountId),
      statement_date: form.statementDate,
      opening_balance: form.openingBalance,
      closing_balance: form.closingBalance,
      reference: form.reference.trim() === "" ? null : form.reference.trim(),
    }
    setSaving(true)
    setSaveError(null)
    const action =
      mode === "create"
        ? createBankStatement(input).then(({ id }) => id)
        : updateBankStatement(statementId, input).then(() => statementId)
    action
      .then((id) => navigate(`/bank-statements/${id}`))
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
          {creating ? "New Bank Statement" : "Edit Bank Statement"}
        </h1>
        <p className="text-sm text-muted-foreground">
          {creating
            ? "Capture a statement's header, then add its transactions and match them against the ledger."
            : "Update the statement's header."}
        </p>
      </header>

      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/bank-statements">Back to statements</Link>
          </Button>
        </div>
      )}

      {error === null && form === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {form !== null && (
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="account">Bank Account</Label>
            {accounts.length > 0 ? (
              <Select
                value={form.accountId}
                onValueChange={(accountId) => setForm({ ...form, accountId })}
              >
                <SelectTrigger id="account" className="w-full">
                  <SelectValue placeholder="Select a cash account" />
                </SelectTrigger>
                <SelectContent>
                  {accounts.map((a) => (
                    <SelectItem key={a.id} value={String(a.id)}>
                      {a.code} {a.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <p className="text-sm text-muted-foreground">
                No cash accounts. Mark a bank account as cash in the chart of
                accounts first.
              </p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="statement_date">Statement Date</Label>
            <Input
              id="statement_date"
              type="date"
              required
              value={form.statementDate}
              onChange={(e) =>
                setForm({ ...form, statementDate: e.target.value })
              }
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="opening_balance">Opening Balance</Label>
              <Input
                id="opening_balance"
                inputMode="decimal"
                required
                value={form.openingBalance}
                onChange={(e) =>
                  setForm({ ...form, openingBalance: e.target.value })
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="closing_balance">Closing Balance</Label>
              <Input
                id="closing_balance"
                inputMode="decimal"
                required
                value={form.closingBalance}
                onChange={(e) =>
                  setForm({ ...form, closingBalance: e.target.value })
                }
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="reference">Reference</Label>
            <Input
              id="reference"
              placeholder="e.g. the bank's statement number"
              value={form.reference}
              onChange={(e) => setForm({ ...form, reference: e.target.value })}
            />
          </div>

          {saveError !== null && (
            <p className="text-sm text-destructive" role="alert">
              Failed to save: {saveError}
            </p>
          )}

          <div className="flex gap-2">
            <Button type="submit" disabled={saving || accounts.length === 0}>
              {saving ? "Saving…" : creating ? "Create" : "Save"}
            </Button>
            <Button type="button" variant="outline" asChild>
              <Link
                to={
                  creating
                    ? "/bank-statements"
                    : `/bank-statements/${statementId}`
                }
              >
                Cancel
              </Link>
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}
