import { useState, type FormEvent } from "react"

import { ApiError, login, type User } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

/** The full-screen sign-in view shown whenever there is no live session.
 *  Accounts are created out of band (server -adduser); there is no sign-up. */
export function LoginForm({ onLogin }: { onLogin: (user: User) => void }) {
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (email.trim() === "" || password === "") {
      setError("Email and password are required.")
      return
    }
    setBusy(true)
    setError(null)
    login(email.trim(), password)
      .then(onLogin)
      .catch((err: unknown) => {
        setBusy(false)
        setError(err instanceof ApiError ? err.message : String(err))
      })
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-6 text-foreground">
      <section className="w-full max-w-sm">
        <header className="mb-6 text-center">
          <h1 className="text-2xl font-semibold tracking-tight">Tadmor</h1>
          <p className="text-sm text-muted-foreground">
            Sign in to your account.
          </p>
        </header>

        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="email">Email</Label>
            <Input
              id="email"
              type="email"
              autoComplete="email"
              autoFocus
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="password">Password</Label>
            <Input
              id="password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>

          {error !== null && (
            <p className="text-sm text-destructive" role="alert">
              {error}
            </p>
          )}

          <Button type="submit" className="w-full" disabled={busy}>
            {busy ? "Signing in…" : "Sign in"}
          </Button>
        </form>

        <p className="mt-6 text-center text-sm text-muted-foreground">
          Guest access: <span className="font-mono">guest@demo</span> /{" "}
          <span className="font-mono">guest123</span>
        </p>
      </section>
    </div>
  )
}
