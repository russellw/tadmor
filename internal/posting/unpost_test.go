package posting_test

import (
	"context"
	"errors"
	"testing"

	"tadmor/internal/dbtest"
	"tadmor/internal/posting"
)

func TestUnpostSalesInvoice(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	dbtest.Reset(ctx, t, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	exec, queryID := execAndQueryID(ctx, t, tx)
	exec(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY2026','2026-01-01','2026-12-31')`)
	exec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
	      SELECT id,'2026-06','2026-06-01','2026-06-30' FROM fiscal_years WHERE name='FY2026'`)
	custID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id,(SELECT id FROM accounts WHERE code='1100') FROM o RETURNING id`)

	inv := newPostedInvoice(ctx, t, tx, custID, "INV-1", "2026-06-15", 100)
	origJE := queryID(`SELECT journal_entry_id FROM sales_invoices WHERE id = $1`, inv)

	rev, err := posting.UnpostSalesInvoice(ctx, tx, inv)
	if err != nil {
		t.Fatalf("unpost: %v", err)
	}
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatalf("reversal entry unbalanced: %v", err)
	}

	// Invoice is back to draft and unlinked.
	var status string
	var je *int
	if err := tx.QueryRow(ctx, `SELECT status, journal_entry_id FROM sales_invoices WHERE id = $1`, inv).Scan(&status, &je); err != nil {
		t.Fatalf("read invoice: %v", err)
	}
	if status != "draft" || je != nil {
		t.Fatalf("invoice status=%s journal_entry_id=%v, want draft/nil", status, je)
	}

	// The reversal is linked to the original.
	var reverses int
	if err := tx.QueryRow(ctx, `SELECT reverses_entry_id FROM journal_entries WHERE id = $1`, rev).Scan(&reverses); err != nil {
		t.Fatalf("read reversal link: %v", err)
	}
	if reverses != origJE {
		t.Errorf("reversal reverses_entry_id = %d, want %d", reverses, origJE)
	}

	// Original + reversal net to zero across the affected accounts.
	for _, code := range []string{"1100", "4000"} {
		var bal string
		if err := tx.QueryRow(ctx, `SELECT balance::text FROM trial_balance WHERE code = $1`, code).Scan(&bal); err != nil {
			t.Fatalf("balance %s: %v", code, err)
		}
		if bal != "0.0000" {
			t.Errorf("account %s balance = %s, want 0.0000 after reversal", code, bal)
		}
	}

	// Unposting again fails (no longer posted), and a second reversal is refused.
	if _, err := posting.UnpostSalesInvoice(ctx, tx, inv); !errors.Is(err, posting.ErrNotPosted) {
		t.Errorf("second unpost error = %v, want ErrNotPosted", err)
	}
}

func TestUnpostCustomerPaymentUnwindsApplications(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	dbtest.Reset(ctx, t, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	exec, queryID := execAndQueryID(ctx, t, tx)
	exec(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY2026','2026-01-01','2026-12-31')`)
	exec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
	      SELECT id,'2026-06','2026-06-01','2026-06-30' FROM fiscal_years WHERE name='FY2026'`)
	custID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id,(SELECT id FROM accounts WHERE code='1100') FROM o RETURNING id`)

	inv := newPostedInvoice(ctx, t, tx, custID, "INV-1", "2026-06-10", 50)
	payID := queryID(`INSERT INTO customer_payments (customer_id, payment_date, currency_code, amount, deposit_account_id)
	      VALUES ($1,'2026-06-12','USD',50,(SELECT id FROM accounts WHERE code='1000')) RETURNING id`, custID)
	if _, err := posting.PostCustomerPayment(ctx, tx, payID); err != nil {
		t.Fatalf("post payment: %v", err)
	}
	if _, err := posting.AutoApplyCustomerPayment(ctx, tx, payID); err != nil {
		t.Fatalf("auto-apply: %v", err)
	}

	// While the payment is applied, the invoice cannot be unposted.
	if _, err := posting.UnpostSalesInvoice(ctx, tx, inv); !errors.Is(err, posting.ErrHasApplications) {
		t.Fatalf("unpost applied invoice error = %v, want ErrHasApplications", err)
	}

	// Unposting the payment unwinds the application and clears the GL effect.
	if _, err := posting.UnpostCustomerPayment(ctx, tx, payID); err != nil {
		t.Fatalf("unpost payment: %v", err)
	}

	var apps int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM payment_applications WHERE payment_id = $1`, payID).Scan(&apps); err != nil {
		t.Fatalf("count applications: %v", err)
	}
	if apps != 0 {
		t.Errorf("applications after unpost = %d, want 0", apps)
	}

	var balance, paymentStatus string
	if err := tx.QueryRow(ctx,
		`SELECT balance::text, payment_status FROM sales_invoice_balances WHERE invoice_id = $1`, inv).Scan(&balance, &paymentStatus); err != nil {
		t.Fatalf("invoice balance: %v", err)
	}
	if balance != "50.0000" || paymentStatus != "unpaid" {
		t.Errorf("invoice balance=%s status=%s, want 50.0000/unpaid", balance, paymentStatus)
	}

	// Cash nets to zero (payment posted then reversed).
	var cash string
	if err := tx.QueryRow(ctx, `SELECT balance::text FROM trial_balance WHERE code='1000'`).Scan(&cash); err != nil {
		t.Fatalf("cash balance: %v", err)
	}
	if cash != "0.0000" {
		t.Errorf("cash balance = %s, want 0.0000", cash)
	}

	// Now the invoice (no longer applied) can be unposted.
	if _, err := posting.UnpostSalesInvoice(ctx, tx, inv); err != nil {
		t.Fatalf("unpost unapplied invoice: %v", err)
	}

	// Validate every entry balances. This goes last: SET CONSTRAINTS ALL
	// IMMEDIATE is sticky for the rest of the transaction, and posting inserts an
	// entry header before its lines, so no further posting may follow it.
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatalf("a reversal entry is unbalanced: %v", err)
	}
}

func TestUnpostStockMovement(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	dbtest.Reset(ctx, t, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	exec, queryID := execAndQueryID(ctx, t, tx)
	exec(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY2026','2026-01-01','2026-12-31')`)
	exec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
	      SELECT id,'2026-06','2026-06-01','2026-06-30' FROM fiscal_years WHERE name='FY2026'`)
	grni := queryID(`SELECT id FROM accounts WHERE code='2150'`)
	invProd := queryID(`INSERT INTO products (sku, name, track_inventory, inventory_account_id, cogs_account_id)
	      VALUES ('P-INV','Widget',true,(SELECT id FROM accounts WHERE code='1200'),(SELECT id FROM accounts WHERE code='5000')) RETURNING id`)
	whID := queryID(`INSERT INTO warehouses (code, name) VALUES ('MAIN','Main') RETURNING id`)
	movID := queryID(`INSERT INTO stock_movements (product_id, warehouse_id, movement_type, quantity, unit_cost)
	      VALUES ($1,$2,'receipt',10,7) RETURNING id`, invProd, whID)

	if _, err := posting.PostInventoryReceipt(ctx, tx, movID, "USD", grni); err != nil {
		t.Fatalf("post receipt: %v", err)
	}
	if _, err := posting.UnpostStockMovement(ctx, tx, movID); err != nil {
		t.Fatalf("unpost movement: %v", err)
	}
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatalf("reversal entry unbalanced: %v", err)
	}

	var je *int
	if err := tx.QueryRow(ctx, `SELECT journal_entry_id FROM stock_movements WHERE id = $1`, movID).Scan(&je); err != nil {
		t.Fatalf("read movement: %v", err)
	}
	if je != nil {
		t.Errorf("movement journal_entry_id = %v, want nil after unpost", je)
	}
	for _, code := range []string{"1200", "2150"} {
		var bal string
		if err := tx.QueryRow(ctx, `SELECT balance::text FROM trial_balance WHERE code = $1`, code).Scan(&bal); err != nil {
			t.Fatalf("balance %s: %v", code, err)
		}
		if bal != "0.0000" {
			t.Errorf("account %s balance = %s, want 0.0000 after reversal", code, bal)
		}
	}

	if _, err := posting.UnpostStockMovement(ctx, tx, movID); !errors.Is(err, posting.ErrNotPosted) {
		t.Errorf("second unpost error = %v, want ErrNotPosted", err)
	}
}
