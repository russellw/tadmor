import { NavLink, Navigate, Route, Routes } from "react-router-dom"

import { AccountForm } from "@/components/account-form"
import { APAging, ARAging } from "@/components/aging-report"
import { BillForm } from "@/components/bill-form"
import { ChartOfAccounts } from "@/components/chart-of-accounts"
import { CustomerForm } from "@/components/customer-form"
import { Customers } from "@/components/customers"
import { BillDetail, InvoiceDetail } from "@/components/document-detail"
import { Bills, Invoices } from "@/components/document-list"
import { InventoryValuation } from "@/components/inventory-valuation"
import { InvoiceForm } from "@/components/invoice-form"
import { OrganizationForm } from "@/components/organization-form"
import { Organizations } from "@/components/organizations"
import { ProductForm } from "@/components/product-form"
import { Products } from "@/components/products"
import { SupplierForm } from "@/components/supplier-form"
import { Suppliers } from "@/components/suppliers"
import { TrialBalance } from "@/components/trial-balance"
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
]

const documentNavItems = [
  { to: "/invoices", label: "Invoices" },
  { to: "/bills", label: "Bills" },
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
  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b">
        <nav className="mx-auto flex w-full max-w-5xl flex-wrap gap-1 p-3">
          <NavItems items={masterNavItems} />
          <span aria-hidden className="mx-1 my-auto h-4 w-px bg-border" />
          <NavItems items={documentNavItems} />
          <span aria-hidden className="mx-1 my-auto h-4 w-px bg-border" />
          <NavItems items={reportNavItems} />
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
          <Route path="/invoices" element={<Invoices />} />
          <Route path="/invoices/new" element={<InvoiceForm />} />
          <Route path="/invoices/:id" element={<InvoiceDetail />} />
          <Route path="/bills" element={<Bills />} />
          <Route path="/bills/new" element={<BillForm />} />
          <Route path="/bills/:id" element={<BillDetail />} />
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
