// Package banking manages bank statements and their reconciliation against
// the general ledger.
//
// A statement is captured for a cash account (CSV import or manual entry)
// while 'open'; each of its lines is then matched 1:1 to a posted journal
// line on that account, and the statement is marked 'reconciled' once every
// line is matched and opening balance + lines = closing balance. Amounts are
// signed from the book's perspective: deposits positive, withdrawals
// negative. The database enforces the invariants (see migration 000017); the
// checks here exist to fail early with distinguishable errors.
//
// Decimal fields are accepted as strings and cast to numeric in SQL, so
// values are exact and never pass through Go (or JSON) floating point.
package banking

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Errors callers may test with errors.Is.
var (
	ErrNotFound       = errors.New("not found")
	ErrNotOpen        = errors.New("bank statement is not open")
	ErrNotReconciled  = errors.New("bank statement is not reconciled")
	ErrAlreadyMatched = errors.New("statement line is already matched")
	ErrUnmatchedLines = errors.New("bank statement has unmatched lines")
	ErrUnbalanced     = errors.New("opening balance plus statement lines does not equal the closing balance")
	ErrBadCSV         = errors.New("invalid CSV")
)

var amountRE = regexp.MustCompile(`^-?(\d+(\.\d*)?|\.\d+)$`)

// StatementInput is the writable header of a bank statement.
type StatementInput struct {
	AccountID      int     `json:"account_id"`
	StatementDate  string  `json:"statement_date"` // YYYY-MM-DD
	OpeningBalance string  `json:"opening_balance"`
	ClosingBalance string  `json:"closing_balance"`
	Reference      *string `json:"reference"`
}

// Validate returns a message describing the first validation problem, or "".
func (in StatementInput) Validate() string {
	switch {
	case in.AccountID <= 0:
		return "account_id is required"
	case in.StatementDate == "":
		return "statement_date is required"
	case in.OpeningBalance == "":
		return "opening_balance is required"
	case in.ClosingBalance == "":
		return "closing_balance is required"
	}
	return ""
}

// LineInput is one statement transaction.
type LineInput struct {
	TxnDate     string  `json:"txn_date"` // YYYY-MM-DD
	Description string  `json:"description"`
	Reference   *string `json:"reference"`
	Amount      string  `json:"amount"` // signed decimal; deposits positive
}

// Validate returns a message describing the first validation problem, or "".
func (in LineInput) Validate() string {
	switch {
	case in.TxnDate == "":
		return "txn_date is required"
	case in.Description == "":
		return "description is required"
	case in.Amount == "":
		return "amount is required"
	}
	return ""
}

// CreateStatement inserts an open statement, returning its id.
func CreateStatement(ctx context.Context, tx pgx.Tx, in StatementInput) (int, error) {
	var id int
	err := tx.QueryRow(ctx,
		`INSERT INTO bank_statements (account_id, statement_date, opening_balance, closing_balance, reference)
		 VALUES ($1, $2::date, $3::numeric, $4::numeric, $5)
		 RETURNING id`,
		in.AccountID, in.StatementDate, in.OpeningBalance, in.ClosingBalance, in.Reference).Scan(&id)
	return id, err
}

// statementStatus reads a statement's status, distinguishing "missing".
func statementStatus(ctx context.Context, tx pgx.Tx, id int) (string, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM bank_statements WHERE id = $1`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("banking: statement %d: %w", id, ErrNotFound)
	}
	return status, err
}

// requireOpen fails with ErrNotFound / ErrNotOpen unless the statement exists
// and is open.
func requireOpen(ctx context.Context, tx pgx.Tx, id int) error {
	status, err := statementStatus(ctx, tx, id)
	if err != nil {
		return err
	}
	if status != "open" {
		return fmt.Errorf("banking: statement %d is %s: %w", id, status, ErrNotOpen)
	}
	return nil
}

// UpdateStatement rewrites an open statement's header.
func UpdateStatement(ctx context.Context, tx pgx.Tx, id int, in StatementInput) error {
	if err := requireOpen(ctx, tx, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`UPDATE bank_statements
		    SET account_id = $2, statement_date = $3::date,
		        opening_balance = $4::numeric, closing_balance = $5::numeric, reference = $6
		  WHERE id = $1`,
		id, in.AccountID, in.StatementDate, in.OpeningBalance, in.ClosingBalance, in.Reference)
	return err
}

// DeleteStatement removes an open statement; its lines cascade.
func DeleteStatement(ctx context.Context, tx pgx.Tx, id int) error {
	if err := requireOpen(ctx, tx, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM bank_statements WHERE id = $1`, id)
	return err
}

// AddLine appends one transaction to an open statement, returning the line id.
func AddLine(ctx context.Context, tx pgx.Tx, statementID int, in LineInput) (int, error) {
	if err := requireOpen(ctx, tx, statementID); err != nil {
		return 0, err
	}
	var id int
	err := tx.QueryRow(ctx,
		`INSERT INTO bank_statement_lines (statement_id, line_no, txn_date, description, reference, amount)
		 VALUES ($1,
		         COALESCE((SELECT max(line_no) FROM bank_statement_lines WHERE statement_id = $1), 0) + 1,
		         $2::date, $3, $4, $5::numeric)
		 RETURNING id`,
		statementID, in.TxnDate, in.Description, in.Reference, in.Amount).Scan(&id)
	return id, err
}

// ImportCSV parses statement transactions from CSV text and appends them to
// an open statement, returning how many lines were added.
//
// Expected columns: date (YYYY-MM-DD), description, amount (signed decimal,
// deposits positive) and an optional fourth reference column. A header row is
// skipped when its first field is not a date. Empty records are ignored.
func ImportCSV(ctx context.Context, tx pgx.Tx, statementID int, csvText string) (int, error) {
	lines, err := parseCSV(csvText)
	if err != nil {
		return 0, err
	}
	if len(lines) == 0 {
		return 0, fmt.Errorf("banking: no data rows: %w", ErrBadCSV)
	}
	// AddLine re-reads max(line_no), so appending one at a time stays correct.
	for _, l := range lines {
		if _, err := AddLine(ctx, tx, statementID, l); err != nil {
			return 0, err
		}
	}
	return len(lines), nil
}

// parseCSV turns CSV text into line inputs, validating each field so the
// error can name the offending record.
func parseCSV(csvText string) ([]LineInput, error) {
	r := csv.NewReader(strings.NewReader(csvText))
	r.FieldsPerRecord = -1 // validated per record below
	r.TrimLeadingSpace = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("banking: %v: %w", err, ErrBadCSV)
	}

	out := []LineInput{}
	for i, rec := range records {
		if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue
		}
		if len(rec) != 3 && len(rec) != 4 {
			return nil, fmt.Errorf("banking: record %d has %d fields, want date,description,amount[,reference]: %w",
				i+1, len(rec), ErrBadCSV)
		}
		for j := range rec {
			rec[j] = strings.TrimSpace(rec[j])
		}
		if _, err := time.Parse("2006-01-02", rec[0]); err != nil {
			if i == 0 {
				continue // header row
			}
			return nil, fmt.Errorf("banking: record %d: %q is not a YYYY-MM-DD date: %w", i+1, rec[0], ErrBadCSV)
		}
		if rec[1] == "" {
			return nil, fmt.Errorf("banking: record %d: description is empty: %w", i+1, ErrBadCSV)
		}
		if !amountRE.MatchString(rec[2]) {
			return nil, fmt.Errorf("banking: record %d: %q is not a decimal amount: %w", i+1, rec[2], ErrBadCSV)
		}
		if strings.Trim(rec[2], "-.0") == "" {
			return nil, fmt.Errorf("banking: record %d: amount must not be zero: %w", i+1, ErrBadCSV)
		}
		l := LineInput{TxnDate: rec[0], Description: rec[1], Amount: rec[2]}
		if len(rec) == 4 && rec[3] != "" {
			l.Reference = &rec[3]
		}
		out = append(out, l)
	}
	return out, nil
}

// lineStatement resolves a statement line to its statement id, requiring the
// statement to be open.
func lineStatement(ctx context.Context, tx pgx.Tx, lineID int) (int, error) {
	var statementID int
	var status string
	err := tx.QueryRow(ctx,
		`SELECT s.id, s.status FROM bank_statement_lines l
		 JOIN bank_statements s ON s.id = l.statement_id
		 WHERE l.id = $1`, lineID).Scan(&statementID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("banking: statement line %d: %w", lineID, ErrNotFound)
	}
	if err != nil {
		return 0, err
	}
	if status != "open" {
		return 0, fmt.Errorf("banking: statement %d is %s: %w", statementID, status, ErrNotOpen)
	}
	return statementID, nil
}

// DeleteLine removes a transaction from an open statement (releasing its
// match, if any) and renumbers nothing: line_no gaps are harmless.
func DeleteLine(ctx context.Context, tx pgx.Tx, lineID int) error {
	if _, err := lineStatement(ctx, tx, lineID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM bank_statement_lines WHERE id = $1`, lineID)
	return err
}

// MatchLine pairs an unmatched statement line with a journal line. The
// database verifies the pairing (posted entry, same account, equal signed
// amount, journal line not already used).
func MatchLine(ctx context.Context, tx pgx.Tx, lineID, journalLineID int) error {
	if _, err := lineStatement(ctx, tx, lineID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx,
		`UPDATE bank_statement_lines SET journal_line_id = $2
		  WHERE id = $1 AND journal_line_id IS NULL`, lineID, journalLineID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("banking: statement line %d: %w", lineID, ErrAlreadyMatched)
	}
	return nil
}

// UnmatchLine releases a statement line's match. Unmatching an unmatched
// line is a no-op.
func UnmatchLine(ctx context.Context, tx pgx.Tx, lineID int) error {
	if _, err := lineStatement(ctx, tx, lineID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`UPDATE bank_statement_lines SET journal_line_id = NULL WHERE id = $1`, lineID)
	return err
}

// AutoMatch pairs each unmatched line of an open statement with the posted,
// unused journal line on the statement's account that has the same signed
// amount, preferring the nearest entry date (ties broken by lowest journal
// line id). Lines with no such candidate are skipped. Returns how many lines
// were matched.
func AutoMatch(ctx context.Context, tx pgx.Tx, statementID int) (int, error) {
	if err := requireOpen(ctx, tx, statementID); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx,
		`SELECT id FROM bank_statement_lines
		 WHERE statement_id = $1 AND journal_line_id IS NULL ORDER BY line_no`, statementID)
	if err != nil {
		return 0, err
	}
	lineIDs := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		lineIDs = append(lineIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	// One line at a time so each match consumes its journal line before the
	// next line picks a candidate.
	matched := 0
	for _, lineID := range lineIDs {
		tag, err := tx.Exec(ctx,
			`UPDATE bank_statement_lines l SET journal_line_id = (
			    SELECT jl.id FROM journal_lines jl
			    JOIN journal_entries je ON je.id = jl.journal_entry_id
			    JOIN bank_statements s ON s.id = l.statement_id
			    WHERE je.status = 'posted'
			      AND jl.account_id = s.account_id
			      AND jl.debit - jl.credit = l.amount
			      AND NOT EXISTS (SELECT 1 FROM bank_statement_lines used
			                      WHERE used.journal_line_id = jl.id)
			    ORDER BY abs(je.entry_date - l.txn_date), jl.id
			    LIMIT 1)
			 WHERE l.id = $1
			   AND EXISTS (
			    SELECT 1 FROM journal_lines jl
			    JOIN journal_entries je ON je.id = jl.journal_entry_id
			    JOIN bank_statements s ON s.id = l.statement_id
			    WHERE je.status = 'posted'
			      AND jl.account_id = s.account_id
			      AND jl.debit - jl.credit = l.amount
			      AND NOT EXISTS (SELECT 1 FROM bank_statement_lines used
			                      WHERE used.journal_line_id = jl.id))`, lineID)
		if err != nil {
			return 0, err
		}
		matched += int(tag.RowsAffected())
	}
	return matched, nil
}

// Reconcile finalizes an open statement: every line must be matched and the
// statement must add up (opening + lines = closing). The database enforces
// the same rules; the checks here exist to return distinguishable errors.
func Reconcile(ctx context.Context, tx pgx.Tx, id int) error {
	if err := requireOpen(ctx, tx, id); err != nil {
		return err
	}
	var unmatched int
	var balances bool
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FILTER (WHERE l.journal_line_id IS NULL),
		        s.opening_balance + COALESCE(sum(l.amount), 0) = s.closing_balance
		 FROM bank_statements s
		 LEFT JOIN bank_statement_lines l ON l.statement_id = s.id
		 WHERE s.id = $1
		 GROUP BY s.id`, id).Scan(&unmatched, &balances); err != nil {
		return err
	}
	if unmatched > 0 {
		return fmt.Errorf("banking: statement %d has %d unmatched lines: %w", id, unmatched, ErrUnmatchedLines)
	}
	if !balances {
		return fmt.Errorf("banking: statement %d: %w", id, ErrUnbalanced)
	}
	_, err := tx.Exec(ctx,
		`UPDATE bank_statements SET status = 'reconciled', reconciled_at = now() WHERE id = $1`, id)
	return err
}

// Reopen returns a reconciled statement to open so it can be corrected.
func Reopen(ctx context.Context, tx pgx.Tx, id int) error {
	status, err := statementStatus(ctx, tx, id)
	if err != nil {
		return err
	}
	if status != "reconciled" {
		return fmt.Errorf("banking: statement %d is %s: %w", id, status, ErrNotReconciled)
	}
	_, err = tx.Exec(ctx,
		`UPDATE bank_statements SET status = 'open', reconciled_at = NULL WHERE id = $1`, id)
	return err
}
