package reporting

// Read-side queries for bank reconciliation: statement headers with matching
// progress, statement lines with their matched journal entries, and the
// unmatched journal lines a statement's lines can be matched against.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// BankStatementSummary is a statement header with derived reconciliation
// progress. LinesTotal is the sum of the line amounts (signed, deposits
// positive); Difference is opening_balance + LinesTotal - closing_balance,
// so 0.0000 means the statement adds up.
type BankStatementSummary struct {
	ID             int     `json:"id"`
	AccountID      int     `json:"account_id"`
	AccountCode    string  `json:"account_code"`
	AccountName    string  `json:"account_name"`
	StatementDate  string  `json:"statement_date"`
	OpeningBalance string  `json:"opening_balance"`
	ClosingBalance string  `json:"closing_balance"`
	Reference      *string `json:"reference"`
	Status         string  `json:"status"`
	LineCount      int     `json:"line_count"`
	MatchedCount   int     `json:"matched_count"`
	LinesTotal     string  `json:"lines_total"`
	Difference     string  `json:"difference"`
}

const bankStatementSelect = `
	SELECT s.id, s.account_id, a.code, a.name, s.statement_date::text,
	       s.opening_balance::text, s.closing_balance::text, s.reference, s.status,
	       count(l.id), count(l.journal_line_id),
	       COALESCE(sum(l.amount), 0)::numeric(19,4)::text,
	       (s.opening_balance + COALESCE(sum(l.amount), 0) - s.closing_balance)::numeric(19,4)::text
	FROM bank_statements s
	JOIN accounts a ON a.id = s.account_id
	LEFT JOIN bank_statement_lines l ON l.statement_id = s.id`

func scanBankStatement(row pgx.Row) (BankStatementSummary, error) {
	var s BankStatementSummary
	err := row.Scan(&s.ID, &s.AccountID, &s.AccountCode, &s.AccountName, &s.StatementDate,
		&s.OpeningBalance, &s.ClosingBalance, &s.Reference, &s.Status,
		&s.LineCount, &s.MatchedCount, &s.LinesTotal, &s.Difference)
	return s, err
}

// BankStatements lists all statements, newest first.
func BankStatements(ctx context.Context, q Querier) ([]BankStatementSummary, error) {
	rows, err := q.Query(ctx, bankStatementSelect+`
		 GROUP BY s.id, a.code, a.name
		 ORDER BY s.statement_date DESC, s.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []BankStatementSummary{}
	for rows.Next() {
		s, err := scanBankStatement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// BankStatementByID returns one statement's header and progress.
func BankStatementByID(ctx context.Context, q Querier, id int) (BankStatementSummary, error) {
	s, err := scanBankStatement(q.QueryRow(ctx, bankStatementSelect+`
		 WHERE s.id = $1
		 GROUP BY s.id, a.code, a.name`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrNotFound
	}
	return s, err
}

// BankStatementLine is one statement transaction; the journal fields are set
// when the line is matched. Memo prefers the journal line's own memo, falling
// back to the entry's.
type BankStatementLine struct {
	ID             int     `json:"id"`
	LineNo         int     `json:"line_no"`
	TxnDate        string  `json:"txn_date"`
	Description    string  `json:"description"`
	Reference      *string `json:"reference"`
	Amount         string  `json:"amount"`
	JournalLineID  *int    `json:"journal_line_id"`
	JournalEntryID *int    `json:"journal_entry_id"`
	EntryDate      *string `json:"entry_date"`
	EntryMemo      *string `json:"entry_memo"`
}

// BankStatementLines returns a statement's lines in statement order.
func BankStatementLines(ctx context.Context, q Querier, statementID int) ([]BankStatementLine, error) {
	rows, err := q.Query(ctx,
		`SELECT l.id, l.line_no, l.txn_date::text, l.description, l.reference,
		        l.amount::text, l.journal_line_id, je.id, je.entry_date::text,
		        COALESCE(jl.memo, je.memo)
		 FROM bank_statement_lines l
		 LEFT JOIN journal_lines jl ON jl.id = l.journal_line_id
		 LEFT JOIN journal_entries je ON je.id = jl.journal_entry_id
		 WHERE l.statement_id = $1
		 ORDER BY l.line_no`, statementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []BankStatementLine{}
	for rows.Next() {
		var l BankStatementLine
		if err := rows.Scan(&l.ID, &l.LineNo, &l.TxnDate, &l.Description, &l.Reference,
			&l.Amount, &l.JournalLineID, &l.JournalEntryID, &l.EntryDate, &l.EntryMemo); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// BankMatchCandidate is a posted journal line on the statement's account that
// no statement line (of any statement) has claimed yet. Amount is signed
// debit-positive, matching the statement-line convention.
type BankMatchCandidate struct {
	JournalLineID  int     `json:"journal_line_id"`
	JournalEntryID int     `json:"journal_entry_id"`
	EntryDate      string  `json:"entry_date"`
	Reference      *string `json:"reference"`
	Memo           *string `json:"memo"`
	Amount         string  `json:"amount"`
}

// BankMatchCandidates lists the journal lines a statement's lines can still
// be matched against, oldest entries first.
func BankMatchCandidates(ctx context.Context, q Querier, statementID int) ([]BankMatchCandidate, error) {
	var accountID int
	err := q.QueryRow(ctx, `SELECT account_id FROM bank_statements WHERE id = $1`, statementID).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := q.Query(ctx,
		`SELECT jl.id, je.id, je.entry_date::text, je.reference,
		        COALESCE(jl.memo, je.memo), (jl.debit - jl.credit)::numeric(19,4)::text
		 FROM journal_lines jl
		 JOIN journal_entries je ON je.id = jl.journal_entry_id
		 WHERE je.status = 'posted'
		   AND jl.account_id = $1
		   AND NOT EXISTS (SELECT 1 FROM bank_statement_lines b WHERE b.journal_line_id = jl.id)
		 ORDER BY je.entry_date, je.id, jl.line_no`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []BankMatchCandidate{}
	for rows.Next() {
		var c BankMatchCandidate
		if err := rows.Scan(&c.JournalLineID, &c.JournalEntryID, &c.EntryDate,
			&c.Reference, &c.Memo, &c.Amount); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
