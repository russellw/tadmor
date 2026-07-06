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

// AutoApplyCustomerPayment allocates a customer payment's unapplied remainder
// across the customer's open invoices (posted, same currency, with an
// outstanding balance), oldest first. It returns the applications it created,
// which may be empty if there is nothing open to apply to. The allocation is
// computed in SQL so it never over-applies the payment or any invoice; the
// database's over-application constraint is the backstop.
func AutoApplyCustomerPayment(ctx context.Context, tx pgx.Tx, paymentID int) ([]Application, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM customer_payments WHERE id = $1`, paymentID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("posting: customer payment %d: %w", paymentID, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	if status == "void" {
		return nil, fmt.Errorf("posting: customer payment %d is void: %w", paymentID, ErrNotPostable)
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
	return apps, rows.Err()
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
	if status == "void" {
		return nil, fmt.Errorf("posting: supplier payment %d is void: %w", paymentID, ErrNotPostable)
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
	return apps, rows.Err()
}
