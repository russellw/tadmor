package banking_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"tadmor/internal/banking"
	"tadmor/internal/dbtest"
	"tadmor/internal/posting"
	"tadmor/internal/reporting"
)

// expectDBError runs op inside a savepoint (a database-raised error aborts
// the surrounding transaction otherwise) and asserts the error mentions
// wantSubstr.
func expectDBError(ctx context.Context, t *testing.T, tx pgx.Tx, wantSubstr string, op func(sp pgx.Tx) error) {
	t.Helper()
	sp, err := tx.Begin(ctx)
	if err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	opErr := op(sp)
	_ = sp.Rollback(ctx)
	if opErr == nil || !strings.Contains(opErr.Error(), wantSubstr) {
		t.Fatalf("err = %v, want mention of %q", opErr, wantSubstr)
	}
}

// TestBankReconciliation walks the whole lifecycle: capture a statement by
// CSV import for a cash account, auto-match its lines against posted
// payments, reconcile, verify the reconciled statement (and its matched
// journal entries) are locked, then reopen and rework a match by hand.
func TestBankReconciliation(t *testing.T) {
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
	exec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
	      SELECT id,'2026-06','2026-06-01','2026-06-30' FROM fiscal_years WHERE name='FY2026'`)

	cashAccount := queryID(`SELECT id FROM accounts WHERE code = '1000'`) // seeded as is_cash
	arAccount := queryID(`SELECT id FROM accounts WHERE code = '1100'`)

	// Two posted customer payments into the cash account: 100.00 and 250.00.
	custID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id, $1 FROM o RETURNING id`, arAccount)
	pay1 := queryID(`INSERT INTO customer_payments (customer_id, payment_date, currency_code, amount, deposit_account_id)
	      VALUES ($1,'2026-06-10','USD',100,$2) RETURNING id`, custID, cashAccount)
	pay2 := queryID(`INSERT INTO customer_payments (customer_id, payment_date, currency_code, amount, deposit_account_id)
	      VALUES ($1,'2026-06-12','USD',250,$2) RETURNING id`, custID, cashAccount)
	if _, err := posting.PostCustomerPayment(ctx, tx, pay1); err != nil {
		t.Fatalf("post payment 1: %v", err)
	}
	if _, err := posting.PostCustomerPayment(ctx, tx, pay2); err != nil {
		t.Fatalf("post payment 2: %v", err)
	}

	// A statement may only be drawn on a cash account.
	expectDBError(ctx, t, tx, "cash account", func(sp pgx.Tx) error {
		_, err := banking.CreateStatement(ctx, sp, banking.StatementInput{
			AccountID: arAccount, StatementDate: "2026-06-30",
			OpeningBalance: "0", ClosingBalance: "0",
		})
		return err
	})

	stmtID, err := banking.CreateStatement(ctx, tx, banking.StatementInput{
		AccountID: cashAccount, StatementDate: "2026-06-30",
		OpeningBalance: "0", ClosingBalance: "350",
	})
	if err != nil {
		t.Fatalf("create statement: %v", err)
	}

	// CSV import: header row skipped, reference column optional.
	if _, err := banking.ImportCSV(ctx, tx, stmtID, "not,a,date\n"); err == nil || !errors.Is(err, banking.ErrBadCSV) {
		t.Fatalf("import header-only CSV: err = %v, want ErrBadCSV", err)
	}
	imported, err := banking.ImportCSV(ctx, tx, stmtID,
		"date,description,amount,reference\n"+
			"2026-06-10,Incoming transfer,100.00\n"+
			"2026-06-13,Wire ACME,250.00,REF-9\n")
	if err != nil {
		t.Fatalf("import CSV: %v", err)
	}
	if imported != 2 {
		t.Fatalf("imported = %d, want 2", imported)
	}

	// Reconciling with unmatched lines is refused.
	if err := banking.Reconcile(ctx, tx, stmtID); !errors.Is(err, banking.ErrUnmatchedLines) {
		t.Fatalf("reconcile unmatched = %v, want ErrUnmatchedLines", err)
	}

	// Both candidates are visible before matching.
	cands, err := reporting.BankMatchCandidates(ctx, tx, stmtID)
	if err != nil {
		t.Fatalf("candidates: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("candidates = %d, want 2", len(cands))
	}

	matched, err := banking.AutoMatch(ctx, tx, stmtID)
	if err != nil {
		t.Fatalf("auto-match: %v", err)
	}
	if matched != 2 {
		t.Fatalf("auto-matched = %d, want 2", matched)
	}
	if cands, err = reporting.BankMatchCandidates(ctx, tx, stmtID); err != nil || len(cands) != 0 {
		t.Fatalf("candidates after match = %d (err %v), want 0", len(cands), err)
	}

	// An unbalanced statement is refused even when fully matched.
	if err := banking.UpdateStatement(ctx, tx, stmtID, banking.StatementInput{
		AccountID: cashAccount, StatementDate: "2026-06-30",
		OpeningBalance: "0", ClosingBalance: "999",
	}); err != nil {
		t.Fatalf("update closing balance: %v", err)
	}
	if err := banking.Reconcile(ctx, tx, stmtID); !errors.Is(err, banking.ErrUnbalanced) {
		t.Fatalf("reconcile unbalanced = %v, want ErrUnbalanced", err)
	}
	if err := banking.UpdateStatement(ctx, tx, stmtID, banking.StatementInput{
		AccountID: cashAccount, StatementDate: "2026-06-30",
		OpeningBalance: "0", ClosingBalance: "350",
	}); err != nil {
		t.Fatalf("restore closing balance: %v", err)
	}
	if err := banking.Reconcile(ctx, tx, stmtID); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	st, err := reporting.BankStatementByID(ctx, tx, stmtID)
	if err != nil {
		t.Fatalf("read statement: %v", err)
	}
	if st.Status != "reconciled" || st.MatchedCount != 2 || st.Difference != "0.0000" {
		t.Fatalf("statement after reconcile = %+v, want reconciled, 2 matched, zero difference", st)
	}

	// Reconciled: header, lines, and the matched entries are all locked.
	if err := banking.DeleteStatement(ctx, tx, stmtID); !errors.Is(err, banking.ErrNotOpen) {
		t.Fatalf("delete reconciled = %v, want ErrNotOpen", err)
	}
	if _, err := banking.AddLine(ctx, tx, stmtID, banking.LineInput{
		TxnDate: "2026-06-20", Description: "Late fee", Amount: "-5",
	}); !errors.Is(err, banking.ErrNotOpen) {
		t.Fatalf("add line to reconciled = %v, want ErrNotOpen", err)
	}
	if _, err := posting.UnpostCustomerPayment(ctx, tx, pay1); !errors.Is(err, posting.ErrBankMatched) {
		t.Fatalf("unpost matched payment = %v, want ErrBankMatched", err)
	}

	// Reopen (correction path): unmatch one line, verify the mismatch guard,
	// rematch it by hand, and reconcile again.
	if err := banking.Reopen(ctx, tx, stmtID); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	lines, err := reporting.BankStatementLines(ctx, tx, stmtID)
	if err != nil {
		t.Fatalf("read lines: %v", err)
	}
	if len(lines) != 2 || lines[0].JournalLineID == nil || lines[1].JournalLineID == nil {
		t.Fatalf("lines after reopen = %+v, want both still matched", lines)
	}
	line100, jl250 := lines[0].ID, *lines[1].JournalLineID
	if err := banking.UnmatchLine(ctx, tx, line100); err != nil {
		t.Fatalf("unmatch: %v", err)
	}
	// The database refuses an amount mismatch (100.00 line vs 250.00 journal line).
	expectDBError(ctx, t, tx, "does not equal", func(sp pgx.Tx) error {
		return banking.MatchLine(ctx, sp, line100, jl250)
	})
	cands, err = reporting.BankMatchCandidates(ctx, tx, stmtID)
	if err != nil || len(cands) != 1 {
		t.Fatalf("candidates after unmatch = %d (err %v), want 1", len(cands), err)
	}
	if err := banking.MatchLine(ctx, tx, line100, cands[0].JournalLineID); err != nil {
		t.Fatalf("manual match: %v", err)
	}
	if err := banking.MatchLine(ctx, tx, line100, cands[0].JournalLineID); !errors.Is(err, banking.ErrAlreadyMatched) {
		t.Fatalf("double match = %v, want ErrAlreadyMatched", err)
	}
	if err := banking.Reconcile(ctx, tx, stmtID); err != nil {
		t.Fatalf("reconcile after rework: %v", err)
	}
}

// TestBankStatementLineRules covers the line-level guard rails that the
// lifecycle test doesn't reach: a journal line can back only one statement
// line across statements, deleting a line releases its match, and deleting
// an open statement cascades.
func TestBankStatementLineRules(t *testing.T) {
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
	exec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
	      SELECT id,'2026-06','2026-06-01','2026-06-30' FROM fiscal_years WHERE name='FY2026'`)
	cashAccount := queryID(`SELECT id FROM accounts WHERE code = '1000'`)
	arAccount := queryID(`SELECT id FROM accounts WHERE code = '1100'`)
	custID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id, $1 FROM o RETURNING id`, arAccount)
	payID := queryID(`INSERT INTO customer_payments (customer_id, payment_date, currency_code, amount, deposit_account_id)
	      VALUES ($1,'2026-06-10','USD',75,$2) RETURNING id`, custID, cashAccount)
	if _, err := posting.PostCustomerPayment(ctx, tx, payID); err != nil {
		t.Fatalf("post payment: %v", err)
	}

	newStatement := func() int {
		t.Helper()
		id, err := banking.CreateStatement(ctx, tx, banking.StatementInput{
			AccountID: cashAccount, StatementDate: "2026-06-30",
			OpeningBalance: "0", ClosingBalance: "75",
		})
		if err != nil {
			t.Fatalf("create statement: %v", err)
		}
		return id
	}
	stmtA, stmtB := newStatement(), newStatement()
	lineA, err := banking.AddLine(ctx, tx, stmtA, banking.LineInput{
		TxnDate: "2026-06-10", Description: "Deposit", Amount: "75",
	})
	if err != nil {
		t.Fatalf("add line A: %v", err)
	}
	lineB, err := banking.AddLine(ctx, tx, stmtB, banking.LineInput{
		TxnDate: "2026-06-10", Description: "Deposit", Amount: "75",
	})
	if err != nil {
		t.Fatalf("add line B: %v", err)
	}

	cands, err := reporting.BankMatchCandidates(ctx, tx, stmtA)
	if err != nil || len(cands) != 1 {
		t.Fatalf("candidates = %d (err %v), want 1", len(cands), err)
	}
	jl := cands[0].JournalLineID
	if err := banking.MatchLine(ctx, tx, lineA, jl); err != nil {
		t.Fatalf("match A: %v", err)
	}
	// The same journal line cannot back a second statement's line.
	expectDBError(ctx, t, tx, "duplicate key", func(sp pgx.Tx) error {
		return banking.MatchLine(ctx, sp, lineB, jl)
	})

	// Deleting the matched line releases the journal line for others.
	if err := banking.DeleteLine(ctx, tx, lineA); err != nil {
		t.Fatalf("delete line A: %v", err)
	}
	if err := banking.MatchLine(ctx, tx, lineB, jl); err != nil {
		t.Fatalf("match B after release: %v", err)
	}

	// Deleting an open statement cascades its lines and releases matches.
	if err := banking.DeleteStatement(ctx, tx, stmtB); err != nil {
		t.Fatalf("delete statement B: %v", err)
	}
	var remaining int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM bank_statement_lines WHERE statement_id = $1`, stmtB).Scan(&remaining); err != nil {
		t.Fatalf("count lines: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("lines remaining after statement delete = %d, want 0", remaining)
	}

	// Unknown ids surface ErrNotFound.
	if err := banking.Reconcile(ctx, tx, 999999); !errors.Is(err, banking.ErrNotFound) {
		t.Fatalf("reconcile missing = %v, want ErrNotFound", err)
	}
	if err := banking.MatchLine(ctx, tx, 999999, jl); !errors.Is(err, banking.ErrNotFound) {
		t.Fatalf("match missing line = %v, want ErrNotFound", err)
	}
}
