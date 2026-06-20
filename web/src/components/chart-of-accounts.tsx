import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import { listAccounts, type Account } from "@/lib/api"
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

// The chart of accounts: a list of GL accounts from GET /api/accounts, with a
// New-account button and per-row Edit link into the account form.
export function ChartOfAccounts() {
  const [accounts, setAccounts] = useState<Account[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    listAccounts()
      .then((data) => {
        if (!cancelled) setAccounts(data)
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
      <header className="mb-6 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Chart of Accounts
          </h1>
          <p className="text-sm text-muted-foreground">
            The general-ledger accounts, ordered by code.
          </p>
        </div>
        <Button asChild>
          <Link to="/accounts/new">New account</Link>
        </Button>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load accounts: {error}
        </p>
      )}

      {error === null && accounts === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {accounts !== null && accounts.length === 0 && (
        <p className="text-sm text-muted-foreground">No accounts yet.</p>
      )}

      {accounts !== null && accounts.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-28">Code</TableHead>
              <TableHead>Name</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>Currency</TableHead>
              <TableHead>Postable</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="w-0"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {accounts.map((a) => (
              <TableRow key={a.id}>
                <TableCell className="font-mono">{a.code}</TableCell>
                <TableCell className="font-medium">{a.name}</TableCell>
                <TableCell>
                  <Badge variant="secondary">{a.account_type}</Badge>
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {a.currency_code ?? "—"}
                </TableCell>
                <TableCell>{a.is_postable ? "Yes" : "No"}</TableCell>
                <TableCell>
                  <Badge variant={a.is_active ? "default" : "outline"}>
                    {a.is_active ? "Active" : "Inactive"}
                  </Badge>
                </TableCell>
                <TableCell className="text-right">
                  <Link
                    to={`/accounts/${a.id}`}
                    className="text-sm font-medium text-primary hover:underline"
                  >
                    Edit
                  </Link>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </section>
  )
}
