package documents

// Draft documents stay editable until they are posted (or, for orders,
// confirmed): update replaces the header fields and the full line set, delete
// removes the document outright. Both are guarded by the draft status in the
// WHERE clause, so a posted document can never be rewritten here.
//
// Documents produced by order fulfilment (invoice/bill lines carrying an
// order_line_id, stock movements carrying a source_type) are not editable:
// their lines are what draws down the order, so a rewrite would corrupt the
// fulfilment arithmetic. They may still be deleted while draft — the
// fulfilment views sum the surviving lines, so deletion simply returns the
// quantities to the order.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var (
	ErrNotFound    = errors.New("document not found")
	ErrNotDraft    = errors.New("document is not in draft status")
	ErrOrderLinked = errors.New("document was created from an order")
)

// notFoundOrNotDraft explains why a draft-guarded write matched no row: the
// document is missing, or it is past draft.
func notFoundOrNotDraft(ctx context.Context, tx pgx.Tx, statusQuery, noun string, id int) error {
	var status string
	err := tx.QueryRow(ctx, statusQuery, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("documents: %s %d: %w", noun, id, ErrNotFound)
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("documents: %s %d is %s: %w", noun, id, status, ErrNotDraft)
}

// refuseOrderLinked rejects the edit when existsQuery reports fulfilment links.
func refuseOrderLinked(ctx context.Context, tx pgx.Tx, existsQuery, noun string, id int) error {
	var linked bool
	if err := tx.QueryRow(ctx, existsQuery, id).Scan(&linked); err != nil {
		return err
	}
	if linked {
		return fmt.Errorf("documents: %s %d: %w", noun, id, ErrOrderLinked)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Sales invoices
// ---------------------------------------------------------------------------

// UpdateSalesInvoice rewrites a draft invoice's header and replaces its lines.
func UpdateSalesInvoice(ctx context.Context, tx pgx.Tx, id int, in SalesInvoiceInput) error {
	if err := refuseOrderLinked(ctx, tx,
		`SELECT EXISTS(SELECT 1 FROM sales_invoice_lines WHERE invoice_id = $1 AND order_line_id IS NOT NULL)`,
		"sales invoice", id); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx,
		`UPDATE sales_invoices
		    SET invoice_number = $2, customer_id = $3, invoice_date = $4::date,
		        due_date = $5::date, currency_code = $6, reference = $7, memo = $8
		  WHERE id = $1 AND status = 'draft'`,
		id, in.InvoiceNumber, in.CustomerID, in.InvoiceDate, in.DueDate, in.CurrencyCode, in.Reference, in.Memo)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM sales_invoices WHERE id = $1`, "sales invoice", id)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sales_invoice_lines WHERE invoice_id = $1`, id); err != nil {
		return err
	}
	return insertSalesInvoiceLines(ctx, tx, id, in.Lines)
}

// DeleteSalesInvoice removes a draft invoice; its lines cascade.
func DeleteSalesInvoice(ctx context.Context, tx pgx.Tx, id int) error {
	tag, err := tx.Exec(ctx, `DELETE FROM sales_invoices WHERE id = $1 AND status = 'draft'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM sales_invoices WHERE id = $1`, "sales invoice", id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Purchase bills
// ---------------------------------------------------------------------------

// UpdatePurchaseBill rewrites a draft bill's header and replaces its lines.
func UpdatePurchaseBill(ctx context.Context, tx pgx.Tx, id int, in PurchaseBillInput) error {
	if err := refuseOrderLinked(ctx, tx,
		`SELECT EXISTS(SELECT 1 FROM purchase_bill_lines WHERE bill_id = $1 AND order_line_id IS NOT NULL)`,
		"purchase bill", id); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx,
		`UPDATE purchase_bills
		    SET bill_number = $2, supplier_id = $3, bill_date = $4::date,
		        due_date = $5::date, currency_code = $6, reference = $7, memo = $8
		  WHERE id = $1 AND status = 'draft'`,
		id, in.BillNumber, in.SupplierID, in.BillDate, in.DueDate, in.CurrencyCode, in.Reference, in.Memo)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM purchase_bills WHERE id = $1`, "purchase bill", id)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM purchase_bill_lines WHERE bill_id = $1`, id); err != nil {
		return err
	}
	return insertPurchaseBillLines(ctx, tx, id, in.Lines)
}

// DeletePurchaseBill removes a draft bill; its lines cascade.
func DeletePurchaseBill(ctx context.Context, tx pgx.Tx, id int) error {
	tag, err := tx.Exec(ctx, `DELETE FROM purchase_bills WHERE id = $1 AND status = 'draft'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM purchase_bills WHERE id = $1`, "purchase bill", id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Credit notes
// ---------------------------------------------------------------------------

// UpdateSalesCreditNote rewrites a draft credit note's header and replaces its
// lines.
func UpdateSalesCreditNote(ctx context.Context, tx pgx.Tx, id int, in SalesCreditNoteInput) error {
	tag, err := tx.Exec(ctx,
		`UPDATE sales_credit_notes
		    SET credit_note_number = $2, customer_id = $3, credit_note_date = $4::date,
		        currency_code = $5, reference = $6, memo = $7
		  WHERE id = $1 AND status = 'draft'`,
		id, in.CreditNoteNumber, in.CustomerID, in.CreditNoteDate, in.CurrencyCode, in.Reference, in.Memo)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM sales_credit_notes WHERE id = $1`, "sales credit note", id)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sales_credit_note_lines WHERE credit_note_id = $1`, id); err != nil {
		return err
	}
	return insertSalesCreditNoteLines(ctx, tx, id, in.Lines)
}

// DeleteSalesCreditNote removes a draft credit note; its lines cascade.
func DeleteSalesCreditNote(ctx context.Context, tx pgx.Tx, id int) error {
	tag, err := tx.Exec(ctx, `DELETE FROM sales_credit_notes WHERE id = $1 AND status = 'draft'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM sales_credit_notes WHERE id = $1`, "sales credit note", id)
	}
	return nil
}

// UpdatePurchaseCreditNote rewrites a draft credit note's header and replaces
// its lines.
func UpdatePurchaseCreditNote(ctx context.Context, tx pgx.Tx, id int, in PurchaseCreditNoteInput) error {
	tag, err := tx.Exec(ctx,
		`UPDATE purchase_credit_notes
		    SET credit_note_number = $2, supplier_id = $3, credit_note_date = $4::date,
		        currency_code = $5, reference = $6, memo = $7
		  WHERE id = $1 AND status = 'draft'`,
		id, in.CreditNoteNumber, in.SupplierID, in.CreditNoteDate, in.CurrencyCode, in.Reference, in.Memo)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM purchase_credit_notes WHERE id = $1`, "purchase credit note", id)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM purchase_credit_note_lines WHERE credit_note_id = $1`, id); err != nil {
		return err
	}
	return insertPurchaseCreditNoteLines(ctx, tx, id, in.Lines)
}

// DeletePurchaseCreditNote removes a draft credit note; its lines cascade.
func DeletePurchaseCreditNote(ctx context.Context, tx pgx.Tx, id int) error {
	tag, err := tx.Exec(ctx, `DELETE FROM purchase_credit_notes WHERE id = $1 AND status = 'draft'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM purchase_credit_notes WHERE id = $1`, "purchase credit note", id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Orders
// ---------------------------------------------------------------------------

// Draft orders cannot have been fulfilled (fulfilment requires an open order,
// and confirmation is one-way), so no order-link guard is needed here.

// UpdateSalesOrder rewrites a draft order's header and replaces its lines.
func UpdateSalesOrder(ctx context.Context, tx pgx.Tx, id int, in SalesOrderInput) error {
	tag, err := tx.Exec(ctx,
		`UPDATE sales_orders
		    SET order_number = $2, customer_id = $3, order_date = $4::date,
		        expected_ship_date = $5::date, currency_code = $6, reference = $7, memo = $8
		  WHERE id = $1 AND status = 'draft'`,
		id, in.OrderNumber, in.CustomerID, in.OrderDate, in.ExpectedShipDate, in.CurrencyCode, in.Reference, in.Memo)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM sales_orders WHERE id = $1`, "sales order", id)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sales_order_lines WHERE order_id = $1`, id); err != nil {
		return err
	}
	return insertSalesOrderLines(ctx, tx, id, in.Lines)
}

// DeleteSalesOrder removes a draft order; its lines cascade.
func DeleteSalesOrder(ctx context.Context, tx pgx.Tx, id int) error {
	tag, err := tx.Exec(ctx, `DELETE FROM sales_orders WHERE id = $1 AND status = 'draft'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM sales_orders WHERE id = $1`, "sales order", id)
	}
	return nil
}

// UpdatePurchaseOrder rewrites a draft order's header and replaces its lines.
func UpdatePurchaseOrder(ctx context.Context, tx pgx.Tx, id int, in PurchaseOrderInput) error {
	tag, err := tx.Exec(ctx,
		`UPDATE purchase_orders
		    SET order_number = $2, supplier_id = $3, order_date = $4::date,
		        expected_receipt_date = $5::date, currency_code = $6, reference = $7, memo = $8
		  WHERE id = $1 AND status = 'draft'`,
		id, in.OrderNumber, in.SupplierID, in.OrderDate, in.ExpectedReceiptDate, in.CurrencyCode, in.Reference, in.Memo)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM purchase_orders WHERE id = $1`, "purchase order", id)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM purchase_order_lines WHERE order_id = $1`, id); err != nil {
		return err
	}
	return insertPurchaseOrderLines(ctx, tx, id, in.Lines)
}

// DeletePurchaseOrder removes a draft order; its lines cascade.
func DeletePurchaseOrder(ctx context.Context, tx pgx.Tx, id int) error {
	tag, err := tx.Exec(ctx, `DELETE FROM purchase_orders WHERE id = $1 AND status = 'draft'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM purchase_orders WHERE id = $1`, "purchase order", id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Payments
// ---------------------------------------------------------------------------

// UpdateCustomerPayment rewrites a draft payment. Draft payments carry no
// applications (unposting deletes them), so only the row itself changes.
func UpdateCustomerPayment(ctx context.Context, tx pgx.Tx, id int, in CustomerPaymentInput) error {
	tag, err := tx.Exec(ctx,
		`UPDATE customer_payments
		    SET customer_id = $2, payment_date = $3::date, currency_code = $4,
		        amount = $5::numeric, method = $6, reference = $7, deposit_account_id = $8
		  WHERE id = $1 AND status = 'draft'`,
		id, in.CustomerID, in.PaymentDate, in.CurrencyCode, in.Amount, in.Method, in.Reference, in.DepositAccountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM customer_payments WHERE id = $1`, "customer payment", id)
	}
	return nil
}

// DeleteCustomerPayment removes a draft payment.
func DeleteCustomerPayment(ctx context.Context, tx pgx.Tx, id int) error {
	tag, err := tx.Exec(ctx, `DELETE FROM customer_payments WHERE id = $1 AND status = 'draft'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM customer_payments WHERE id = $1`, "customer payment", id)
	}
	return nil
}

// UpdateSupplierPayment rewrites a draft payment.
func UpdateSupplierPayment(ctx context.Context, tx pgx.Tx, id int, in SupplierPaymentInput) error {
	tag, err := tx.Exec(ctx,
		`UPDATE supplier_payments
		    SET supplier_id = $2, payment_date = $3::date, currency_code = $4,
		        amount = $5::numeric, method = $6, reference = $7, payment_account_id = $8
		  WHERE id = $1 AND status = 'draft'`,
		id, in.SupplierID, in.PaymentDate, in.CurrencyCode, in.Amount, in.Method, in.Reference, in.PaymentAccountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM supplier_payments WHERE id = $1`, "supplier payment", id)
	}
	return nil
}

// DeleteSupplierPayment removes a draft payment.
func DeleteSupplierPayment(ctx context.Context, tx pgx.Tx, id int) error {
	tag, err := tx.Exec(ctx, `DELETE FROM supplier_payments WHERE id = $1 AND status = 'draft'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFoundOrNotDraft(ctx, tx, `SELECT status FROM supplier_payments WHERE id = $1`, "supplier payment", id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Stock movements
// ---------------------------------------------------------------------------

// A stock movement has no status column: it is draft while journal_entry_id is
// null. Movements created by order fulfilment (ship/receive) carry a
// source_type and are not editable — rewriting one would corrupt the order's
// shipped/received arithmetic — but may be deleted while draft to undo the
// fulfilment.

// stockMovementWriteError explains why a guarded stock-movement write matched
// no row.
func stockMovementWriteError(ctx context.Context, tx pgx.Tx, id int, forUpdate bool) error {
	var posted, linked bool
	err := tx.QueryRow(ctx,
		`SELECT journal_entry_id IS NOT NULL, source_type IS NOT NULL FROM stock_movements WHERE id = $1`,
		id).Scan(&posted, &linked)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("documents: stock movement %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return err
	}
	if posted {
		return fmt.Errorf("documents: stock movement %d is posted: %w", id, ErrNotDraft)
	}
	if forUpdate && linked {
		return fmt.Errorf("documents: stock movement %d: %w", id, ErrOrderLinked)
	}
	return fmt.Errorf("documents: stock movement %d: %w", id, ErrNotFound)
}

// UpdateStockMovement rewrites an unposted, non-fulfilment movement.
func UpdateStockMovement(ctx context.Context, tx pgx.Tx, id int, in StockMovementInput) error {
	tag, err := tx.Exec(ctx,
		`UPDATE stock_movements
		    SET product_id = $2, warehouse_id = $3, movement_type = $4,
		        movement_date = COALESCE($5::date, current_date),
		        quantity = $6::numeric, unit_cost = $7::numeric, reference = $8, notes = $9
		  WHERE id = $1 AND journal_entry_id IS NULL AND source_type IS NULL`,
		id, in.ProductID, in.WarehouseID, in.MovementType, in.MovementDate,
		in.Quantity, orDefault(in.UnitCost, "0"), in.Reference, in.Notes)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return stockMovementWriteError(ctx, tx, id, true)
	}
	return nil
}

// DeleteStockMovement removes an unposted movement.
func DeleteStockMovement(ctx context.Context, tx pgx.Tx, id int) error {
	tag, err := tx.Exec(ctx, `DELETE FROM stock_movements WHERE id = $1 AND journal_entry_id IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return stockMovementWriteError(ctx, tx, id, false)
	}
	return nil
}
