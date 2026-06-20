import { NavLink, Navigate, Route, Routes } from "react-router-dom"

import { ChartOfAccounts } from "@/components/chart-of-accounts"
import { Customers } from "@/components/customers"
import { cn } from "@/lib/utils"

// URL routing via react-router-dom (v7). The Go backend's spaHandler falls back
// to index.html for non-/api/ paths, so deep links like /customers resolve in
// production; Vite's dev server does the same in development.
const navItems = [
  { to: "/accounts", label: "Chart of Accounts" },
  { to: "/customers", label: "Customers" },
]

export default function App() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b">
        <nav className="mx-auto flex w-full max-w-5xl gap-1 p-3">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                cn(
                  "rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
                  isActive
                    ? "bg-secondary text-secondary-foreground"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground",
                )
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
      </header>
      <main>
        <Routes>
          <Route path="/" element={<Navigate to="/accounts" replace />} />
          <Route path="/accounts" element={<ChartOfAccounts />} />
          <Route path="/customers" element={<Customers />} />
          <Route
            path="*"
            element={
              <p className="mx-auto w-full max-w-5xl p-6 text-sm text-muted-foreground">
                Page not found.
              </p>
            }
          />
        </Routes>
      </main>
    </div>
  )
}
