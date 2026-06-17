package reporting_test

import (
	"context"
	"errors"
	"testing"

	"tadmor/internal/dbtest"
	"tadmor/internal/posting"
	"tadmor/internal/reporting"
)

func TestReportingQueries(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	dbtest.Reset(ctx, t, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("setup exec: %v\nsql: %s", err, sql)
		}
	}
	queryID := func(sql string, args ...any) int {
		t.Helper()
		var id int
		if err := tx.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("setup query: %v\nsql: %s", err, sql)
		}
		return id
	}

	// A period wide enough to contain current_date-relative dates (aging buckets
	// are computed against current_date).
	exec(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY', current_date - 400, current_date + 400)`)
	exec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
	      SELECT id,'P', current_date - 400, current_date + 400 FROM fiscal_years WHERE name='FY'`)
	custID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id,(SELECT id FROM accounts WHERE code='1100') FROM o RETURNING id`)

	// An invoice overdue by ~5 days lands in the 1-30 aging bucket.
	invID := queryID(`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, due_date, currency_code)
	      VALUES ('INV-1',$1, current_date - 10, current_date - 5, 'USD') RETURNING id`, custID)
	exec(`INSERT INTO sales_invoice_lines (invoice_id, line_no, description, quantity, unit_price, revenue_account_id)
	      VALUES ($1,1,'Service',100,1,(SELECT id FROM accounts WHERE code='4000'))`, invID)
	if _, err := posting.PostSalesInvoice(ctx, tx, invID); err != nil {
		t.Fatalf("post invoice: %v", err)
	}

	// Stock for valuation (the valuation view sums movements; no GL posting needed).
	invProd := queryID(`INSERT INTO products (sku, name, track_inventory, inventory_account_id, cogs_account_id)
	      VALUES ('P-INV','Widget',true,(SELECT id FROM accounts WHERE code='1200'),(SELECT id FROM accounts WHERE code='5000')) RETURNING id`)
	whID := queryID(`INSERT INTO warehouses (code, name) VALUES ('MAIN','Main') RETURNING id`)
	exec(`INSERT INTO stock_movements (product_id, warehouse_id, movement_type, quantity, unit_cost)
	      VALUES ($1,$2,'receipt',10,7)`, invProd, whID)

	t.Run("trial balance", func(t *testing.T) {
		tb, err := reporting.TrialBalance(ctx, tx)
		if err != nil {
			t.Fatalf("trial balance: %v", err)
		}
		bal := map[string]string{}
		for _, r := range tb {
			bal[r.Code] = r.Balance
		}
		if bal["1100"] != "100.0000" {
			t.Errorf("A/R balance = %s, want 100.0000", bal["1100"])
		}
		if bal["4000"] != "-100.0000" {
			t.Errorf("revenue balance = %s, want -100.0000", bal["4000"])
		}
		// Zero-activity accounts are still scale-4, not a bare "0".
		if bal["1000"] != "0.0000" {
			t.Errorf("cash balance = %q, want 0.0000", bal["1000"])
		}
	})

	t.Run("single invoice", func(t *testing.T) {
		inv, err := reporting.SalesInvoiceBalance(ctx, tx, invID)
		if err != nil {
			t.Fatalf("invoice balance: %v", err)
		}
		if inv.Total != "100.0000" || inv.Balance != "100.0000" || inv.PaymentStatus != "unpaid" {
			t.Errorf("invoice = %+v, want total/balance 100.0000 and unpaid", inv)
		}
		if inv.AmountApplied != "0.0000" {
			t.Errorf("amount_applied = %q, want 0.0000", inv.AmountApplied)
		}
	})

	t.Run("AR aging", func(t *testing.T) {
		aging, err := reporting.ARaging(ctx, tx)
		if err != nil {
			t.Fatalf("ar aging: %v", err)
		}
		if len(aging) != 1 {
			t.Fatalf("aging rows = %d, want 1", len(aging))
		}
		a := aging[0]
		if a.PartyID != custID || a.TotalOutstanding != "100.0000" || a.Days1To30 != "100.0000" || a.NotYetDue != "0.0000" {
			t.Errorf("aging = %+v, want party %d, 100.0000 in days_1_30", a, custID)
		}
	})

	t.Run("stock valuation", func(t *testing.T) {
		val, err := reporting.StockValuation(ctx, tx)
		if err != nil {
			t.Fatalf("valuation: %v", err)
		}
		if len(val) != 1 || val[0].QtyOnHand != "10.0000" || val[0].ValueOnHand != "70.0000" || val[0].AvgUnitCost != "7.0000" {
			t.Errorf("valuation = %+v, want qty 10 / value 70 / avg 7", val)
		}
	})

	t.Run("missing invoice", func(t *testing.T) {
		if _, err := reporting.SalesInvoiceBalance(ctx, tx, 999999); !errors.Is(err, reporting.ErrNotFound) {
			t.Errorf("error = %v, want ErrNotFound", err)
		}
	})
}
