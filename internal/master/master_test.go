package master_test

import (
	"context"
	"errors"
	"testing"

	"tadmor/internal/dbtest"
	"tadmor/internal/master"
)

func TestMasterCRUD(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	dbtest.Reset(ctx, t, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Organization round-trip.
	orgID, err := master.CreateOrganization(ctx, tx, master.OrganizationInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if org, err := master.GetOrganization(ctx, tx, orgID); err != nil || org.Name != "Acme" {
		t.Fatalf("get organization = %+v, %v", org, err)
	}

	// Customer create + update (full replace).
	custID, err := master.CreateCustomer(ctx, tx, master.CustomerInput{OrganizationID: orgID})
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}
	num := "C-001"
	if err := master.UpdateCustomer(ctx, tx, custID, master.CustomerInput{
		OrganizationID: orgID, CustomerNumber: &num, IsActive: false,
	}); err != nil {
		t.Fatalf("update customer: %v", err)
	}
	cust, err := master.GetCustomer(ctx, tx, custID)
	if err != nil || cust.CustomerNumber == nil || *cust.CustomerNumber != "C-001" || cust.IsActive {
		t.Fatalf("get customer = %+v, %v", cust, err)
	}

	// Product with a decimal price comes back scale-4.
	prodID, err := master.CreateProduct(ctx, tx, master.ProductInput{SKU: "P-1", Name: "Widget", UnitPrice: "9.99"})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	if p, err := master.GetProduct(ctx, tx, prodID); err != nil || p.UnitPrice != "9.9900" {
		t.Fatalf("get product = %+v, %v", p, err)
	}
	if err := master.UpdateProduct(ctx, tx, prodID, master.ProductInput{SKU: "P-1", Name: "Widget Pro", UnitPrice: "12.50", IsActive: true}); err != nil {
		t.Fatalf("update product: %v", err)
	}
	if p, err := master.GetProduct(ctx, tx, prodID); err != nil || p.Name != "Widget Pro" || p.UnitPrice != "12.5000" {
		t.Fatalf("updated product = %+v, %v", p, err)
	}

	// Tax code (natural key) round-trip.
	if _, err := master.CreateTaxCode(ctx, tx, master.TaxCodeInput{Code: "VAT", Name: "VAT 20%", Rate: "20"}); err != nil {
		t.Fatalf("create tax code: %v", err)
	}
	if tc, err := master.GetTaxCode(ctx, tx, "VAT"); err != nil || tc.Rate != "20.0000" {
		t.Fatalf("get tax code = %+v, %v", tc, err)
	}

	// Payment term (natural key) round-trip with update.
	if _, err := master.CreatePaymentTerm(ctx, tx, master.PaymentTermInput{Code: "NET45", Name: "Net 45", DueDays: 45}); err != nil {
		t.Fatalf("create payment term: %v", err)
	}
	if err := master.UpdatePaymentTerm(ctx, tx, "NET45", master.PaymentTermInput{Code: "NET45", Name: "Net 45 days", DueDays: 45}); err != nil {
		t.Fatalf("update payment term: %v", err)
	}
	if pt, err := master.GetPaymentTerm(ctx, tx, "NET45"); err != nil || pt.Name != "Net 45 days" || pt.DueDays != 45 {
		t.Fatalf("get payment term = %+v, %v", pt, err)
	}

	// Smoke-create the remaining entities so every Create path is exercised.
	supID, err := master.CreateSupplier(ctx, tx, master.SupplierInput{OrganizationID: orgID})
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	if _, err := master.GetSupplier(ctx, tx, supID); err != nil {
		t.Fatalf("get supplier: %v", err)
	}
	acctID, err := master.CreateAccount(ctx, tx, master.AccountInput{
		Code: "9000", Name: "Suspense", AccountType: "asset", IsPostable: true,
		IsCash: true, CashFlowActivity: "investing",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if a, err := master.GetAccount(ctx, tx, acctID); err != nil || !a.IsCash || a.CashFlowActivity != "investing" {
		t.Fatalf("get account = %+v, %v; want is_cash investing", a, err)
	}
	if _, err := master.CreateWarehouse(ctx, tx, master.WarehouseInput{Code: "W1", Name: "Main"}); err != nil {
		t.Fatalf("create warehouse: %v", err)
	}
	fyID, err := master.CreateFiscalYear(ctx, tx, master.FiscalYearInput{Name: "FY2026", StartDate: "2026-01-01", EndDate: "2026-12-31"})
	if err != nil {
		t.Fatalf("create fiscal year: %v", err)
	}
	if _, err := master.CreateAccountingPeriod(ctx, tx, master.AccountingPeriodInput{
		FiscalYearID: fyID, Name: "2026-06", StartDate: "2026-06-01", EndDate: "2026-06-30",
	}); err != nil {
		t.Fatalf("create accounting period: %v", err)
	}

	// Lists return non-nil slices including what we created.
	if accts, err := master.ListAccounts(ctx, tx); err != nil || len(accts) == 0 {
		t.Fatalf("list accounts = %d, %v", len(accts), err)
	}

	// Missing record -> ErrNotFound.
	if _, err := master.GetProduct(ctx, tx, 999999); !errors.Is(err, master.ErrNotFound) {
		t.Errorf("get missing product err = %v, want ErrNotFound", err)
	}
	// Updating a missing record -> ErrNotFound.
	if err := master.UpdateWarehouse(ctx, tx, 999999, master.WarehouseInput{Code: "X", Name: "X"}); !errors.Is(err, master.ErrNotFound) {
		t.Errorf("update missing warehouse err = %v, want ErrNotFound", err)
	}
}
