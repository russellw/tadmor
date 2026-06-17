package documents_test

import (
	"context"
	"testing"

	"tadmor/internal/dbtest"
	"tadmor/internal/documents"
)

func TestCreateSalesInvoiceComputesTotals(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	dbtest.Reset(ctx, t, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var custID, revAcct int
	if err := tx.QueryRow(ctx,
		`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
		 INSERT INTO customers (organization_id) SELECT id FROM o RETURNING id`).Scan(&custID); err != nil {
		t.Fatalf("create customer: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM accounts WHERE code='4000'`).Scan(&revAcct); err != nil {
		t.Fatalf("revenue account: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO tax_codes (code,name,rate,tax_account_id) VALUES ('T10','10% tax',10,(SELECT id FROM accounts WHERE code='2100'))`); err != nil {
		t.Fatalf("tax code: %v", err)
	}

	tc := "T10"
	in := documents.SalesInvoiceInput{
		InvoiceNumber: "INV-1",
		CustomerID:    custID,
		InvoiceDate:   "2026-06-15",
		CurrencyCode:  "USD",
		Lines: []documents.SalesInvoiceLineInput{
			{Description: "Widget", Quantity: "10", UnitPrice: "5", RevenueAccountID: &revAcct, TaxCode: &tc, TaxRate: "10"}, // 50 + 5
			{Description: "Gadget", Quantity: "2", UnitPrice: "20", RevenueAccountID: &revAcct},                              // 40 + 0
		},
	}
	id, err := documents.CreateSalesInvoice(ctx, tx, in)
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}

	// The header totals were maintained from the lines by trigger.
	var subtotal, tax, total, status string
	if err := tx.QueryRow(ctx,
		`SELECT subtotal::text, tax_total::text, total::text, status FROM sales_invoices WHERE id = $1`, id).
		Scan(&subtotal, &tax, &total, &status); err != nil {
		t.Fatalf("read invoice: %v", err)
	}
	if subtotal != "90.0000" || tax != "5.0000" || total != "95.0000" {
		t.Errorf("totals = %s/%s/%s, want 90.0000/5.0000/95.0000", subtotal, tax, total)
	}
	if status != "draft" {
		t.Errorf("status = %s, want draft", status)
	}

	var lineCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM sales_invoice_lines WHERE invoice_id = $1`, id).Scan(&lineCount); err != nil {
		t.Fatalf("count lines: %v", err)
	}
	if lineCount != 2 {
		t.Errorf("line count = %d, want 2", lineCount)
	}
}

func TestCreateStockMovement(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	dbtest.Reset(ctx, t, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var prodID, whID int
	if err := tx.QueryRow(ctx,
		`INSERT INTO products (sku,name,track_inventory) VALUES ('P-INV','Widget',true) RETURNING id`).Scan(&prodID); err != nil {
		t.Fatalf("create product: %v", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO warehouses (code,name) VALUES ('MAIN','Main') RETURNING id`).Scan(&whID); err != nil {
		t.Fatalf("create warehouse: %v", err)
	}

	id, err := documents.CreateStockMovement(ctx, tx, documents.StockMovementInput{
		ProductID:    prodID,
		WarehouseID:  whID,
		MovementType: "receipt",
		Quantity:     "10",
		UnitCost:     "7",
	})
	if err != nil {
		t.Fatalf("create movement: %v", err)
	}

	var totalCost string
	if err := tx.QueryRow(ctx, `SELECT total_cost::text FROM stock_movements WHERE id = $1`, id).Scan(&totalCost); err != nil {
		t.Fatalf("read movement: %v", err)
	}
	if totalCost != "70.0000" {
		t.Errorf("total_cost = %s, want 70.0000", totalCost)
	}
}
