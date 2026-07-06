package posting_test

import (
	"context"
	"errors"
	"testing"

	"tadmor/internal/dbtest"
	"tadmor/internal/posting"
)

// TestSalesCreditNoteLifecycle posts a sales credit note, checks the mirrored
// GL entry, unposts and re-posts it, applies it across open invoices oldest
// first, and verifies the interplay with payment applications and the unpost
// guard.
func TestSalesCreditNoteLifecycle(t *testing.T) {
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
	exec(`INSERT INTO tax_codes (code, name, rate, tax_account_id)
	      VALUES ('T10','10% tax',10,(SELECT id FROM accounts WHERE code='2100'))`)
	custID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id,(SELECT id FROM accounts WHERE code='1100') FROM o RETURNING id`)

	// Two posted invoices (no tax): older 30, newer 40 → A/R 70.
	older := newPostedInvoice(ctx, t, tx, custID, "INV-A", "2026-06-10", 30)
	newer := newPostedInvoice(ctx, t, tx, custID, "INV-B", "2026-06-20", 40)

	// Credit note: 10 x 5 @ 10% = 50 net + 5 tax = 55 gross.
	noteID := queryID(`INSERT INTO sales_credit_notes (credit_note_number, customer_id, credit_note_date, currency_code)
	      VALUES ('CN-1',$1,'2026-06-21','USD') RETURNING id`, custID)
	exec(`INSERT INTO sales_credit_note_lines (credit_note_id, line_no, description, quantity, unit_price, revenue_account_id, tax_code, tax_rate)
	      VALUES ($1,1,'Returned service',10,5,(SELECT id FROM accounts WHERE code='4000'),'T10',10)`, noteID)
	if _, err := posting.PostSalesCreditNote(ctx, tx, noteID); err != nil {
		t.Fatalf("post credit note: %v", err)
	}

	// Unpost while unapplied returns it to draft; re-post for the rest of the
	// test.
	if _, err := posting.UnpostSalesCreditNote(ctx, tx, noteID); err != nil {
		t.Fatalf("unpost credit note: %v", err)
	}
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM sales_credit_notes WHERE id = $1`, noteID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "draft" {
		t.Fatalf("status after unpost = %s, want draft", status)
	}
	if _, err := posting.PostSalesCreditNote(ctx, tx, noteID); err != nil {
		t.Fatalf("re-post credit note: %v", err)
	}

	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatalf("a generated journal entry is unbalanced: %v", err)
	}
	// Re-defer so the postings below can again build entries line by line.
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL DEFERRED`); err != nil {
		t.Fatalf("re-defer constraints: %v", err)
	}

	// The credit note mirrors the invoice posting (net of the reversed pair):
	// A/R 70 - 55, revenue -70 + 50, tax +5.
	want := map[string]string{
		"1100": "15.0000",
		"4000": "-20.0000",
		"2100": "5.0000",
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

	// Auto-apply clears the older invoice (30) and puts 25 on the newer.
	apps, err := posting.AutoApplySalesCreditNote(ctx, tx, noteID)
	if err != nil {
		t.Fatalf("auto-apply: %v", err)
	}
	wantApps := []posting.Application{
		{DocumentID: older, Amount: "30.0000"},
		{DocumentID: newer, Amount: "25.0000"},
	}
	if len(apps) != len(wantApps) {
		t.Fatalf("got %d applications, want %d: %+v", len(apps), len(wantApps), apps)
	}
	for i, w := range wantApps {
		if apps[i] != w {
			t.Errorf("application %d = %+v, want %+v", i, apps[i], w)
		}
	}

	// Invoice balances count the credit; the note itself is fully applied.
	assertInvoiceBalance := func(invoiceID int, balance, status string) {
		t.Helper()
		var b, s string
		if err := tx.QueryRow(ctx,
			`SELECT balance::text, payment_status FROM sales_invoice_balances WHERE invoice_id = $1`,
			invoiceID).Scan(&b, &s); err != nil {
			t.Fatalf("read invoice balance %d: %v", invoiceID, err)
		}
		if b != balance || s != status {
			t.Errorf("invoice %d: balance=%s status=%s, want balance=%s status=%s", invoiceID, b, s, balance, status)
		}
	}
	assertInvoiceBalance(older, "0.0000", "paid")
	assertInvoiceBalance(newer, "15.0000", "partial")

	var noteBalance, noteStatus string
	if err := tx.QueryRow(ctx,
		`SELECT balance::text, application_status FROM sales_credit_note_balances WHERE credit_note_id = $1`,
		noteID).Scan(&noteBalance, &noteStatus); err != nil {
		t.Fatalf("read credit note balance: %v", err)
	}
	if noteBalance != "0.0000" || noteStatus != "applied" {
		t.Errorf("credit note: balance=%s status=%s, want 0.0000/applied", noteBalance, noteStatus)
	}

	// A payment's auto-apply sees only what the credit left open: a 20 payment
	// against the newer invoice's remaining 15 applies 15.
	payID := queryID(`INSERT INTO customer_payments (customer_id, payment_date, currency_code, amount, deposit_account_id)
	      VALUES ($1,'2026-06-25','USD',20,(SELECT id FROM accounts WHERE code='1000')) RETURNING id`, custID)
	if _, err := posting.PostCustomerPayment(ctx, tx, payID); err != nil {
		t.Fatalf("post payment: %v", err)
	}
	payApps, err := posting.AutoApplyCustomerPayment(ctx, tx, payID)
	if err != nil {
		t.Fatalf("auto-apply payment: %v", err)
	}
	if len(payApps) != 1 || payApps[0] != (posting.Application{DocumentID: newer, Amount: "15.0000"}) {
		t.Errorf("payment applications = %+v, want [{%d 15.0000}]", payApps, newer)
	}

	// An applied credit note cannot be unposted.
	if _, err := posting.UnpostSalesCreditNote(ctx, tx, noteID); !errors.Is(err, posting.ErrHasApplications) {
		t.Errorf("unpost applied credit note: err = %v, want ErrHasApplications", err)
	}

	// Everything posted after the mid-test check balances too.
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatalf("a generated journal entry is unbalanced: %v", err)
	}
}

// TestPurchaseCreditNoteLifecycle is the purchasing-side mirror: post a
// supplier credit note, check the GL, apply it across open bills oldest first,
// and verify payment auto-apply sees the reduced availability.
func TestPurchaseCreditNoteLifecycle(t *testing.T) {
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

	// Two posted bills (no tax): older 25, newer 25 → A/P 50.
	older := newPostedBill(ctx, t, tx, supID, "BILL-A", "2026-06-05", 25)
	newer := newPostedBill(ctx, t, tx, supID, "BILL-B", "2026-06-15", 25)

	// Supplier credit note for 30 against the expense account.
	noteID := queryID(`INSERT INTO purchase_credit_notes (credit_note_number, supplier_id, credit_note_date, currency_code)
	      VALUES ('SCN-1',$1,'2026-06-18','USD') RETURNING id`, supID)
	exec(`INSERT INTO purchase_credit_note_lines (credit_note_id, line_no, description, quantity, unit_cost, expense_account_id)
	      VALUES ($1,1,'Returned materials',1,30,(SELECT id FROM accounts WHERE code='6000'))`, noteID)
	if _, err := posting.PostPurchaseCreditNote(ctx, tx, noteID); err != nil {
		t.Fatalf("post credit note: %v", err)
	}

	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatalf("a generated journal entry is unbalanced: %v", err)
	}
	// Re-defer so the postings below can again build entries line by line.
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL DEFERRED`); err != nil {
		t.Fatalf("re-defer constraints: %v", err)
	}

	// Dr A/P 30 / Cr expense 30: A/P -50 + 30, expense +50 - 30.
	want := map[string]string{
		"2000": "-20.0000",
		"6000": "20.0000",
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

	// Auto-apply clears the older bill (25) and puts 5 on the newer.
	apps, err := posting.AutoApplyPurchaseCreditNote(ctx, tx, noteID)
	if err != nil {
		t.Fatalf("auto-apply: %v", err)
	}
	wantApps := []posting.Application{
		{DocumentID: older, Amount: "25.0000"},
		{DocumentID: newer, Amount: "5.0000"},
	}
	if len(apps) != len(wantApps) {
		t.Fatalf("got %d applications, want %d: %+v", len(apps), len(wantApps), apps)
	}
	for i, w := range wantApps {
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

	// A 30 payment sees only the remaining 20 across the supplier's bills.
	payID := queryID(`INSERT INTO supplier_payments (supplier_id, payment_date, currency_code, amount, payment_account_id)
	      VALUES ($1,'2026-06-20','USD',30,(SELECT id FROM accounts WHERE code='1000')) RETURNING id`, supID)
	if _, err := posting.PostSupplierPayment(ctx, tx, payID); err != nil {
		t.Fatalf("post payment: %v", err)
	}
	payApps, err := posting.AutoApplySupplierPayment(ctx, tx, payID)
	if err != nil {
		t.Fatalf("auto-apply payment: %v", err)
	}
	if len(payApps) != 1 || payApps[0] != (posting.Application{DocumentID: newer, Amount: "20.0000"}) {
		t.Errorf("payment applications = %+v, want [{%d 20.0000}]", payApps, newer)
	}

	// An applied credit note cannot be unposted.
	if _, err := posting.UnpostPurchaseCreditNote(ctx, tx, noteID); !errors.Is(err, posting.ErrHasApplications) {
		t.Errorf("unpost applied credit note: err = %v, want ErrHasApplications", err)
	}

	// Everything posted after the mid-test check balances too.
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatalf("a generated journal entry is unbalanced: %v", err)
	}
}
