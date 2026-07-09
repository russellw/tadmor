import { useEffect, useState } from "react"
import { NavLink, Route, Routes } from "react-router-dom"
import { MenuIcon } from "lucide-react"

import { logout, me, UNAUTHORIZED_EVENT, type User } from "@/lib/api"
import { LoginForm } from "@/components/login-form"
import { Button } from "@/components/ui/button"
import { AccountForm } from "@/components/account-form"
import { AccountLedger } from "@/components/account-ledger"
import { APAging, ARAging } from "@/components/aging-report"
import { BalanceSheetReport } from "@/components/balance-sheet"
import { BankStatementDetail } from "@/components/bank-statement-detail"
import { BankStatementForm } from "@/components/bank-statement-form"
import { BankStatements } from "@/components/bank-statements"
import { BillForm } from "@/components/bill-form"
import { CashFlowStatement } from "@/components/cash-flow"
import { ChartOfAccounts } from "@/components/chart-of-accounts"
import { CustomerForm } from "@/components/customer-form"
import { Customers } from "@/components/customers"
import { Dashboard } from "@/components/dashboard"
import { FiscalYearClose } from "@/components/fiscal-year-close"
import { FiscalYearForm } from "@/components/fiscal-year-form"
import { CreditNoteForm, SupplierCreditForm } from "@/components/credit-note-form"
import {
  BillDetail,
  CreditNoteDetail,
  InvoiceDetail,
  SupplierCreditDetail,
} from "@/components/document-detail"
import {
  Bills,
  CreditNotes,
  Invoices,
  SupplierCredits,
} from "@/components/document-list"
import { ExchangeRateForm } from "@/components/exchange-rate-form"
import { ExchangeRates } from "@/components/exchange-rates"
import { InventoryValuation } from "@/components/inventory-valuation"
import { InvoiceForm } from "@/components/invoice-form"
import { JournalEntryDetail } from "@/components/journal-entry"
import { PurchaseOrders, SalesOrders } from "@/components/order-list"
import { PurchaseOrderForm, SalesOrderForm } from "@/components/order-form"
import {
  PurchaseOrderDetail,
  SalesOrderDetail,
} from "@/components/order-detail"
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
import { PaymentTermForm } from "@/components/payment-term-form"
import { PaymentTerms } from "@/components/payment-terms"
import { PeriodForm } from "@/components/period-form"
import { Periods } from "@/components/periods"
import { StockMovementDetail } from "@/components/stock-movement-detail"
import { StockMovementForm } from "@/components/stock-movement-form"
import { StockMovements } from "@/components/stock-movements"
import { OrganizationForm } from "@/components/organization-form"
import { Organizations } from "@/components/organizations"
import { ProductForm } from "@/components/product-form"
import { Products } from "@/components/products"
import { ProfitAndLoss } from "@/components/profit-and-loss"
import { SettingsScreen } from "@/components/settings"
import { SupplierForm } from "@/components/supplier-form"
import { Suppliers } from "@/components/suppliers"
import { TaxCodeForm } from "@/components/tax-code-form"
import { TaxCodes } from "@/components/tax-codes"
import { TrialBalance } from "@/components/trial-balance"
import { UserForm } from "@/components/user-form"
import { Users } from "@/components/users"
import { WarehouseForm } from "@/components/warehouse-form"
import { Warehouses } from "@/components/warehouses"
import { CurrentUserContext } from "@/lib/current-user"
import { cn } from "@/lib/utils"

// URL routing via react-router-dom (v7). The Go backend's spaHandler falls back
// to index.html for non-/api/ paths, so deep links like /customers resolve in
// production; Vite's dev server does the same in development.
//
// Navigation is a fixed sidebar grouped by business domain, ordered roughly by
// how often each group is touched (daily document work first, configure-once
// data last).
const navGroups = [
  {
    label: "Sales",
    items: [
      { to: "/sales-orders", label: "Sales Orders" },
      { to: "/invoices", label: "Invoices" },
      { to: "/credit-notes", label: "Credit Notes" },
      { to: "/customer-payments", label: "Customer Payments" },
      { to: "/customers", label: "Customers" },
    ],
  },
  {
    label: "Purchases",
    items: [
      { to: "/purchase-orders", label: "Purchase Orders" },
      { to: "/bills", label: "Bills" },
      { to: "/supplier-credits", label: "Supplier Credits" },
      { to: "/supplier-payments", label: "Supplier Payments" },
      { to: "/suppliers", label: "Suppliers" },
    ],
  },
  {
    label: "Inventory",
    items: [
      { to: "/products", label: "Products" },
      { to: "/stock-movements", label: "Stock Movements" },
      { to: "/warehouses", label: "Warehouses" },
    ],
  },
  {
    label: "Reports",
    items: [
      { to: "/reports/profit-and-loss", label: "Profit & Loss" },
      { to: "/reports/balance-sheet", label: "Balance Sheet" },
      { to: "/reports/cash-flow", label: "Cash Flow" },
      { to: "/reports/trial-balance", label: "Trial Balance" },
      { to: "/reports/ar-aging", label: "AR Aging" },
      { to: "/reports/ap-aging", label: "AP Aging" },
      { to: "/reports/inventory", label: "Inventory Valuation" },
    ],
  },
  {
    label: "Accounting",
    items: [
      { to: "/accounts", label: "Chart of Accounts" },
      { to: "/bank-statements", label: "Bank Reconciliation" },
      { to: "/exchange-rates", label: "Exchange Rates" },
      { to: "/periods", label: "Periods" },
    ],
  },
  {
    label: "Setup",
    items: [
      { to: "/organizations", label: "Organizations" },
      { to: "/tax-codes", label: "Tax Codes" },
      { to: "/payment-terms", label: "Payment Terms" },
      { to: "/settings", label: "Settings" },
      // Users is appended for administrators only; the backend enforces the
      // same rule on the /users endpoints.
    ],
  },
]

const adminNavItems = [{ to: "/users", label: "Users" }]

function SidebarLink({
  to,
  label,
  end,
}: {
  to: string
  label: string
  end?: boolean
}) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        cn(
          "block rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
          isActive
            ? "bg-sidebar-accent text-sidebar-accent-foreground"
            : "text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground",
        )
      }
    >
      {label}
    </NavLink>
  )
}

// onNavigate lets the mobile drawer close on any link click; clicks bubble up
// from the NavLinks.
function SidebarNav({
  isAdmin,
  onNavigate,
}: {
  isAdmin: boolean
  onNavigate: () => void
}) {
  return (
    <nav className="flex-1 overflow-y-auto px-3 pb-4" onClick={onNavigate}>
      {/* `end` keeps Home from matching every route ("/" is a prefix of all). */}
      <SidebarLink to="/" label="Home" end />
      {navGroups.map((group) => (
        <div key={group.label}>
          <p className="px-3 pb-1 pt-5 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            {group.label}
          </p>
          {(group.label === "Setup" && isAdmin
            ? [...group.items, ...adminNavItems]
            : group.items
          ).map((item) => (
            <SidebarLink key={item.to} to={item.to} label={item.label} />
          ))}
        </div>
      ))}
    </nav>
  )
}

export default function App() {
  // undefined = still probing the session, null = logged out.
  const [user, setUser] = useState<User | null | undefined>(undefined)
  // Mobile-only: whether the sidebar drawer is open. On md+ the sidebar is
  // always visible and this state is ignored.
  const [navOpen, setNavOpen] = useState(false)

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
    <CurrentUserContext.Provider value={user}>
    <div className="flex min-h-screen bg-background text-foreground">
      {navOpen && (
        <div
          aria-hidden
          className="fixed inset-0 z-30 bg-black/40 md:hidden"
          onClick={() => setNavOpen(false)}
        />
      )}
      <aside
        className={cn(
          "z-40 w-60 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground",
          "md:sticky md:top-0 md:flex md:h-screen",
          navOpen ? "fixed inset-y-0 left-0 flex" : "hidden",
        )}
      >
        <div className="px-6 pt-5 text-lg font-semibold tracking-tight">
          Tadmor
        </div>
        <SidebarNav
          isAdmin={user.is_admin}
          onNavigate={() => setNavOpen(false)}
        />
      </aside>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center gap-2 border-b px-4 py-2 md:px-6">
          <Button
            variant="outline"
            size="sm"
            className="md:hidden"
            aria-label="Open navigation"
            onClick={() => setNavOpen(true)}
          >
            <MenuIcon className="size-4" />
          </Button>
          <span className="ml-auto flex items-center gap-2">
            <span className="text-sm text-muted-foreground">
              {user.full_name}
            </span>
            <Button variant="outline" size="sm" onClick={handleSignOut}>
              Sign out
            </Button>
          </span>
        </header>
        <main className="flex-1">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/accounts" element={<ChartOfAccounts />} />
          <Route path="/accounts/new" element={<AccountForm mode="create" />} />
          <Route path="/accounts/:id" element={<AccountForm mode="edit" />} />
          <Route path="/accounts/:id/ledger" element={<AccountLedger />} />
          <Route
            path="/journal-entries/:id"
            element={<JournalEntryDetail />}
          />
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
          <Route path="/payment-terms" element={<PaymentTerms />} />
          <Route
            path="/payment-terms/new"
            element={<PaymentTermForm mode="create" />}
          />
          <Route
            path="/payment-terms/:code"
            element={<PaymentTermForm mode="edit" />}
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
          <Route path="/bank-statements" element={<BankStatements />} />
          <Route
            path="/bank-statements/new"
            element={<BankStatementForm mode="create" />}
          />
          <Route
            path="/bank-statements/:id"
            element={<BankStatementDetail />}
          />
          <Route
            path="/bank-statements/:id/edit"
            element={<BankStatementForm mode="edit" />}
          />
          <Route path="/exchange-rates" element={<ExchangeRates />} />
          <Route
            path="/exchange-rates/new"
            element={<ExchangeRateForm mode="create" />}
          />
          <Route
            path="/exchange-rates/:currency/:date"
            element={<ExchangeRateForm mode="edit" />}
          />
          <Route path="/settings" element={<SettingsScreen />} />
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
          <Route
            path="/fiscal-years/:id/close"
            element={<FiscalYearClose />}
          />
          <Route path="/users" element={<Users />} />
          <Route path="/users/new" element={<UserForm mode="create" />} />
          <Route path="/users/:id" element={<UserForm mode="edit" />} />
          <Route path="/sales-orders" element={<SalesOrders />} />
          <Route path="/sales-orders/new" element={<SalesOrderForm />} />
          <Route path="/sales-orders/:id" element={<SalesOrderDetail />} />
          <Route
            path="/sales-orders/:id/edit"
            element={<SalesOrderForm mode="edit" />}
          />
          <Route path="/purchase-orders" element={<PurchaseOrders />} />
          <Route
            path="/purchase-orders/new"
            element={<PurchaseOrderForm />}
          />
          <Route
            path="/purchase-orders/:id"
            element={<PurchaseOrderDetail />}
          />
          <Route
            path="/purchase-orders/:id/edit"
            element={<PurchaseOrderForm mode="edit" />}
          />
          <Route path="/invoices" element={<Invoices />} />
          <Route path="/invoices/new" element={<InvoiceForm />} />
          <Route path="/invoices/:id" element={<InvoiceDetail />} />
          <Route
            path="/invoices/:id/edit"
            element={<InvoiceForm mode="edit" />}
          />
          <Route path="/bills" element={<Bills />} />
          <Route path="/bills/new" element={<BillForm />} />
          <Route path="/bills/:id" element={<BillDetail />} />
          <Route path="/bills/:id/edit" element={<BillForm mode="edit" />} />
          <Route path="/credit-notes" element={<CreditNotes />} />
          <Route path="/credit-notes/new" element={<CreditNoteForm />} />
          <Route path="/credit-notes/:id" element={<CreditNoteDetail />} />
          <Route
            path="/credit-notes/:id/edit"
            element={<CreditNoteForm mode="edit" />}
          />
          <Route path="/supplier-credits" element={<SupplierCredits />} />
          <Route
            path="/supplier-credits/new"
            element={<SupplierCreditForm />}
          />
          <Route
            path="/supplier-credits/:id"
            element={<SupplierCreditDetail />}
          />
          <Route
            path="/supplier-credits/:id/edit"
            element={<SupplierCreditForm mode="edit" />}
          />
          <Route path="/customer-payments" element={<CustomerPayments />} />
          <Route
            path="/customer-payments/new"
            element={<CustomerPaymentForm />}
          />
          <Route
            path="/customer-payments/:id"
            element={<CustomerPaymentDetail />}
          />
          <Route
            path="/customer-payments/:id/edit"
            element={<CustomerPaymentForm mode="edit" />}
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
          <Route
            path="/supplier-payments/:id/edit"
            element={<SupplierPaymentForm mode="edit" />}
          />
          <Route path="/stock-movements" element={<StockMovements />} />
          <Route path="/stock-movements/new" element={<StockMovementForm />} />
          <Route
            path="/stock-movements/:id"
            element={<StockMovementDetail />}
          />
          <Route
            path="/stock-movements/:id/edit"
            element={<StockMovementForm mode="edit" />}
          />
          <Route
            path="/reports/profit-and-loss"
            element={<ProfitAndLoss />}
          />
          <Route
            path="/reports/balance-sheet"
            element={<BalanceSheetReport />}
          />
          <Route path="/reports/cash-flow" element={<CashFlowStatement />} />
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
    </div>
    </CurrentUserContext.Provider>
  )
}
