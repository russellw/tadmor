import { useState } from "react"

import { ChartOfAccounts } from "@/components/chart-of-accounts"
import { Customers } from "@/components/customers"
import { cn } from "@/lib/utils"

// A minimal state-based view switcher. Deliberately not a URL router yet: a real
// client-side router is a new runtime dependency and a deliberate choice under
// the supply-chain policy (see docs/frontend-stack.md), to be made when deep
// links / the back button actually matter.
const screens = {
  accounts: { label: "Chart of Accounts", Component: ChartOfAccounts },
  customers: { label: "Customers", Component: Customers },
} as const

type ScreenKey = keyof typeof screens
const screenKeys = Object.keys(screens) as ScreenKey[]

export default function App() {
  const [active, setActive] = useState<ScreenKey>("accounts")
  const { Component } = screens[active]

  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b">
        <nav className="mx-auto flex w-full max-w-5xl gap-1 p-3">
          {screenKeys.map((key) => (
            <button
              key={key}
              type="button"
              onClick={() => setActive(key)}
              aria-current={active === key ? "page" : undefined}
              className={cn(
                "rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
                active === key
                  ? "bg-secondary text-secondary-foreground"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground",
              )}
            >
              {screens[key].label}
            </button>
          ))}
        </nav>
      </header>
      <main>
        <Component />
      </main>
    </div>
  )
}
