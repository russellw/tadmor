package posting

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Application records that an amount of a payment was applied to a document —
// an invoice for customer payments, a bill for supplier payments.
type Application struct {
	DocumentID int    `json:"document_id"`
	Amount     string `json:"amount"`
}

// fxSpec parameterizes realized-FX posting over the four application kinds.
type fxSpec struct {
	// candidates selects the settling document's applications that still
	// need an FX entry: those without one whose two documents were posted at
	// rates that convert the applied amount to different base values. Columns:
	// application id, document number, party control account (A/R or A/P),
	// settlement date, and the signed base difference
	// round(applied·settler rate) − round(applied·document rate), as text.
	candidates string
	// update links the posted FX entry ($1) back to the application ($2).
	update string
	// memo is the FX entry's memo, with the document number interpolated.
	memo string
	// arSide is true when the control account is A/R (customer side): a
	// positive difference is then a gain (the settlement relieved more base
	// value than the receivable carried). On the A/P side the same positive
	// difference is a loss.
	arSide bool
}

// postSettlementFX books the realized exchange gain or loss for every
// application of one settling document (payment or credit note) that needs
// it. Each FX entry is a base-currency entry dated on the settlement date:
// one line trues the party control account up to zero for the settled
// amount, the other books the gain/loss to the configured FX account.
func postSettlementFX(ctx context.Context, tx pgx.Tx, settlerID int, spec fxSpec) error {
	type candidate struct {
		appID        int
		number       string
		partyAccount int
		date         string
		diff         string
	}
	rows, err := tx.Query(ctx, spec.candidates, settlerID)
	if err != nil {
		return err
	}
	var cands []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.appID, &c.number, &c.partyAccount, &c.date, &c.diff); err != nil {
			rows.Close()
			return err
		}
		cands = append(cands, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(cands) == 0 {
		return nil
	}

	var base string
	var fxAccount *int
	if err := tx.QueryRow(ctx,
		`SELECT base_currency, fx_gain_loss_account_id FROM gl_settings`).Scan(&base, &fxAccount); err != nil {
		return err
	}
	if fxAccount == nil {
		return fmt.Errorf("posting: exchange gain/loss account is not configured in settings: %w", ErrMissingAccount)
	}

	for _, c := range cands {
		period, err := periodForDate(ctx, tx, c.date)
		if err != nil {
			return err
		}
		je, err := createEntry(ctx, tx, c.date, period, base, fmt.Sprintf(spec.memo, c.number), c.number)
		if err != nil {
			return err
		}
		// v is oriented so that a positive value debits the control account
		// and credits the FX account; on the A/R side that orientation is the
		// difference as computed, on the A/P side its negation.
		sign := 1
		if !spec.arSide {
			sign = -1
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo, base_debit, base_credit)
			 SELECT $1, 1, $2, greatest(d.v, 0), greatest(-d.v, 0), 'Settlement revaluation',
			        greatest(d.v, 0), greatest(-d.v, 0)
			 FROM (SELECT $3::numeric(19,4) * $4 AS v) d`, je, c.partyAccount, c.diff, sign); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo, base_debit, base_credit)
			 SELECT $1, 2, $2, greatest(-d.v, 0), greatest(d.v, 0), 'Exchange gain (loss)',
			        greatest(-d.v, 0), greatest(d.v, 0)
			 FROM (SELECT $3::numeric(19,4) * $4 AS v) d`, je, *fxAccount, c.diff, sign); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, spec.update, je, c.appID); err != nil {
			return err
		}
	}
	return nil
}

var customerPaymentFX = fxSpec{
	candidates: `
		SELECT pa.id, si.invoice_number, c.ar_account_id, cp.payment_date::text,
		       (round(pa.amount_applied * jes.exchange_rate, 4)
		      - round(pa.amount_applied * jed.exchange_rate, 4))::text
		FROM payment_applications pa
		JOIN customer_payments cp ON cp.id = pa.payment_id
		JOIN customers c          ON c.id = cp.customer_id
		JOIN sales_invoices si    ON si.id = pa.invoice_id
		JOIN journal_entries jes  ON jes.id = cp.journal_entry_id
		JOIN journal_entries jed  ON jed.id = si.journal_entry_id
		WHERE pa.payment_id = $1 AND pa.fx_journal_entry_id IS NULL
		  AND round(pa.amount_applied * jes.exchange_rate, 4)
		   <> round(pa.amount_applied * jed.exchange_rate, 4)`,
	update: `UPDATE payment_applications SET fx_journal_entry_id = $1 WHERE id = $2`,
	memo:   "Exchange difference on settlement of invoice %s",
	arSide: true,
}

// AutoApplyCustomerPayment allocates a customer payment's unapplied remainder
// across the customer's open invoices (posted, same currency, with an
// outstanding balance), oldest first. It returns the applications it created,
// which may be empty if there is nothing open to apply to. The allocation is
// computed in SQL so it never over-applies the payment or any invoice; the
// database's over-application constraint is the backstop. The payment itself
// must be posted — applications realize FX differences between the two
// documents' journal entries, so both entries must exist (the database
// enforces the same rule).
func AutoApplyCustomerPayment(ctx context.Context, tx pgx.Tx, paymentID int) ([]Application, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM customer_payments WHERE id = $1`, paymentID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("posting: customer payment %d: %w", paymentID, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	if status != "posted" {
		return nil, fmt.Errorf("posting: customer payment %d: %w", paymentID, ErrNotPosted)
	}

	rows, err := tx.Query(ctx, `
WITH p AS (
    SELECT cp.id AS payment_id, cp.customer_id, cp.currency_code,
           cp.amount - COALESCE((SELECT sum(amount_applied) FROM payment_applications
                                 WHERE payment_id = cp.id), 0) AS remaining
    FROM customer_payments cp WHERE cp.id = $1
),
open_inv AS (
    -- Availability counts payments and credit notes combined (000012).
    SELECT si.id AS invoice_id, si.invoice_date,
           si.total - invoice_amount_settled(si.id) AS available
    FROM sales_invoices si JOIN p ON si.customer_id = p.customer_id
                                 AND si.currency_code = p.currency_code
    WHERE si.status = 'posted'
),
ranked AS (
    SELECT invoice_id, available,
           COALESCE(sum(available) OVER (ORDER BY invoice_date, invoice_id
                                         ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING), 0) AS prior
    FROM open_inv WHERE available > 0
),
alloc AS (
    SELECT invoice_id,
           LEAST(available, GREATEST((SELECT remaining FROM p) - prior, 0)) AS amount
    FROM ranked
)
INSERT INTO payment_applications (payment_id, invoice_id, amount_applied)
SELECT (SELECT payment_id FROM p), invoice_id, amount
FROM alloc WHERE amount > 0
RETURNING invoice_id, amount_applied::text`, paymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []Application
	for rows.Next() {
		var a Application
		if err := rows.Scan(&a.DocumentID, &a.Amount); err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := postSettlementFX(ctx, tx, paymentID, customerPaymentFX); err != nil {
		return nil, err
	}
	return apps, nil
}

var supplierPaymentFX = fxSpec{
	candidates: `
		SELECT ba.id, pb.bill_number, s.ap_account_id, sp.payment_date::text,
		       (round(ba.amount_applied * jes.exchange_rate, 4)
		      - round(ba.amount_applied * jed.exchange_rate, 4))::text
		FROM bill_applications ba
		JOIN supplier_payments sp ON sp.id = ba.payment_id
		JOIN suppliers s          ON s.id = sp.supplier_id
		JOIN purchase_bills pb    ON pb.id = ba.bill_id
		JOIN journal_entries jes  ON jes.id = sp.journal_entry_id
		JOIN journal_entries jed  ON jed.id = pb.journal_entry_id
		WHERE ba.payment_id = $1 AND ba.fx_journal_entry_id IS NULL
		  AND round(ba.amount_applied * jes.exchange_rate, 4)
		   <> round(ba.amount_applied * jed.exchange_rate, 4)`,
	update: `UPDATE bill_applications SET fx_journal_entry_id = $1 WHERE id = $2`,
	memo:   "Exchange difference on settlement of bill %s",
	arSide: false,
}

// AutoApplySupplierPayment is the purchasing-side mirror of
// AutoApplyCustomerPayment: it allocates a supplier payment across the
// supplier's open bills, oldest first.
func AutoApplySupplierPayment(ctx context.Context, tx pgx.Tx, paymentID int) ([]Application, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM supplier_payments WHERE id = $1`, paymentID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("posting: supplier payment %d: %w", paymentID, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	if status != "posted" {
		return nil, fmt.Errorf("posting: supplier payment %d: %w", paymentID, ErrNotPosted)
	}

	rows, err := tx.Query(ctx, `
WITH p AS (
    SELECT sp.id AS payment_id, sp.supplier_id, sp.currency_code,
           sp.amount - COALESCE((SELECT sum(amount_applied) FROM bill_applications
                                 WHERE payment_id = sp.id), 0) AS remaining
    FROM supplier_payments sp WHERE sp.id = $1
),
open_bill AS (
    -- Availability counts payments and credit notes combined (000012).
    SELECT pb.id AS bill_id, pb.bill_date,
           pb.total - bill_amount_settled(pb.id) AS available
    FROM purchase_bills pb JOIN p ON pb.supplier_id = p.supplier_id
                                 AND pb.currency_code = p.currency_code
    WHERE pb.status = 'posted'
),
ranked AS (
    SELECT bill_id, available,
           COALESCE(sum(available) OVER (ORDER BY bill_date, bill_id
                                         ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING), 0) AS prior
    FROM open_bill WHERE available > 0
),
alloc AS (
    SELECT bill_id,
           LEAST(available, GREATEST((SELECT remaining FROM p) - prior, 0)) AS amount
    FROM ranked
)
INSERT INTO bill_applications (payment_id, bill_id, amount_applied)
SELECT (SELECT payment_id FROM p), bill_id, amount
FROM alloc WHERE amount > 0
RETURNING bill_id, amount_applied::text`, paymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []Application
	for rows.Next() {
		var a Application
		if err := rows.Scan(&a.DocumentID, &a.Amount); err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := postSettlementFX(ctx, tx, paymentID, supplierPaymentFX); err != nil {
		return nil, err
	}
	return apps, nil
}
