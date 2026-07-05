import { createContext, useContext } from "react"

import type { User } from "@/lib/api"

// The signed-in user, provided by App once the session probe resolves. The
// backend enforces authorization on every request; this context only tailors
// the UI (hide the Users nav and the Unpost buttons from non-administrators).
export const CurrentUserContext = createContext<User | null>(null)

export function useCurrentUser(): User {
  const user = useContext(CurrentUserContext)
  if (user === null) {
    // App only renders the shell (and thus any consumer) after login.
    throw new Error("useCurrentUser called outside the signed-in app shell")
  }
  return user
}
