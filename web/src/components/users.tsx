import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import { listUsers, type UserRecord } from "@/lib/api"
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

// Login users from GET /api/users, ordered by email. Admin-only: the nav
// entry is hidden from non-administrators and the API returns 403 to them.
// Deactivation replaces deletion.
export function Users() {
  const [users, setUsers] = useState<UserRecord[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    listUsers()
      .then((data) => {
        if (!cancelled) setUsers(data)
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
          <h1 className="text-2xl font-semibold tracking-tight">Users</h1>
          <p className="text-sm text-muted-foreground">
            Who can sign in, ordered by email. Deactivate a user to revoke
            access; there is no delete.
          </p>
        </div>
        <Button asChild>
          <Link to="/users/new">New user</Link>
        </Button>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load users: {error}
        </p>
      )}

      {error === null && users === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {users !== null && users.length === 0 && (
        <p className="text-sm text-muted-foreground">No users yet.</p>
      )}

      {users !== null && users.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Email</TableHead>
              <TableHead>Name</TableHead>
              <TableHead>Role</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="w-0"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {users.map((u) => (
              <TableRow key={u.id}>
                <TableCell className="font-mono">{u.email}</TableCell>
                <TableCell className="font-medium">{u.full_name}</TableCell>
                <TableCell>
                  {u.is_admin ? (
                    <Badge variant="secondary">Admin</Badge>
                  ) : (
                    <span className="text-sm text-muted-foreground">User</span>
                  )}
                </TableCell>
                <TableCell>
                  <Badge variant={u.is_active ? "default" : "outline"}>
                    {u.is_active ? "Active" : "Inactive"}
                  </Badge>
                </TableCell>
                <TableCell className="text-right">
                  <Link
                    to={`/users/${u.id}`}
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
