package posting_test

import (
	"context"
	"errors"
	"testing"

	"tadmor/internal/dbtest"
	"tadmor/internal/posting"
	"tadmor/internal/reporting"
)

// TestYearEndClose walks the whole close/reopen lifecycle: a year with revenue
// and expenses is closed (sweeping net income into retained earnings, locking
// the periods, rolling the calendar forward), reports stay sensible, and the
// close can be reversed and redone.
func TestYearEndClose(t *testing.T) {
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

	// Two consecutive years; only FY2026 has activity (in its June period).
	fy2025 := queryID(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY2025','2025-01-01','2025-12-31') RETURNING id`)
	fy2026 := queryID(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY2026','2026-01-01','2026-12-31') RETURNING id`)
	exec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
	      VALUES ($1,'2026-06','2026-06-01','2026-06-30')`, fy2026)

	reAccount := queryID(`SELECT id FROM accounts WHERE code = '3000'`)
	revAccount := queryID(`SELECT id FROM accounts WHERE code = '4000'`)

	// Revenue 50 (invoice) and expense 40 (bill): net income 10.
	custID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id, (SELECT id FROM accounts WHERE code='1100') FROM o RETURNING id`)
	supID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Beta') RETURNING id)
	      INSERT INTO suppliers (organization_id, ap_account_id)
	      SELECT o.id, (SELECT id FROM accounts WHERE code='2000') FROM o RETURNING id`)
	invID := queryID(`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, currency_code)
	      VALUES ('INV-1',$1,'2026-06-15','USD') RETURNING id`, custID)
	exec(`INSERT INTO sales_invoice_lines (invoice_id, line_no, description, quantity, unit_price, revenue_account_id)
	      VALUES ($1,1,'Service',10,5,(SELECT id FROM accounts WHERE code='4000'))`, invID)
	billID := queryID(`INSERT INTO purchase_bills (bill_number, supplier_id, bill_date, currency_code)
	      VALUES ('BILL-1',$1,'2026-06-15','USD') RETURNING id`, supID)
	exec(`INSERT INTO purchase_bill_lines (bill_id, line_no, description, quantity, unit_cost, expense_account_id)
	      VALUES ($1,1,'Materials',2,20,(SELECT id FROM accounts WHERE code='6000'))`, billID)
	if _, err := posting.PostSalesInvoice(ctx, tx, invID); err != nil {
		t.Fatalf("post invoice: %v", err)
	}
	if _, err := posting.PostPurchaseBill(ctx, tx, billID); err != nil {
		t.Fatalf("post bill: %v", err)
	}

	balance := func(code string) string {
		t.Helper()
		var bal string
		if err := tx.QueryRow(ctx, `SELECT balance::text FROM trial_balance WHERE code = $1`, code).Scan(&bal); err != nil {
			t.Fatalf("balance %s: %v", code, err)
		}
		return bal
	}
	assertBalances := func(label string, want map[string]string) {
		t.Helper()
		for code, expected := range want {
			if got := balance(code); got != expected {
				t.Errorf("%s: account %s balance = %s, want %s", label, code, got, expected)
			}
		}
	}

	// Years must close oldest-first.
	if _, err := posting.CloseFiscalYear(ctx, tx, fy2026, reAccount); !errors.Is(err, posting.ErrPriorYearOpen) {
		t.Fatalf("close FY2026 before FY2025 = %v, want ErrPriorYearOpen", err)
	}
	// The sweep target must be a postable, active equity account.
	if _, err := posting.CloseFiscalYear(ctx, tx, fy2025, revAccount); !errors.Is(err, posting.ErrMissingAccount) {
		t.Fatalf("close into a revenue account = %v, want ErrMissingAccount", err)
	}

	// FY2025 has nothing to sweep: no closing entry, and no new year either
	// since FY2026 already covers the following day.
	res, err := posting.CloseFiscalYear(ctx, tx, fy2025, reAccount)
	if err != nil {
		t.Fatalf("close FY2025: %v", err)
	}
	if res.ClosingEntryID != 0 || res.NextFiscalYearID != 0 {
		t.Errorf("close FY2025 = %+v, want no closing entry and no new year", res)
	}

	// Close FY2026 for real.
	res, err = posting.CloseFiscalYear(ctx, tx, fy2026, reAccount)
	if err != nil {
		t.Fatalf("close FY2026: %v", err)
	}
	if res.ClosingEntryID == 0 {
		t.Fatal("close FY2026 posted no closing entry")
	}
	// Force the deferred balance check now, then defer it again so later
	// posting in this transaction can build entries header-first.
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE; SET CONSTRAINTS ALL DEFERRED`); err != nil {
		t.Fatalf("closing entry is unbalanced: %v", err)
	}

	// The closing entry: Dr revenue 50, Cr expense 40, Cr retained earnings 10.
	rows, err := tx.Query(ctx,
		`SELECT a.code, jl.debit::text, jl.credit::text
		 FROM journal_lines jl JOIN accounts a ON a.id = jl.account_id
		 WHERE jl.journal_entry_id = $1 ORDER BY jl.line_no`, res.ClosingEntryID)
	if err != nil {
		t.Fatalf("read closing lines: %v", err)
	}
	type line struct{ code, debit, credit string }
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.code, &l.debit, &l.credit); err != nil {
			t.Fatalf("scan closing line: %v", err)
		}
		lines = append(lines, l)
	}
	rows.Close()
	wantLines := []line{
		{"4000", "50.0000", "0.0000"},
		{"6000", "0.0000", "40.0000"},
		{"3000", "0.0000", "10.0000"},
	}
	if len(lines) != len(wantLines) {
		t.Fatalf("closing entry has %d lines, want %d: %+v", len(lines), len(wantLines), lines)
	}
	for i, want := range wantLines {
		if lines[i] != want {
			t.Errorf("closing line %d = %+v, want %+v", i+1, lines[i], want)
		}
	}

	// Post-close: P&L accounts are zero, retained earnings holds the profit.
	assertBalances("post-close", map[string]string{
		"4000": "0.0000", "6000": "0.0000", "3000": "-10.0000",
	})

	// Everything about the year is locked, and FY2027 was rolled forward.
	var yearStatus string
	var closingEntry *int
	if err := tx.QueryRow(ctx,
		`SELECT status, closing_entry_id FROM fiscal_years WHERE id = $1`, fy2026).
		Scan(&yearStatus, &closingEntry); err != nil {
		t.Fatalf("read FY2026: %v", err)
	}
	if yearStatus != "closed" || closingEntry == nil || *closingEntry != res.ClosingEntryID {
		t.Errorf("FY2026 = %s / closing entry %v, want closed / %d", yearStatus, closingEntry, res.ClosingEntryID)
	}
	var openPeriods int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM accounting_periods WHERE fiscal_year_id = $1 AND status = 'open'`, fy2026).
		Scan(&openPeriods); err != nil {
		t.Fatalf("count open periods: %v", err)
	}
	if openPeriods != 0 {
		t.Errorf("FY2026 still has %d open periods after close", openPeriods)
	}
	if res.NextFiscalYearID == 0 {
		t.Fatal("close FY2026 did not create FY2027")
	}
	var nextName, nextStart, nextEnd string
	if err := tx.QueryRow(ctx,
		`SELECT name, start_date::text, end_date::text FROM fiscal_years WHERE id = $1`, res.NextFiscalYearID).
		Scan(&nextName, &nextStart, &nextEnd); err != nil {
		t.Fatalf("read next fiscal year: %v", err)
	}
	if nextName != "FY2027" || nextStart != "2027-01-01" || nextEnd != "2027-12-31" {
		t.Errorf("next fiscal year = %s %s..%s, want FY2027 2027-01-01..2027-12-31", nextName, nextStart, nextEnd)
	}

	// The income statement must not be zeroed by its own year's close.
	from, to := "2026-01-01", "2026-12-31"
	pl, err := reporting.ProfitAndLoss(ctx, tx, &from, &to)
	if err != nil {
		t.Fatalf("profit and loss: %v", err)
	}
	plAmounts := map[string]string{}
	for _, r := range pl {
		plAmounts[r.Code] = r.Amount
	}
	if plAmounts["4000"] != "50.0000" || plAmounts["6000"] != "40.0000" {
		t.Errorf("post-close P&L = %v, want revenue 50.0000 and expense 40.0000", plAmounts)
	}

	// Closed is closed: no second close, and no reopening an older year while
	// a newer one is closed.
	if _, err := posting.CloseFiscalYear(ctx, tx, fy2026, reAccount); !errors.Is(err, posting.ErrYearNotOpen) {
		t.Fatalf("second close = %v, want ErrYearNotOpen", err)
	}
	if _, err := posting.ReopenFiscalYear(ctx, tx, fy2025); !errors.Is(err, posting.ErrLaterYearClosed) {
		t.Fatalf("reopen FY2025 under closed FY2026 = %v, want ErrLaterYearClosed", err)
	}

	// Reopen FY2026: the closing entry is reversed and the balances return.
	reversalID, err := posting.ReopenFiscalYear(ctx, tx, fy2026)
	if err != nil {
		t.Fatalf("reopen FY2026: %v", err)
	}
	if reversalID == 0 {
		t.Fatal("reopen FY2026 posted no reversal")
	}
	assertBalances("post-reopen", map[string]string{
		"4000": "-50.0000", "6000": "40.0000", "3000": "0.0000",
	})
	var isClosing bool
	if err := tx.QueryRow(ctx,
		`SELECT is_closing FROM journal_entries WHERE id = $1`, reversalID).Scan(&isClosing); err != nil {
		t.Fatalf("read reversal: %v", err)
	}
	if !isClosing {
		t.Error("the closing entry's reversal is not flagged is_closing")
	}
	if err := tx.QueryRow(ctx,
		`SELECT status, closing_entry_id FROM fiscal_years WHERE id = $1`, fy2026).
		Scan(&yearStatus, &closingEntry); err != nil {
		t.Fatalf("re-read FY2026: %v", err)
	}
	if yearStatus != "open" || closingEntry != nil {
		t.Errorf("reopened FY2026 = %s / closing entry %v, want open / nil", yearStatus, closingEntry)
	}
	// Only the period that held the closing entry reopens; June stays closed.
	var periodStatus string
	if err := tx.QueryRow(ctx,
		`SELECT status FROM accounting_periods WHERE name = '2026-12'`).Scan(&periodStatus); err != nil {
		t.Fatalf("read auto-created year-end period: %v", err)
	}
	if periodStatus != "open" {
		t.Errorf("year-end period after reopen = %s, want open", periodStatus)
	}
	if err := tx.QueryRow(ctx,
		`SELECT status FROM accounting_periods WHERE name = '2026-06'`).Scan(&periodStatus); err != nil {
		t.Fatalf("read June period: %v", err)
	}
	if periodStatus != "closed" {
		t.Errorf("June period after reopen = %s, want closed", periodStatus)
	}

	// Closing again sweeps the same balances afresh (the old entry and its
	// reversal net out) and lands on the same retained-earnings figure.
	res, err = posting.CloseFiscalYear(ctx, tx, fy2026, reAccount)
	if err != nil {
		t.Fatalf("re-close FY2026: %v", err)
	}
	if res.ClosingEntryID == 0 {
		t.Fatal("re-close posted no closing entry")
	}
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatalf("re-close entry is unbalanced: %v", err)
	}
	assertBalances("post-re-close", map[string]string{
		"4000": "0.0000", "6000": "0.0000", "3000": "-10.0000",
	})
}

// TestClosedYearPeriodsStayShut covers the database invariant added with the
// year-end close: a period in a closed fiscal year can be neither reopened
// nor created open, so nothing can post into a closed year.
func TestClosedYearPeriodsStayShut(t *testing.T) {
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
	fyID := queryID(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY2025','2025-01-01','2025-12-31') RETURNING id`)
	periodID := queryID(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date, status)
	      VALUES ($1,'2025-06','2025-06-01','2025-06-30','closed') RETURNING id`, fyID)
	exec(`UPDATE fiscal_years SET status = 'closed' WHERE id = $1`, fyID)

	if _, err := tx.Exec(ctx,
		`UPDATE accounting_periods SET status = 'open' WHERE id = $1`, periodID); err == nil {
		t.Error("reopening a period in a closed fiscal year was allowed")
	}
}
