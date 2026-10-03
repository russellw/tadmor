package posting_test

import (
	"context"
	"errors"
	"testing"

	"tadmor/internal/dbtest"
	"tadmor/internal/posting"
)

// TestPostingBalances posts one of every document type and asserts the resulting
// ledger balances overall and per account. Everything runs in a single
// transaction that is rolled back, so it leaves no data behind.
func TestPostingBalances(t *testing.T) {
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

	// Fiscal calendar + a tax code that points at the sales-tax-payable account.
	exec(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY2026','2026-01-01','2026-12-31')`)
	exec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
	      SELECT id,'2026-06','2026-06-01','2026-06-30' FROM fiscal_years WHERE name='FY2026'`)
	exec(`INSERT INTO tax_codes (code, name, rate, tax_account_id)
	      VALUES ('T10','10% tax',10,(SELECT id FROM accounts WHERE code='2100'))`)

	// A customer (A/R = 1100) and a supplier (A/P = 2000).
	custID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme Customer') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id, (SELECT id FROM accounts WHERE code='1100') FROM o RETURNING id`)
	supID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Beta Supplier') RETURNING id)
	      INSERT INTO suppliers (organization_id, ap_account_id)
	      SELECT o.id, (SELECT id FROM accounts WHERE code='2000') FROM o RETURNING id`)

	// A service product (revenue 4000) and an inventory product (inv 1200 / COGS 5000).
	revProd := queryID(`INSERT INTO products (sku, name, revenue_account_id, tax_code)
	      VALUES ('P-REV','Service',(SELECT id FROM accounts WHERE code='4000'),'T10') RETURNING id`)
	invProd := queryID(`INSERT INTO products (sku, name, track_inventory, inventory_account_id, cogs_account_id)
	      VALUES ('P-INV','Widget',true,(SELECT id FROM accounts WHERE code='1200'),(SELECT id FROM accounts WHERE code='5000')) RETURNING id`)
	whID := queryID(`INSERT INTO warehouses (code, name) VALUES ('MAIN','Main') RETURNING id`)

	// Sales invoice: 10 x 5 @ 10% = 50 net + 5 tax = 55 gross.
	invID := queryID(`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, currency_code)
	      VALUES ('INV-1',$1,'2026-06-15','USD') RETURNING id`, custID)
	exec(`INSERT INTO sales_invoice_lines (invoice_id, line_no, product_id, description, quantity, unit_price, revenue_account_id, tax_code, tax_rate)
	      VALUES ($1,1,$2,'Service',10,5,(SELECT id FROM accounts WHERE code='4000'),'T10',10)`, invID, revProd)

	// Purchase bill: 2 x 20 @ 10% = 40 net + 4 tax = 44 gross.
	billID := queryID(`INSERT INTO purchase_bills (bill_number, supplier_id, bill_date, currency_code)
	      VALUES ('BILL-1',$1,'2026-06-15','USD') RETURNING id`, supID)
	exec(`INSERT INTO purchase_bill_lines (bill_id, line_no, description, quantity, unit_cost, expense_account_id, tax_code, tax_rate)
	      VALUES ($1,1,'Materials',2,20,(SELECT id FROM accounts WHERE code='6000'),'T10',10)`, billID)

	// Payments and an inventory issue (3 units @ cost 7 = 21).
	custPay := queryID(`INSERT INTO customer_payments (customer_id, payment_date, currency_code, amount, deposit_account_id)
	      VALUES ($1,'2026-06-16','USD',55,(SELECT id FROM accounts WHERE code='1000')) RETURNING id`, custID)
	supPay := queryID(`INSERT INTO supplier_payments (supplier_id, payment_date, currency_code, amount, payment_account_id)
	      VALUES ($1,'2026-06-16','USD',44,(SELECT id FROM accounts WHERE code='1000')) RETURNING id`, supID)
	movID := queryID(`INSERT INTO stock_movements (product_id, warehouse_id, movement_type, movement_date, quantity, unit_cost)
	      VALUES ($1,$2,'issue','2026-06-16',-3,7) RETURNING id`, invProd, whID)

	// Post everything.
	mustPost := func(name string, fn func() (int, error)) {
		t.Helper()
		if _, err := fn(); err != nil {
			t.Fatalf("post %s: %v", name, err)
		}
	}
	mustPost("sales invoice", func() (int, error) { return posting.PostSalesInvoice(ctx, tx, invID) })
	mustPost("purchase bill", func() (int, error) { return posting.PostPurchaseBill(ctx, tx, billID) })
	mustPost("customer payment", func() (int, error) { return posting.PostCustomerPayment(ctx, tx, custPay) })
	mustPost("supplier payment", func() (int, error) { return posting.PostSupplierPayment(ctx, tx, supPay) })
	mustPost("inventory issue", func() (int, error) { return posting.PostInventoryIssue(ctx, tx, movID) })

	// Force the deferred balance constraints to run now; this fails if any
	// generated journal entry is unbalanced.
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatalf("a generated journal entry is unbalanced: %v", err)
	}

	// The entire ledger must net to zero.
	var sumBalance string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(balance),0)::text FROM trial_balance`).Scan(&sumBalance); err != nil {
		t.Fatalf("sum trial balance: %v", err)
	}
	if sumBalance != "0.0000" {
		t.Fatalf("trial balance does not net to zero: %s", sumBalance)
	}

	// Per-account balances (debit-positive).
	want := map[string]string{
		"1000": "11.0000",  // cash:      +55 received, -44 paid
		"1100": "0.0000",   // A/R:        55 invoiced, 55 received
		"1200": "-21.0000", // inventory: -21 issued
		"2000": "0.0000",   // A/P:        44 billed,   44 paid
		"2100": "-1.0000",  // tax:       -5 output,    +4 input
		"4000": "-50.0000", // revenue
		"5000": "21.0000",  // COGS
		"6000": "40.0000",  // expense
	}
	for code, expected := range want {
		var bal string
		if err := tx.QueryRow(ctx, `SELECT balance::text FROM trial_balance WHERE code = $1`, code).Scan(&bal); err != nil {
			t.Fatalf("balance %s: %v", code, err)
		}
		if bal != expected {
			t.Errorf("account %s balance = %s, want %s", code, bal, expected)
		}
	}

	// Posted documents now carry their journal-entry link.
	var jeID *int
	if err := tx.QueryRow(ctx, `SELECT journal_entry_id FROM sales_invoices WHERE id = $1`, invID).Scan(&jeID); err != nil {
		t.Fatalf("read invoice journal_entry_id: %v", err)
	}
	if jeID == nil {
		t.Error("sales invoice was not linked to a journal entry")
	}
}

// TestPeriodAutoCreate covers the month-rollover behaviour of posting: a date
// inside an open fiscal year but not covered by any period gets its calendar
// month auto-created as an open period (clipped to the fiscal year's bounds),
// while dates in a closed period, a closed fiscal year, or outside every
// fiscal year still refuse to post.
func TestPeriodAutoCreate(t *testing.T) {
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

	// FY2026 has only June; FY2025 starts mid-month to exercise clipping.
	exec(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY2026','2026-01-01','2026-12-31')`)
	exec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
	      SELECT id,'2026-06','2026-06-01','2026-06-30' FROM fiscal_years WHERE name='FY2026'`)
	exec(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY2025','2025-01-15','2025-12-31')`)

	custID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id, (SELECT id FROM accounts WHERE code='1100') FROM o RETURNING id`)
	newPayment := func(date string) int {
		return queryID(`INSERT INTO customer_payments (customer_id, payment_date, currency_code, amount, deposit_account_id)
		      VALUES ($1,$2::date,'USD',10,(SELECT id FROM accounts WHERE code='1000')) RETURNING id`, custID, date)
	}
	assertPeriod := func(name, wantStart, wantEnd string) {
		t.Helper()
		var start, end, status string
		if err := tx.QueryRow(ctx,
			`SELECT start_date::text, end_date::text, status FROM accounting_periods WHERE name = $1`, name).
			Scan(&start, &end, &status); err != nil {
			t.Fatalf("period %s was not created: %v", name, err)
		}
		if start != wantStart || end != wantEnd || status != "open" {
			t.Errorf("period %s = %s..%s %s, want %s..%s open", name, start, end, status, wantStart, wantEnd)
		}
	}

	// A date past the last existing period auto-creates its calendar month.
	entryID, err := posting.PostCustomerPayment(ctx, tx, newPayment("2026-07-15"))
	if err != nil {
		t.Fatalf("post payment after month rollover: %v", err)
	}
	assertPeriod("2026-07", "2026-07-01", "2026-07-31")
	var entryPeriod string
	if err := tx.QueryRow(ctx,
		`SELECT p.name FROM journal_entries je JOIN accounting_periods p ON p.id = je.period_id
		 WHERE je.id = $1`, entryID).Scan(&entryPeriod); err != nil {
		t.Fatalf("read entry period: %v", err)
	}
	if entryPeriod != "2026-07" {
		t.Errorf("entry posted into period %s, want 2026-07", entryPeriod)
	}

	// The auto-created month is clipped to the fiscal year's bounds.
	if _, err := posting.PostCustomerPayment(ctx, tx, newPayment("2025-01-20")); err != nil {
		t.Fatalf("post payment in clipped month: %v", err)
	}
	assertPeriod("2025-01", "2025-01-15", "2025-01-31")

	// A date outside every fiscal year still refuses to post.
	if _, err := posting.PostCustomerPayment(ctx, tx, newPayment("2028-03-01")); !errors.Is(err, posting.ErrNoOpenPeriod) {
		t.Errorf("post outside any fiscal year: err = %v, want ErrNoOpenPeriod", err)
	}

	// A closed period covering the date must not get a sibling auto-created.
	exec(`UPDATE accounting_periods SET status='closed' WHERE name='2026-06'`)
	if _, err := posting.PostCustomerPayment(ctx, tx, newPayment("2026-06-20")); !errors.Is(err, posting.ErrNoOpenPeriod) {
		t.Errorf("post into closed period: err = %v, want ErrNoOpenPeriod", err)
	}

	// A closed fiscal year must not grow new periods.
	exec(`UPDATE fiscal_years SET status='closed' WHERE name='FY2025'`)
	if _, err := posting.PostCustomerPayment(ctx, tx, newPayment("2025-05-10")); !errors.Is(err, posting.ErrNoOpenPeriod) {
		t.Errorf("post into closed fiscal year: err = %v, want ErrNoOpenPeriod", err)
	}
}

// TestInventoryReceiptClearsAgainstBill posts an inventory receipt (Dr inventory
// / Cr GRNI) and the matching purchase bill (Dr GRNI / Cr A/P), then checks the
// clearing account nets to zero and inventory/A/P land correctly.
func TestInventoryReceiptClearsAgainstBill(t *testing.T) {
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
	supID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Beta') RETURNING id)
	      INSERT INTO suppliers (organization_id, ap_account_id)
	      SELECT o.id,(SELECT id FROM accounts WHERE code='2000') FROM o RETURNING id`)
	invProd := queryID(`INSERT INTO products (sku, name, track_inventory, inventory_account_id, cogs_account_id)
	      VALUES ('P-INV','Widget',true,(SELECT id FROM accounts WHERE code='1200'),(SELECT id FROM accounts WHERE code='5000')) RETURNING id`)
	whID := queryID(`INSERT INTO warehouses (code, name) VALUES ('MAIN','Main') RETURNING id`)

	// Receive 10 units @ 7 = 70: Dr inventory 70 / Cr GRNI 70.
	movID := queryID(`INSERT INTO stock_movements (product_id, warehouse_id, movement_type, movement_date, quantity, unit_cost)
	      VALUES ($1,$2,'receipt','2026-06-16',10,7) RETURNING id`, invProd, whID)
	if _, err := posting.PostInventoryReceipt(ctx, tx, movID, grni); err != nil {
		t.Fatalf("post receipt: %v", err)
	}

	// The matching bill clears GRNI: a line for the same goods booked to GRNI
	// gives Dr GRNI 70 / Cr A/P 70.
	billID := queryID(`INSERT INTO purchase_bills (bill_number, supplier_id, bill_date, currency_code)
	      VALUES ('BILL-1',$1,'2026-06-15','USD') RETURNING id`, supID)
	exec(`INSERT INTO purchase_bill_lines (bill_id, line_no, description, quantity, unit_cost, expense_account_id)
	      VALUES ($1,1,'Widget',10,7,$2)`, billID, grni)
	if _, err := posting.PostPurchaseBill(ctx, tx, billID); err != nil {
		t.Fatalf("post bill: %v", err)
	}

	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatalf("a generated journal entry is unbalanced: %v", err)
	}

	want := map[string]string{
		"1200": "70.0000",  // inventory on the books
		"2150": "0.0000",   // GRNI cleared (received then invoiced)
		"2000": "-70.0000", // A/P owed to the supplier
	}
	for code, expected := range want {
		var bal string
		if err := tx.QueryRow(ctx, `SELECT balance::text FROM trial_balance WHERE code = $1`, code).Scan(&bal); err != nil {
			t.Fatalf("balance %s: %v", code, err)
		}
		if bal != expected {
			t.Errorf("account %s balance = %s, want %s", code, bal, expected)
		}
	}
}

// TestNegativeNetAccountPostsOnOppositeSide posts documents whose lines net
// negative on one account (a discount or rebate line on its own account): that
// account is posted on the opposite side, and the control line carries the
// document total, in base as well as transaction currency.
func TestNegativeNetAccountPostsOnOppositeSide(t *testing.T) {
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

	exec(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY2026','2026-01-01','2026-12-31')`)
	exec(`INSERT INTO exchange_rates (currency_code, rate_date, rate) VALUES ('EUR','2026-01-01',1.123456)`)
	exec(`INSERT INTO accounts (code, name, account_type, is_postable) VALUES
	      ('4100','Discounts','revenue',true), ('6100','Rebates','expense',true)`)
	custID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id, (SELECT id FROM accounts WHERE code='1100') FROM o RETURNING id`)
	supID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Beta') RETURNING id)
	      INSERT INTO suppliers (organization_id, ap_account_id)
	      SELECT o.id, (SELECT id FROM accounts WHERE code='2000') FROM o RETURNING id`)

	type line struct{ code, debit, credit, baseDebit, baseCredit string }
	check := func(name string, je int, want []line) {
		t.Helper()
		rows, err := tx.Query(ctx,
			`SELECT a.code, jl.debit::text, jl.credit::text, jl.base_debit::text, jl.base_credit::text
			 FROM journal_lines jl JOIN accounts a ON a.id = jl.account_id
			 WHERE jl.journal_entry_id = $1 ORDER BY a.code`, je)
		if err != nil {
			t.Fatalf("%s: lines: %v", name, err)
		}
		defer rows.Close()
		var got []line
		for rows.Next() {
			var l line
			if err := rows.Scan(&l.code, &l.debit, &l.credit, &l.baseDebit, &l.baseCredit); err != nil {
				t.Fatalf("%s: scan: %v", name, err)
			}
			got = append(got, l)
		}
		if len(got) != len(want) {
			t.Fatalf("%s: lines = %+v, want %+v", name, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: line %d = %+v, want %+v", name, i, got[i], want[i])
			}
		}
	}

	// EUR invoice: 100 goods (4000) less a 10 discount (4100) = 90 at 1.123456.
	// Revenue base 112.3456, discount base 11.2346, A/R base their net 101.1110.
	invID := queryID(`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, currency_code)
	      VALUES ('INV-NEG',$1,'2026-03-01','EUR') RETURNING id`, custID)
	exec(`INSERT INTO sales_invoice_lines (invoice_id, line_no, description, unit_price, revenue_account_id) VALUES
	      ($1,1,'Goods',100,(SELECT id FROM accounts WHERE code='4000')),
	      ($1,2,'Discount',-10,(SELECT id FROM accounts WHERE code='4100'))`, invID)
	je, err := posting.PostSalesInvoice(ctx, tx, invID)
	if err != nil {
		t.Fatalf("post invoice: %v", err)
	}
	check("invoice", je, []line{
		{"1100", "90.0000", "0.0000", "101.1110", "0.0000"},
		{"4000", "0.0000", "100.0000", "0.0000", "112.3456"},
		{"4100", "10.0000", "0.0000", "11.2346", "0.0000"},
	})

	// Bill: 50 materials (6000) less a 5 rebate (6100) = 45.
	billID := queryID(`INSERT INTO purchase_bills (bill_number, supplier_id, bill_date, currency_code)
	      VALUES ('BILL-NEG',$1,'2026-03-01','USD') RETURNING id`, supID)
	exec(`INSERT INTO purchase_bill_lines (bill_id, line_no, description, unit_cost, expense_account_id) VALUES
	      ($1,1,'Materials',50,(SELECT id FROM accounts WHERE code='6000')),
	      ($1,2,'Rebate',-5,(SELECT id FROM accounts WHERE code='6100'))`, billID)
	if je, err = posting.PostPurchaseBill(ctx, tx, billID); err != nil {
		t.Fatalf("post bill: %v", err)
	}
	check("bill", je, []line{
		{"2000", "0.0000", "45.0000", "0.0000", "45.0000"},
		{"6000", "50.0000", "0.0000", "50.0000", "0.0000"},
		{"6100", "0.0000", "5.0000", "0.0000", "5.0000"},
	})

	// Sales credit note: credit 30 revenue (4000), claw back 4 discount (4100).
	cnID := queryID(`INSERT INTO sales_credit_notes (credit_note_number, customer_id, credit_note_date, currency_code)
	      VALUES ('CN-NEG',$1,'2026-03-02','USD') RETURNING id`, custID)
	exec(`INSERT INTO sales_credit_note_lines (credit_note_id, line_no, description, unit_price, revenue_account_id) VALUES
	      ($1,1,'Return',30,(SELECT id FROM accounts WHERE code='4000')),
	      ($1,2,'Discount reversed',-4,(SELECT id FROM accounts WHERE code='4100'))`, cnID)
	if je, err = posting.PostSalesCreditNote(ctx, tx, cnID); err != nil {
		t.Fatalf("post sales credit note: %v", err)
	}
	check("sales credit note", je, []line{
		{"1100", "0.0000", "26.0000", "0.0000", "26.0000"},
		{"4000", "30.0000", "0.0000", "30.0000", "0.0000"},
		{"4100", "0.0000", "4.0000", "0.0000", "4.0000"},
	})

	// Purchase credit note: 20 materials credited (6000), 3 rebate reversed (6100).
	pcnID := queryID(`INSERT INTO purchase_credit_notes (credit_note_number, supplier_id, credit_note_date, currency_code)
	      VALUES ('SCN-NEG',$1,'2026-03-02','USD') RETURNING id`, supID)
	exec(`INSERT INTO purchase_credit_note_lines (credit_note_id, line_no, description, unit_cost, expense_account_id) VALUES
	      ($1,1,'Returned',20,(SELECT id FROM accounts WHERE code='6000')),
	      ($1,2,'Rebate reversed',-3,(SELECT id FROM accounts WHERE code='6100'))`, pcnID)
	if je, err = posting.PostPurchaseCreditNote(ctx, tx, pcnID); err != nil {
		t.Fatalf("post purchase credit note: %v", err)
	}
	check("purchase credit note", je, []line{
		{"2000", "17.0000", "0.0000", "17.0000", "0.0000"},
		{"6000", "0.0000", "20.0000", "0.0000", "20.0000"},
		{"6100", "3.0000", "0.0000", "3.0000", "0.0000"},
	})

	// Committing runs the deferred balance checks.
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}
