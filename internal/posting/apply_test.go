package posting_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"tadmor/internal/dbtest"
	"tadmor/internal/posting"
)

func TestAutoApplyCustomerPayment(t *testing.T) {
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

	// Two posted invoices: older 30, newer 40.
	older := newPostedInvoice(ctx, t, tx, custID, "INV-A", "2026-06-10", 30)
	newer := newPostedInvoice(ctx, t, tx, custID, "INV-B", "2026-06-20", 40)

	// A posted 50 payment; auto-apply should clear the older invoice (30) and
	// partially pay the newer one (20), oldest first.
	payID := queryID(`INSERT INTO customer_payments (customer_id, payment_date, currency_code, amount, deposit_account_id)
	      VALUES ($1,'2026-06-25','USD',50,(SELECT id FROM accounts WHERE code='1000')) RETURNING id`, custID)
	if _, err := posting.PostCustomerPayment(ctx, tx, payID); err != nil {
		t.Fatalf("post payment: %v", err)
	}

	apps, err := posting.AutoApplyCustomerPayment(ctx, tx, payID)
	if err != nil {
		t.Fatalf("auto-apply: %v", err)
	}
	want := []posting.Application{
		{DocumentID: older, Amount: "30.0000"},
		{DocumentID: newer, Amount: "20.0000"},
	}
	if len(apps) != len(want) {
		t.Fatalf("got %d applications, want %d: %+v", len(apps), len(want), apps)
	}
	for i, w := range want {
		if apps[i] != w {
			t.Errorf("application %d = %+v, want %+v", i, apps[i], w)
		}
	}

	// The balance view (posted payments only) should now show A/R settled.
	assertBalance := func(invoiceID int, balance, status string) {
		t.Helper()
		var b, s string
		if err := tx.QueryRow(ctx,
			`SELECT balance::text, payment_status FROM sales_invoice_balances WHERE invoice_id = $1`,
			invoiceID).Scan(&b, &s); err != nil {
			t.Fatalf("read balance %d: %v", invoiceID, err)
		}
		if b != balance || s != status {
			t.Errorf("invoice %d: balance=%s status=%s, want balance=%s status=%s", invoiceID, b, s, balance, status)
		}
	}
	assertBalance(older, "0.0000", "paid")
	assertBalance(newer, "20.0000", "partial")
}

func TestAutoApplySupplierPayment(t *testing.T) {
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
	supID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Beta') RETURNING id)
	      INSERT INTO suppliers (organization_id, ap_account_id)
	      SELECT o.id,(SELECT id FROM accounts WHERE code='2000') FROM o RETURNING id`)

	older := newPostedBill(ctx, t, tx, supID, "BILL-A", "2026-06-05", 25)
	newer := newPostedBill(ctx, t, tx, supID, "BILL-B", "2026-06-15", 25)

	// A 30 payment clears the older bill (25) and partially pays the newer (5).
	payID := queryID(`INSERT INTO supplier_payments (supplier_id, payment_date, currency_code, amount, payment_account_id)
	      VALUES ($1,'2026-06-20','USD',30,(SELECT id FROM accounts WHERE code='1000')) RETURNING id`, supID)
	if _, err := posting.PostSupplierPayment(ctx, tx, payID); err != nil {
		t.Fatalf("post payment: %v", err)
	}

	apps, err := posting.AutoApplySupplierPayment(ctx, tx, payID)
	if err != nil {
		t.Fatalf("auto-apply: %v", err)
	}
	want := []posting.Application{
		{DocumentID: older, Amount: "25.0000"},
		{DocumentID: newer, Amount: "5.0000"},
	}
	if len(apps) != len(want) {
		t.Fatalf("got %d applications, want %d: %+v", len(apps), len(want), apps)
	}
	for i, w := range want {
		if apps[i] != w {
			t.Errorf("application %d = %+v, want %+v", i, apps[i], w)
		}
	}

	var b, s string
	if err := tx.QueryRow(ctx,
		`SELECT balance::text, payment_status FROM purchase_bill_balances WHERE bill_id = $1`, newer).Scan(&b, &s); err != nil {
		t.Fatalf("read bill balance: %v", err)
	}
	if b != "20.0000" || s != "partial" {
		t.Errorf("newer bill: balance=%s status=%s, want 20.0000/partial", b, s)
	}
}

// execAndQueryID returns small helpers bound to a transaction.
func execAndQueryID(ctx context.Context, t *testing.T, tx pgx.Tx) (func(string, ...any), func(string, ...any) int) {
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
	return exec, queryID
}

// newPostedInvoice creates and posts a single-line invoice for the given gross
// total (no tax) and returns its id.
func newPostedInvoice(ctx context.Context, t *testing.T, tx pgx.Tx, customerID int, number, date string, total int) int {
	t.Helper()
	var id int
	if err := tx.QueryRow(ctx,
		`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, currency_code)
		 VALUES ($1,$2,$3::date,'USD') RETURNING id`, number, customerID, date).Scan(&id); err != nil {
		t.Fatalf("create invoice %s: %v", number, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO sales_invoice_lines (invoice_id, line_no, description, quantity, unit_price, revenue_account_id)
		 VALUES ($1,1,'Item',$2,1,(SELECT id FROM accounts WHERE code='4000'))`, id, total); err != nil {
		t.Fatalf("create invoice line %s: %v", number, err)
	}
	if _, err := posting.PostSalesInvoice(ctx, tx, id); err != nil {
		t.Fatalf("post invoice %s: %v", number, err)
	}
	return id
}

// newPostedBill creates and posts a single-line bill for the given gross total.
func newPostedBill(ctx context.Context, t *testing.T, tx pgx.Tx, supplierID int, number, date string, total int) int {
	t.Helper()
	var id int
	if err := tx.QueryRow(ctx,
		`INSERT INTO purchase_bills (bill_number, supplier_id, bill_date, currency_code)
		 VALUES ($1,$2,$3::date,'USD') RETURNING id`, number, supplierID, date).Scan(&id); err != nil {
		t.Fatalf("create bill %s: %v", number, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO purchase_bill_lines (bill_id, line_no, description, quantity, unit_cost, expense_account_id)
		 VALUES ($1,1,'Item',$2,1,(SELECT id FROM accounts WHERE code='6000'))`, id, total); err != nil {
		t.Fatalf("create bill line %s: %v", number, err)
	}
	if _, err := posting.PostPurchaseBill(ctx, tx, id); err != nil {
		t.Fatalf("post bill %s: %v", number, err)
	}
	return id
}
