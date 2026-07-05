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

	// A bill overdue by ~5 days lands in the 1-30 AP aging bucket.
	suppID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Globex') RETURNING id)
	      INSERT INTO suppliers (organization_id, ap_account_id)
	      SELECT o.id,(SELECT id FROM accounts WHERE code='2000') FROM o RETURNING id`)
	billID := queryID(`INSERT INTO purchase_bills (bill_number, supplier_id, bill_date, due_date, currency_code)
	      VALUES ('BILL-1',$1, current_date - 10, current_date - 5, 'USD') RETURNING id`, suppID)
	exec(`INSERT INTO purchase_bill_lines (bill_id, line_no, description, quantity, unit_cost, expense_account_id)
	      VALUES ($1,1,'Rent',1,40,(SELECT id FROM accounts WHERE code='6000'))`, billID)
	if _, err := posting.PostPurchaseBill(ctx, tx, billID); err != nil {
		t.Fatalf("post bill: %v", err)
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

	t.Run("profit and loss", func(t *testing.T) {
		strPtr := func(s string) *string { return &s }
		rows, err := reporting.ProfitAndLoss(ctx, tx, nil, nil)
		if err != nil {
			t.Fatalf("profit and loss: %v", err)
		}
		amounts := map[string]reporting.AccountActivityRow{}
		for _, r := range rows {
			amounts[r.Code] = r
		}
		// Natural sign: revenue credit-positive, expenses debit-positive.
		if r := amounts["4000"]; r.AccountType != "revenue" || r.Amount != "100.0000" {
			t.Errorf("revenue row = %+v, want revenue 100.0000", r)
		}
		if r := amounts["6000"]; r.AccountType != "expense" || r.Amount != "40.0000" {
			t.Errorf("expense row = %+v, want expense 40.0000", r)
		}
		// Accounts with no P&L activity are omitted entirely.
		if _, found := amounts["1000"]; found {
			t.Error("cash appeared on the P&L")
		}
		// A range before all activity yields no rows.
		if rows, err := reporting.ProfitAndLoss(ctx, tx, strPtr("1990-01-01"), strPtr("1990-12-31")); err != nil || len(rows) != 0 {
			t.Errorf("out-of-range P&L = %+v, %v, want empty", rows, err)
		}
		// A range covering the postings yields both rows again.
		if rows, err := reporting.ProfitAndLoss(ctx, tx, strPtr("1990-01-01"), nil); err != nil || len(rows) != 2 {
			t.Errorf("open-ended P&L rows = %d, %v, want 2", len(rows), err)
		}
	})

	t.Run("balance sheet", func(t *testing.T) {
		bs, err := reporting.BalanceSheetAsOf(ctx, tx, nil)
		if err != nil {
			t.Fatalf("balance sheet: %v", err)
		}
		amounts := map[string]reporting.AccountActivityRow{}
		for _, r := range bs.Rows {
			amounts[r.Code] = r
		}
		// A/R debit-positive, A/P credit-positive.
		if r := amounts["1100"]; r.AccountType != "asset" || r.Amount != "100.0000" {
			t.Errorf("A/R row = %+v, want asset 100.0000", r)
		}
		if r := amounts["2000"]; r.AccountType != "liability" || r.Amount != "40.0000" {
			t.Errorf("A/P row = %+v, want liability 40.0000", r)
		}
		// Revenue/expense accounts never appear as rows; their net is the
		// current-earnings figure that balances the sheet (100 - 40).
		if _, found := amounts["4000"]; found {
			t.Error("revenue appeared on the balance sheet")
		}
		if bs.CurrentEarnings != "60.0000" {
			t.Errorf("current earnings = %s, want 60.0000", bs.CurrentEarnings)
		}
		// As of a date before all activity: no rows, zero earnings.
		early := "1990-01-01"
		if bs, err := reporting.BalanceSheetAsOf(ctx, tx, &early); err != nil ||
			len(bs.Rows) != 0 || bs.CurrentEarnings != "0.0000" {
			t.Errorf("early balance sheet = %+v, %v, want empty and 0.0000", bs, err)
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

	t.Run("invoice list", func(t *testing.T) {
		invs, err := reporting.SalesInvoiceBalances(ctx, tx)
		if err != nil {
			t.Fatalf("invoice list: %v", err)
		}
		if len(invs) != 1 {
			t.Fatalf("invoice rows = %d, want 1", len(invs))
		}
		inv := invs[0]
		if inv.ID != invID || inv.Number != "INV-1" || inv.Status != "posted" || inv.Total != "100.0000" {
			t.Errorf("invoice = %+v, want INV-1 posted total 100.0000", inv)
		}
	})

	t.Run("invoice lines", func(t *testing.T) {
		lines, err := reporting.SalesInvoiceLines(ctx, tx, invID)
		if err != nil {
			t.Fatalf("invoice lines: %v", err)
		}
		if len(lines) != 1 {
			t.Fatalf("line rows = %d, want 1", len(lines))
		}
		l := lines[0]
		if l.LineNo != 1 || l.Description != "Service" || l.Quantity != "100.0000" ||
			l.UnitPrice != "1.0000" || l.LineSubtotal != "100.0000" || l.TaxAmount != "0.0000" || l.LineTotal != "100.0000" {
			t.Errorf("line = %+v, want 100 x 1.0000 = 100.0000 untaxed", l)
		}
	})

	t.Run("lines of missing invoice", func(t *testing.T) {
		if _, err := reporting.SalesInvoiceLines(ctx, tx, 999999); !errors.Is(err, reporting.ErrNotFound) {
			t.Errorf("error = %v, want ErrNotFound", err)
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

	t.Run("bill list", func(t *testing.T) {
		bills, err := reporting.PurchaseBillBalances(ctx, tx)
		if err != nil {
			t.Fatalf("bill list: %v", err)
		}
		if len(bills) != 1 {
			t.Fatalf("bill rows = %d, want 1", len(bills))
		}
		b := bills[0]
		if b.ID != billID || b.Number != "BILL-1" || b.Status != "posted" || b.Total != "40.0000" {
			t.Errorf("bill = %+v, want BILL-1 posted total 40.0000", b)
		}
	})

	t.Run("bill lines", func(t *testing.T) {
		lines, err := reporting.PurchaseBillLines(ctx, tx, billID)
		if err != nil {
			t.Fatalf("bill lines: %v", err)
		}
		if len(lines) != 1 {
			t.Fatalf("line rows = %d, want 1", len(lines))
		}
		l := lines[0]
		if l.LineNo != 1 || l.Description != "Rent" || l.Quantity != "1.0000" ||
			l.UnitCost != "40.0000" || l.LineSubtotal != "40.0000" || l.TaxAmount != "0.0000" || l.LineTotal != "40.0000" {
			t.Errorf("line = %+v, want 1 x 40.0000 = 40.0000 untaxed", l)
		}
	})

	t.Run("lines of missing bill", func(t *testing.T) {
		if _, err := reporting.PurchaseBillLines(ctx, tx, 999999); !errors.Is(err, reporting.ErrNotFound) {
			t.Errorf("error = %v, want ErrNotFound", err)
		}
	})

	t.Run("AP aging", func(t *testing.T) {
		aging, err := reporting.APaging(ctx, tx)
		if err != nil {
			t.Fatalf("ap aging: %v", err)
		}
		if len(aging) != 1 {
			t.Fatalf("aging rows = %d, want 1", len(aging))
		}
		a := aging[0]
		if a.PartyID != suppID || a.TotalOutstanding != "40.0000" || a.Days1To30 != "40.0000" || a.NotYetDue != "0.0000" {
			t.Errorf("aging = %+v, want party %d, 40.0000 in days_1_30", a, suppID)
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

	t.Run("stock movement list", func(t *testing.T) {
		ms, err := reporting.StockMovements(ctx, tx)
		if err != nil {
			t.Fatalf("stock movements: %v", err)
		}
		if len(ms) != 1 {
			t.Fatalf("movement rows = %d, want 1", len(ms))
		}
		m := ms[0]
		if m.ProductID != invProd || m.WarehouseID != whID || m.Type != "receipt" ||
			m.Quantity != "10.0000" || m.UnitCost != "7.0000" || m.TotalCost != "70.0000" {
			t.Errorf("movement = %+v, want receipt 10 x 7.0000", m)
		}
		if m.JournalEntryID != nil {
			t.Errorf("journal_entry_id = %v, want nil (unposted)", m.JournalEntryID)
		}
	})

	t.Run("stock movement single", func(t *testing.T) {
		ms, err := reporting.StockMovements(ctx, tx)
		if err != nil || len(ms) == 0 {
			t.Fatalf("stock movements: %v", err)
		}
		m, err := reporting.StockMovementByID(ctx, tx, ms[0].ID)
		if err != nil {
			t.Fatalf("single movement: %v", err)
		}
		if m.ID != ms[0].ID || m.TotalCost != "70.0000" {
			t.Errorf("movement = %+v, want id %d total 70.0000", m, ms[0].ID)
		}
	})

	t.Run("missing stock movement", func(t *testing.T) {
		if _, err := reporting.StockMovementByID(ctx, tx, 999999); !errors.Is(err, reporting.ErrNotFound) {
			t.Errorf("error = %v, want ErrNotFound", err)
		}
	})
}

// TestPaymentQueries exercises the payment list/single/applications queries in
// their own reset so the GL totals asserted above stay undisturbed.
func TestPaymentQueries(t *testing.T) {
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

	exec(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY', current_date - 400, current_date + 400)`)
	exec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
	      SELECT id,'P', current_date - 400, current_date + 400 FROM fiscal_years WHERE name='FY'`)

	// A posted 100.0000 invoice and a posted 130.0000 receipt: auto-apply
	// covers the invoice in full and leaves 30.0000 unapplied.
	custID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id,(SELECT id FROM accounts WHERE code='1100') FROM o RETURNING id`)
	invID := queryID(`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, currency_code)
	      VALUES ('INV-1',$1, current_date - 10, 'USD') RETURNING id`, custID)
	exec(`INSERT INTO sales_invoice_lines (invoice_id, line_no, description, quantity, unit_price, revenue_account_id)
	      VALUES ($1,1,'Service',100,1,(SELECT id FROM accounts WHERE code='4000'))`, invID)
	if _, err := posting.PostSalesInvoice(ctx, tx, invID); err != nil {
		t.Fatalf("post invoice: %v", err)
	}
	cpID := queryID(`INSERT INTO customer_payments (customer_id, payment_date, currency_code, amount, method, deposit_account_id)
	      VALUES ($1, current_date - 5, 'USD', 130, 'transfer', (SELECT id FROM accounts WHERE code='1000')) RETURNING id`, custID)
	if _, err := posting.PostCustomerPayment(ctx, tx, cpID); err != nil {
		t.Fatalf("post payment: %v", err)
	}
	if _, err := posting.AutoApplyCustomerPayment(ctx, tx, cpID); err != nil {
		t.Fatalf("apply payment: %v", err)
	}

	// Supplier side: a posted 40.0000 bill and a fully-applied 15.0000 payment.
	suppID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Globex') RETURNING id)
	      INSERT INTO suppliers (organization_id, ap_account_id)
	      SELECT o.id,(SELECT id FROM accounts WHERE code='2000') FROM o RETURNING id`)
	billID := queryID(`INSERT INTO purchase_bills (bill_number, supplier_id, bill_date, currency_code)
	      VALUES ('BILL-1',$1, current_date - 10, 'USD') RETURNING id`, suppID)
	exec(`INSERT INTO purchase_bill_lines (bill_id, line_no, description, quantity, unit_cost, expense_account_id)
	      VALUES ($1,1,'Rent',1,40,(SELECT id FROM accounts WHERE code='6000'))`, billID)
	if _, err := posting.PostPurchaseBill(ctx, tx, billID); err != nil {
		t.Fatalf("post bill: %v", err)
	}
	spID := queryID(`INSERT INTO supplier_payments (supplier_id, payment_date, currency_code, amount, payment_account_id)
	      VALUES ($1, current_date - 3, 'USD', 15, (SELECT id FROM accounts WHERE code='1000')) RETURNING id`, suppID)
	if _, err := posting.PostSupplierPayment(ctx, tx, spID); err != nil {
		t.Fatalf("post supplier payment: %v", err)
	}
	if _, err := posting.AutoApplySupplierPayment(ctx, tx, spID); err != nil {
		t.Fatalf("apply supplier payment: %v", err)
	}

	t.Run("customer payment list", func(t *testing.T) {
		ps, err := reporting.CustomerPayments(ctx, tx)
		if err != nil {
			t.Fatalf("customer payments: %v", err)
		}
		if len(ps) != 1 {
			t.Fatalf("payment rows = %d, want 1", len(ps))
		}
		p := ps[0]
		if p.ID != cpID || p.PartyID != custID || p.Status != "posted" ||
			p.Amount != "130.0000" || p.AmountApplied != "100.0000" || p.Unapplied != "30.0000" {
			t.Errorf("payment = %+v, want posted 130.0000 with 100.0000 applied", p)
		}
	})

	t.Run("customer payment applications", func(t *testing.T) {
		apps, err := reporting.CustomerPaymentApplications(ctx, tx, cpID)
		if err != nil {
			t.Fatalf("applications: %v", err)
		}
		if len(apps) != 1 || apps[0].DocumentID != invID ||
			apps[0].DocumentNumber != "INV-1" || apps[0].AmountApplied != "100.0000" {
			t.Errorf("applications = %+v, want INV-1 100.0000", apps)
		}
	})

	t.Run("supplier payment single", func(t *testing.T) {
		p, err := reporting.SupplierPayment(ctx, tx, spID)
		if err != nil {
			t.Fatalf("supplier payment: %v", err)
		}
		if p.PartyID != suppID || p.Amount != "15.0000" || p.AmountApplied != "15.0000" || p.Unapplied != "0.0000" {
			t.Errorf("payment = %+v, want 15.0000 fully applied", p)
		}
	})

	t.Run("supplier payment applications", func(t *testing.T) {
		apps, err := reporting.SupplierPaymentApplications(ctx, tx, spID)
		if err != nil {
			t.Fatalf("applications: %v", err)
		}
		if len(apps) != 1 || apps[0].DocumentID != billID ||
			apps[0].DocumentNumber != "BILL-1" || apps[0].AmountApplied != "15.0000" {
			t.Errorf("applications = %+v, want BILL-1 15.0000", apps)
		}
	})

	t.Run("missing payment", func(t *testing.T) {
		if _, err := reporting.CustomerPayment(ctx, tx, 999999); !errors.Is(err, reporting.ErrNotFound) {
			t.Errorf("single: error = %v, want ErrNotFound", err)
		}
		if _, err := reporting.SupplierPaymentApplications(ctx, tx, 999999); !errors.Is(err, reporting.ErrNotFound) {
			t.Errorf("applications: error = %v, want ErrNotFound", err)
		}
	})
}
