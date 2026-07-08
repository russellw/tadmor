import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import {
  ApiError,
  createUser,
  getUser,
  setUserPassword,
  updateUser,
} from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { PasswordInput } from "@/components/password-input"

type Mode = "create" | "edit"

interface FormState {
  email: string
  fullName: string
  password: string // create mode only
  isActive: boolean
  isAdmin: boolean
}

const blankForm: FormState = {
  email: "",
  fullName: "",
  password: "",
  isActive: true,
  isAdmin: false,
}

const MIN_PASSWORD = 8

export function UserForm({ mode }: { mode: Mode }) {
  const { id } = useParams()
  const userId = Number(id)
  const navigate = useNavigate()

  const [form, setForm] = useState<FormState | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  // Edit mode's separate reset-password section.
  const [newPassword, setNewPassword] = useState("")
  const [resetting, setResetting] = useState(false)
  const [resetResult, setResetResult] = useState<string | null>(null)
  const [resetError, setResetError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      try {
        if (mode === "edit") {
          if (!Number.isInteger(userId) || userId <= 0) {
            setError("Invalid user id.")
            return
          }
          const u = await getUser(userId)
          if (cancelled) return
          setForm({
            email: u.email,
            fullName: u.full_name,
            password: "",
            isActive: u.is_active,
            isAdmin: u.is_admin,
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
  }, [mode, userId])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form) return
    const email = form.email.trim()
    const fullName = form.fullName.trim()
    if (email === "" || fullName === "") {
      setSaveError("Email and name are required.")
      return
    }
    if (mode === "create" && form.password.length < MIN_PASSWORD) {
      setSaveError(`Password must be at least ${MIN_PASSWORD} characters.`)
      return
    }
    setSaving(true)
    setSaveError(null)
    const action =
      mode === "create"
        ? createUser({
            email,
            full_name: fullName,
            password: form.password,
            is_admin: form.isAdmin,
          }).then(() => undefined)
        : updateUser(userId, {
            email,
            full_name: fullName,
            is_active: form.isActive,
            is_admin: form.isAdmin,
          })
    action
      .then(() => navigate("/users"))
      .catch((err: unknown) => {
        setSaving(false)
        setSaveError(err instanceof ApiError ? err.message : String(err))
      })
  }

  function handleResetPassword() {
    if (newPassword.length < MIN_PASSWORD) {
      setResetError(`Password must be at least ${MIN_PASSWORD} characters.`)
      setResetResult(null)
      return
    }
    setResetting(true)
    setResetError(null)
    setResetResult(null)
    setUserPassword(userId, newPassword)
      .then(() => {
        setResetting(false)
        setNewPassword("")
        setResetResult("Password changed; the user's sessions were signed out.")
      })
      .catch((err: unknown) => {
        setResetting(false)
        setResetError(err instanceof ApiError ? err.message : String(err))
      })
  }

  const creating = mode === "create"

  return (
    <section className="mx-auto w-full max-w-2xl p-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">
          {creating ? "New User" : "Edit User"}
        </h1>
        <p className="text-sm text-muted-foreground">
          {creating
            ? "Add a login for the app."
            : "Update the user's details, access, or password."}
        </p>
      </header>

      {error !== null && (
        <div className="space-y-4">
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
          <Button variant="outline" asChild>
            <Link to="/users">Back to users</Link>
          </Button>
        </div>
      )}

      {error === null && form === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {form !== null && (
        <>
          <form onSubmit={handleSubmit} className="space-y-5">
            <div className="space-y-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                placeholder="e.g. pat@example.com"
                value={form.email}
                onChange={(e) => setForm({ ...form, email: e.target.value })}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="full_name">Full Name</Label>
              <Input
                id="full_name"
                placeholder="e.g. Pat Smith"
                value={form.fullName}
                onChange={(e) => setForm({ ...form, fullName: e.target.value })}
              />
            </div>

            {creating && (
              <div className="space-y-2">
                <Label htmlFor="password">Password</Label>
                <PasswordInput
                  id="password"
                  autoComplete="new-password"
                  value={form.password}
                  onChange={(e) =>
                    setForm({ ...form, password: e.target.value })
                  }
                />
                <p className="text-xs text-muted-foreground">
                  At least {MIN_PASSWORD} characters.
                </p>
              </div>
            )}

            <div className="flex items-center gap-2">
              <Checkbox
                id="is_admin"
                checked={form.isAdmin}
                onCheckedChange={(c) =>
                  setForm({ ...form, isAdmin: c === true })
                }
              />
              <Label htmlFor="is_admin">Administrator</Label>
              <span className="text-xs text-muted-foreground">
                Can manage users and unpost documents.
              </span>
            </div>

            {!creating && (
              <div className="flex items-center gap-2">
                <Checkbox
                  id="is_active"
                  checked={form.isActive}
                  onCheckedChange={(c) =>
                    setForm({ ...form, isActive: c === true })
                  }
                />
                <Label htmlFor="is_active">Active</Label>
              </div>
            )}

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
                <Link to="/users">Cancel</Link>
              </Button>
            </div>
          </form>

          {!creating && (
            <div className="mt-10 space-y-4 rounded-md border p-4">
              <div>
                <h2 className="font-semibold">Reset password</h2>
                <p className="text-sm text-muted-foreground">
                  Sets a new password and signs the user out everywhere.
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="new_password">New Password</Label>
                <PasswordInput
                  id="new_password"
                  autoComplete="new-password"
                  className="max-w-80"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                />
              </div>
              {resetError !== null && (
                <p className="text-sm text-destructive" role="alert">
                  Failed to reset: {resetError}
                </p>
              )}
              {resetResult !== null && (
                <p className="text-sm text-muted-foreground" role="status">
                  {resetResult}
                </p>
              )}
              <Button
                type="button"
                variant="outline"
                disabled={resetting}
                onClick={handleResetPassword}
              >
                {resetting ? "Resetting…" : "Reset password"}
              </Button>
            </div>
          )}
        </>
      )}
    </section>
  )
}
