package posting

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CloseResult reports what closing a fiscal year produced. ClosingEntryID is
// zero when the year had no revenue or expense balances to sweep (no entry is
// posted then); NextFiscalYearID is zero when the following year already
// existed (or its derived name was already taken).
type CloseResult struct {
	ClosingEntryID   int
	NextFiscalYearID int
}

// CloseFiscalYear performs the year-end close of an open fiscal year, in one
// transaction supplied by the caller:
//
//   - posts a closing entry dated on the year's last day that zeroes every
//     revenue and expense account and moves the net income into the given
//     retained-earnings account (flagged is_closing so income statements can
//     exclude it);
//   - closes all of the year's accounting periods and the year itself;
//   - creates the next fiscal year when none covers the following day, so the
//     calendar rolls forward without manual setup.
//
// Earlier fiscal years must already be closed — the sweep takes cumulative
// balances up to the year end, which only equal the year's own activity once
// every prior year has been swept to zero.
//
// The closing entry needs a period covering the year-end date: a missing one
// is auto-created (like document posting does on month rollover), and one the
// user already closed is used anyway — every period in the year is closed
// again as part of the operation.
func CloseFiscalYear(ctx context.Context, tx pgx.Tx, fiscalYearID, retainedEarningsAccountID int) (CloseResult, error) {
	var res CloseResult

	var name, endDate, status string
	err := tx.QueryRow(ctx,
		`SELECT name, end_date::text, status FROM fiscal_years WHERE id = $1`, fiscalYearID).
		Scan(&name, &endDate, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return res, fmt.Errorf("posting: fiscal year %d: %w", fiscalYearID, ErrNotFound)
	}
	if err != nil {
		return res, err
	}
	if status != "open" {
		return res, fmt.Errorf("posting: fiscal year %s: %w", name, ErrYearNotOpen)
	}

	var priorOpen bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM fiscal_years
		 WHERE id <> $1 AND status = 'open'
		   AND start_date < (SELECT start_date FROM fiscal_years WHERE id = $1))`,
		fiscalYearID).Scan(&priorOpen); err != nil {
		return res, err
	}
	if priorOpen {
		return res, fmt.Errorf("posting: fiscal year %s: %w", name, ErrPriorYearOpen)
	}

	var accountOK bool
	err = tx.QueryRow(ctx,
		`SELECT is_postable AND is_active AND account_type = 'equity' FROM accounts WHERE id = $1`,
		retainedEarningsAccountID).Scan(&accountOK)
	if errors.Is(err, pgx.ErrNoRows) {
		return res, fmt.Errorf("posting: retained-earnings account %d: %w", retainedEarningsAccountID, ErrMissingAccount)
	}
	if err != nil {
		return res, err
	}
	if !accountOK {
		return res, fmt.Errorf("posting: retained-earnings account %d must be a postable, active equity account: %w",
			retainedEarningsAccountID, ErrMissingAccount)
	}

	// The per-account revenue/expense balances the closing entry must zero,
	// cumulative over all posted entries up to the year end (equal to the
	// year's own activity because every earlier year is already swept).
	const balancesSQL = `
		SELECT jl.account_id, sum(jl.debit - jl.credit) AS bal
		FROM journal_lines jl
		JOIN journal_entries je ON je.id = jl.journal_entry_id
		JOIN accounts a ON a.id = jl.account_id
		WHERE je.status = 'posted'
		  AND a.account_type IN ('revenue', 'expense')
		  AND je.entry_date <= $1::date
		GROUP BY jl.account_id
		HAVING sum(jl.debit - jl.credit) <> 0`

	var toSweep int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM (`+balancesSQL+`) b`, endDate).Scan(&toSweep); err != nil {
		return res, err
	}

	if toSweep > 0 {
		// The entry itself is currency-neutral (it restates GL balances), but
		// the header needs a currency: use the one most of the ledger is in.
		var currency string
		if err := tx.QueryRow(ctx,
			`SELECT currency_code FROM journal_entries
			 WHERE status = 'posted' AND entry_date <= $1::date
			 GROUP BY currency_code ORDER BY count(*) DESC, currency_code LIMIT 1`,
			endDate).Scan(&currency); err != nil {
			return res, err
		}

		// Land the entry in the period covering the year-end date. A closed
		// one is briefly reopened — the close below shuts every period again —
		// and a missing one is auto-created via the usual posting path.
		var periodID int
		var periodStatus string
		err := tx.QueryRow(ctx,
			`SELECT id, status FROM accounting_periods
			 WHERE $1::date BETWEEN start_date AND end_date ORDER BY id LIMIT 1`,
			endDate).Scan(&periodID, &periodStatus)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if periodID, err = periodForDate(ctx, tx, endDate); err != nil {
				return res, err
			}
		case err != nil:
			return res, err
		case periodStatus == "closed":
			if _, err := tx.Exec(ctx,
				`UPDATE accounting_periods SET status = 'open' WHERE id = $1`, periodID); err != nil {
				return res, err
			}
		}

		var je int
		if err := tx.QueryRow(ctx,
			`INSERT INTO journal_entries (entry_date, period_id, currency_code, memo, reference, status, posted_at, is_closing)
			 VALUES ($1::date, $2, $3, $4, $5, 'posted', now(), true)
			 RETURNING id`,
			endDate, periodID, currency, "Year-end close "+name, name).Scan(&je); err != nil {
			return res, err
		}

		// One line per swept account on its balancing side, then the retained-
		// earnings line for the net income (credit) or net loss (debit).
		if _, err := tx.Exec(ctx,
			`WITH bal AS (`+balancesSQL+`),
			 lines AS (
			     SELECT 0 AS ord, account_id,
			            CASE WHEN bal < 0 THEN -bal ELSE 0 END AS debit,
			            CASE WHEN bal > 0 THEN bal ELSE 0 END AS credit,
			            'Year-end close'::text AS memo
			     FROM bal
			     UNION ALL
			     SELECT 1, $3,
			            CASE WHEN t.total > 0 THEN t.total ELSE 0 END,
			            CASE WHEN t.total < 0 THEN -t.total ELSE 0 END,
			            'Net income (loss) for ' || $4
			     FROM (SELECT sum(bal) AS total FROM bal) t
			     WHERE t.total <> 0
			 )
			 INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
			 SELECT $2, row_number() OVER (ORDER BY ord, account_id), account_id, debit, credit, memo
			 FROM lines`, endDate, je, retainedEarningsAccountID, name); err != nil {
			return res, err
		}
		res.ClosingEntryID = je
	}

	if _, err := tx.Exec(ctx,
		`UPDATE accounting_periods SET status = 'closed' WHERE fiscal_year_id = $1 AND status = 'open'`,
		fiscalYearID); err != nil {
		return res, err
	}
	var closingEntry *int
	if res.ClosingEntryID != 0 {
		closingEntry = &res.ClosingEntryID
	}
	if _, err := tx.Exec(ctx,
		`UPDATE fiscal_years SET status = 'closed', closing_entry_id = $2 WHERE id = $1`,
		fiscalYearID, closingEntry); err != nil {
		return res, err
	}

	// Roll the calendar forward: same-length year named after its final
	// calendar year, unless one already covers the next day (or the derived
	// name is taken — then the user names the next year themselves).
	err = tx.QueryRow(ctx,
		`INSERT INTO fiscal_years (name, start_date, end_date)
		 SELECT 'FY' || to_char(($1::date + 1 + interval '1 year' - interval '1 day')::date, 'YYYY'),
		        $1::date + 1,
		        ($1::date + 1 + interval '1 year' - interval '1 day')::date
		 WHERE NOT EXISTS (SELECT 1 FROM fiscal_years WHERE ($1::date + 1) BETWEEN start_date AND end_date)
		 ON CONFLICT (name) DO NOTHING
		 RETURNING id`, endDate).Scan(&res.NextFiscalYearID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return res, err
	}
	return res, nil
}

// ReopenFiscalYear reverses a year-end close: the fiscal year returns to open
// and its closing entry (if one was posted) is reversed, restoring the
// revenue and expense balances. Only the most recently closed year may be
// reopened — a later year's close swept cumulative balances that depend on
// this one staying swept.
//
// Only the period that held the closing entry is reopened (the reversal must
// post somewhere); the year's other periods stay closed until reopened by
// hand. It returns the reversal entry's id, or zero when there was no closing
// entry to reverse.
func ReopenFiscalYear(ctx context.Context, tx pgx.Tx, fiscalYearID int) (int, error) {
	var name, status string
	var closingEntry *int
	err := tx.QueryRow(ctx,
		`SELECT name, status, closing_entry_id FROM fiscal_years WHERE id = $1`, fiscalYearID).
		Scan(&name, &status, &closingEntry)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("posting: fiscal year %d: %w", fiscalYearID, ErrNotFound)
	}
	if err != nil {
		return 0, err
	}
	if status != "closed" {
		return 0, fmt.Errorf("posting: fiscal year %s: %w", name, ErrYearNotClosed)
	}

	var laterClosed bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM fiscal_years
		 WHERE status = 'closed'
		   AND start_date > (SELECT start_date FROM fiscal_years WHERE id = $1))`,
		fiscalYearID).Scan(&laterClosed); err != nil {
		return 0, err
	}
	if laterClosed {
		return 0, fmt.Errorf("posting: fiscal year %s: %w", name, ErrLaterYearClosed)
	}

	// Reopen the year first — the period trigger forbids open periods in a
	// closed year.
	if _, err := tx.Exec(ctx,
		`UPDATE fiscal_years SET status = 'open', closing_entry_id = NULL WHERE id = $1`,
		fiscalYearID); err != nil {
		return 0, err
	}
	if closingEntry == nil {
		return 0, nil
	}
	if _, err := tx.Exec(ctx,
		`UPDATE accounting_periods SET status = 'open'
		 WHERE id = (SELECT period_id FROM journal_entries WHERE id = $1) AND status = 'closed'`,
		*closingEntry); err != nil {
		return 0, err
	}
	return reverseEntry(ctx, tx, *closingEntry)
}
