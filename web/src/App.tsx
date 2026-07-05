import { useEffect, useState } from "react"
import { NavLink, Navigate, Route, Routes } from "react-router-dom"

import { logout, me, UNAUTHORIZED_EVENT, type User } from "@/lib/api"
import { LoginForm } from "@/components/login-form"
import { Button } from "@/components/ui/button"
import { AccountForm } from "@/components/account-form"
import { APAging, ARAging } from "@/components/aging-report"
import { BillForm } from "@/components/bill-form"
import { ChartOfAccounts } from "@/components/chart-of-accounts"
import { CustomerForm } from "@/components/customer-form"
import { Customers } from "@/components/customers"
import { FiscalYearForm } from "@/components/fiscal-year-form"
import { BillDetail, InvoiceDetail } from "@/components/document-detail"
import { Bills, Invoices } from "@/components/document-list"
import { InventoryValuation } from "@/components/inventory-valuation"
import { InvoiceForm } from "@/components/invoice-form"
import {
  CustomerPaymentDetail,
  SupplierPaymentDetail,
} from "@/components/payment-detail"
import {
  CustomerPaymentForm,
  SupplierPaymentForm,
} from "@/components/payment-form"
import {
  CustomerPayments,
  SupplierPayments,
} from "@/components/payment-list"
import { PeriodForm } from "@/components/period-form"
import { Periods } from "@/components/periods"
import { StockMovementDetail } from "@/components/stock-movement-detail"
import { StockMovementForm } from "@/components/stock-movement-form"
import { StockMovements } from "@/components/stock-movements"
import { OrganizationForm } from "@/components/organization-form"
import { Organizations } from "@/components/organizations"
import { ProductForm } from "@/components/product-form"
import { Products } from "@/components/products"
import { SupplierForm } from "@/components/supplier-form"
import { Suppliers } from "@/components/suppliers"
import { TaxCodeForm } from "@/components/tax-code-form"
import { TaxCodes } from "@/components/tax-codes"
import { TrialBalance } from "@/components/trial-balance"
import { WarehouseForm } from "@/components/warehouse-form"
import { Warehouses } from "@/components/warehouses"
import { cn } from "@/lib/utils"

// URL routing via react-router-dom (v7). The Go backend's spaHandler falls back
// to index.html for non-/api/ paths, so deep links like /customers resolve in
// production; Vite's dev server does the same in development.
const masterNavItems = [
  { to: "/accounts", label: "Chart of Accounts" },
  { to: "/organizations", label: "Organizations" },
  { to: "/customers", label: "Customers" },
  { to: "/suppliers", label: "Suppliers" },
  { to: "/products", label: "Products" },
  { to: "/tax-codes", label: "Tax Codes" },
  { to: "/warehouses", label: "Warehouses" },
  { to: "/periods", label: "Periods" },
]

const documentNavItems = [
  { to: "/invoices", label: "Invoices" },
  { to: "/bills", label: "Bills" },
  { to: "/customer-payments", label: "Customer Payments" },
  { to: "/supplier-payments", label: "Supplier Payments" },
  { to: "/stock-movements", label: "Stock" },
]

const reportNavItems = [
  { to: "/reports/trial-balance", label: "Trial Balance" },
  { to: "/reports/ar-aging", label: "AR Aging" },
  { to: "/reports/ap-aging", label: "AP Aging" },
  { to: "/reports/inventory", label: "Inventory" },
]

function NavItems({ items }: { items: { to: string; label: string }[] }) {
  return items.map((item) => (
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
  ))
}

export default function App() {
  // undefined = still probing the session, null = logged out.
  const [user, setUser] = useState<User | null | undefined>(undefined)

  useEffect(() => {
    me()
      .then(setUser)
      .catch(() => setUser(null))
  }, [])

  // Any 401 (e.g. the session expiring mid-use) drops back to the login screen.
  useEffect(() => {
    const onUnauthorized = () => setUser(null)
    window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized)
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized)
  }, [])

  function handleSignOut() {
    // Even if the request fails the session cookie is gone client-side intent-
    // wise; always fall back to the login screen.
    void logout()
      .catch(() => undefined)
      .finally(() => setUser(null))
  }

  if (user === undefined) {
    // Brief blank while the session probe runs; avoids flashing the login
    // screen at already-signed-in users.
    return null
  }
  if (user === null) {
    return <LoginForm onLogin={setUser} />
  }

  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b">
        <nav className="mx-auto flex w-full max-w-5xl flex-wrap gap-1 p-3">
          <NavItems items={masterNavItems} />
          <span aria-hidden className="mx-1 my-auto h-4 w-px bg-border" />
          <NavItems items={documentNavItems} />
          <span aria-hidden className="mx-1 my-auto h-4 w-px bg-border" />
          <NavItems items={reportNavItems} />
          <span className="ml-auto flex items-center gap-2">
            <span className="text-sm text-muted-foreground">
              {user.full_name}
            </span>
            <Button variant="outline" size="sm" onClick={handleSignOut}>
              Sign out
            </Button>
          </span>
        </nav>
      </header>
      <main>
        <Routes>
          <Route path="/" element={<Navigate to="/accounts" replace />} />
          <Route path="/accounts" element={<ChartOfAccounts />} />
          <Route path="/accounts/new" element={<AccountForm mode="create" />} />
          <Route path="/accounts/:id" element={<AccountForm mode="edit" />} />
          <Route path="/organizations" element={<Organizations />} />
          <Route
            path="/organizations/new"
            element={<OrganizationForm mode="create" />}
          />
          <Route
            path="/organizations/:id"
            element={<OrganizationForm mode="edit" />}
          />
          <Route path="/customers" element={<Customers />} />
          <Route path="/customers/new" element={<CustomerForm mode="create" />} />
          <Route path="/customers/:id" element={<CustomerForm mode="edit" />} />
          <Route path="/suppliers" element={<Suppliers />} />
          <Route path="/suppliers/new" element={<SupplierForm mode="create" />} />
          <Route path="/suppliers/:id" element={<SupplierForm mode="edit" />} />
          <Route path="/products" element={<Products />} />
          <Route path="/products/new" element={<ProductForm mode="create" />} />
          <Route path="/products/:id" element={<ProductForm mode="edit" />} />
          <Route path="/tax-codes" element={<TaxCodes />} />
          <Route path="/tax-codes/new" element={<TaxCodeForm mode="create" />} />
          <Route
            path="/tax-codes/:code"
            element={<TaxCodeForm mode="edit" />}
          />
          <Route path="/warehouses" element={<Warehouses />} />
          <Route
            path="/warehouses/new"
            element={<WarehouseForm mode="create" />}
          />
          <Route
            path="/warehouses/:id"
            element={<WarehouseForm mode="edit" />}
          />
          <Route path="/periods" element={<Periods />} />
          <Route path="/periods/new" element={<PeriodForm mode="create" />} />
          <Route path="/periods/:id" element={<PeriodForm mode="edit" />} />
          <Route
            path="/fiscal-years/new"
            element={<FiscalYearForm mode="create" />}
          />
          <Route
            path="/fiscal-years/:id"
            element={<FiscalYearForm mode="edit" />}
          />
          <Route path="/invoices" element={<Invoices />} />
          <Route path="/invoices/new" element={<InvoiceForm />} />
          <Route path="/invoices/:id" element={<InvoiceDetail />} />
          <Route path="/bills" element={<Bills />} />
          <Route path="/bills/new" element={<BillForm />} />
          <Route path="/bills/:id" element={<BillDetail />} />
          <Route path="/customer-payments" element={<CustomerPayments />} />
          <Route
            path="/customer-payments/new"
            element={<CustomerPaymentForm />}
          />
          <Route
            path="/customer-payments/:id"
            element={<CustomerPaymentDetail />}
          />
          <Route path="/supplier-payments" element={<SupplierPayments />} />
          <Route
            path="/supplier-payments/new"
            element={<SupplierPaymentForm />}
          />
          <Route
            path="/supplier-payments/:id"
            element={<SupplierPaymentDetail />}
          />
          <Route path="/stock-movements" element={<StockMovements />} />
          <Route path="/stock-movements/new" element={<StockMovementForm />} />
          <Route
            path="/stock-movements/:id"
            element={<StockMovementDetail />}
          />
          <Route path="/reports/trial-balance" element={<TrialBalance />} />
          <Route path="/reports/ar-aging" element={<ARAging />} />
          <Route path="/reports/ap-aging" element={<APAging />} />
          <Route path="/reports/inventory" element={<InventoryValuation />} />
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
