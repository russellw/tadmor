import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import {
  ACCOUNT_TYPES,
  ApiError,
  createAccount,
  getAccount,
  listAccounts,
  updateAccount,
  type Account,
  type AccountInput,
} from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
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

// Radix Select reserves "" as a value, so the nullable parent select uses this.
const NONE = "__none__"

// account_type is required, so its select starts empty (no sentinel) and is
// validated on submit. parent_id is the self-referencing FK; in edit mode the
// account itself is excluded from the options (a CHECK forbids self-parent).
interface FormState {
  code: string
  name: string
  accountType: string
  parentId: string
  currencyCode: string
  isPostable: boolean
  isActive: boolean
}

const blankForm: FormState = {
  code: "",
  name: "",
  accountType: "",
  parentId: NONE,
  currencyCode: "",
  isPostable: true,
  isActive: true,
}

export function AccountForm({ mode }: { mode: Mode }) {
  const { id } = useParams()
  const accountId = Number(id)
  const navigate = useNavigate()

  const [form, setForm] = useState<FormState | null>(null)
  // Candidate parent accounts (all accounts except, in edit mode, this one).
  const [parents, setParents] = useState<Account[]>([])
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      try {
        if (mode === "edit") {
          if (!Number.isInteger(accountId) || accountId <= 0) {
            setError("Invalid account id.")
            return
          }
          const [account, accounts] = await Promise.all([
            getAccount(accountId),
            listAccounts(),
          ])
          if (cancelled) return
          setParents(accounts.filter((a) => a.id !== account.id))
          setForm({
            code: account.code,
            name: account.name,
            accountType: account.account_type,
            parentId:
              account.parent_id != null ? String(account.parent_id) : NONE,
            currencyCode: account.currency_code ?? "",
            isPostable: account.is_postable,
            isActive: account.is_active,
          })
        } else {
          const accounts = await listAccounts()
          if (cancelled) return
          setParents(accounts)
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
  }, [mode, accountId])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form) return
    if (form.code.trim() === "" || form.name.trim() === "") {
      setSaveError("Code and name are required.")
      return
    }
    if (form.accountType === "") {
      setSaveError("Please choose an account type.")
      return
    }
    const currency = form.currencyCode.trim().toUpperCase()
    const input: AccountInput = {
      code: form.code.trim(),
      name: form.name.trim(),
      account_type: form.accountType,
      parent_id: form.parentId === NONE ? null : Number(form.parentId),
      currency_code: currency === "" ? null : currency,
      is_postable: form.isPostable,
      is_active: form.isActive,
    }
    setSaving(true)
    setSaveError(null)
    const action =
      mode === "create"
        ? createAccount(input).then(() => undefined)
        : updateAccount(accountId, input)
    action
      .then(() => navigate("/accounts"))
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
          {creating ? "New Account" : "Edit Account"}
        </h1>
        <p className="text-sm text-muted-foreground">
          {creating
            ? "Add an account to the chart of accounts."
            : "Update the account's details."}
        </p>
      </header>

      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/accounts">Back to accounts</Link>
          </Button>
        </div>
      )}

      {error === null && form === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {form !== null && (
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="code">Code</Label>
              <Input
                id="code"
                required
                value={form.code}
                onChange={(e) => setForm({ ...form, code: e.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="name">Name</Label>
              <Input
                id="name"
                required
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="account_type">Account Type</Label>
            <Select
              value={form.accountType}
              onValueChange={(v) => setForm({ ...form, accountType: v })}
            >
              <SelectTrigger id="account_type" className="w-full">
                <SelectValue placeholder="Select a type" />
              </SelectTrigger>
              <SelectContent>
                {ACCOUNT_TYPES.map((t) => (
                  <SelectItem key={t.code} value={t.code}>
                    {t.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label htmlFor="parent">Parent Account</Label>
            <Select
              value={form.parentId}
              onValueChange={(v) => setForm({ ...form, parentId: v })}
            >
              <SelectTrigger id="parent" className="w-full">
                <SelectValue placeholder="None" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NONE}>None (top-level)</SelectItem>
                {parents.map((a) => (
                  <SelectItem key={a.id} value={String(a.id)}>
                    {a.code} — {a.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label htmlFor="currency">Currency</Label>
            <Input
              id="currency"
              maxLength={3}
              placeholder="e.g. USD (optional restriction)"
              value={form.currencyCode}
              onChange={(e) =>
                setForm({ ...form, currencyCode: e.target.value })
              }
            />
          </div>

          <div className="flex items-center gap-2">
            <Checkbox
              id="is_postable"
              checked={form.isPostable}
              onCheckedChange={(c) =>
                setForm({ ...form, isPostable: c === true })
              }
            />
            <Label htmlFor="is_postable">
              Postable (uncheck for a summary/header account)
            </Label>
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
              <Link to="/accounts">Cancel</Link>
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}
