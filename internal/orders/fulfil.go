package orders

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// LineQty requests a specific quantity against one order line. Callers pass a
// set of these to fulfil an order partially; an empty set means "fulfil every
// line's full remaining quantity".
type LineQty struct {
	OrderLineID int    `json:"order_line_id"`
	Quantity    string `json:"quantity"`
}

// split turns the requested lines into parallel id/quantity arrays for SQL
// unnest. Empty arrays signal "no override": take each line's full remainder.
func split(lines []LineQty) ([]int, []string) {
	ids := make([]int, 0, len(lines))
	qtys := make([]string, 0, len(lines))
	for _, l := range lines {
		ids = append(ids, l.OrderLineID)
		qtys = append(qtys, l.Quantity)
	}
	return ids, qtys
}

// InvoiceFromOrderInput is the fulfilment request for turning open sales-order
// lines into a draft invoice.
type InvoiceFromOrderInput struct {
	InvoiceNumber string    `json:"invoice_number"`
	InvoiceDate   string    `json:"invoice_date"` // YYYY-MM-DD
	DueDate       *string   `json:"due_date"`
	Lines         []LineQty `json:"lines"` // empty = all remaining
}

func (in InvoiceFromOrderInput) Validate() string {
	switch {
	case in.InvoiceNumber == "":
		return "invoice_number is required"
	case in.InvoiceDate == "":
		return "invoice_date is required"
	}
	return ""
}

// CreateInvoiceFromSalesOrder creates a draft sales invoice from an open sales
// order's not-yet-invoiced quantities, linking each invoice line back to its
// order line. It returns the new invoice id, or ErrNothingToFulfil when there
// is nothing left to invoice.
func CreateInvoiceFromSalesOrder(ctx context.Context, tx pgx.Tx, orderID int, in InvoiceFromOrderInput) (int, error) {
	var customerID int
	var currency, orderNumber string
	err := tx.QueryRow(ctx,
		`SELECT customer_id, currency_code, order_number FROM sales_orders WHERE id = $1 AND status = 'open'`,
		orderID).Scan(&customerID, &currency, &orderNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, notFoundOrNotOpen(ctx, tx, salesSide, orderID)
	}
	if err != nil {
		return 0, err
	}

	var invoiceID int
	if err := tx.QueryRow(ctx,
		`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, due_date, currency_code, reference)
		 VALUES ($1, $2, $3::date, $4::date, $5, $6)
		 RETURNING id`,
		in.InvoiceNumber, customerID, in.InvoiceDate, in.DueDate, currency, orderNumber).Scan(&invoiceID); err != nil {
		return 0, err
	}

	ids, qtys := split(in.Lines)
	tag, err := tx.Exec(ctx, `
WITH req AS (
    SELECT order_line_id, qty FROM unnest($2::int[], $3::numeric[]) AS r(order_line_id, qty)
),
picked AS (
    SELECT sol.id AS order_line_id, sol.product_id, sol.description, sol.unit_price,
           sol.revenue_account_id, sol.tax_code, sol.tax_rate,
           CASE WHEN (SELECT count(*) FROM req) = 0 THEN f.qty_to_invoice
                ELSE LEAST(f.qty_to_invoice, COALESCE(r.qty, 0)) END AS qty
    FROM sales_order_lines sol
    JOIN sales_order_line_fulfilment f ON f.order_line_id = sol.id
    LEFT JOIN req r ON r.order_line_id = sol.id
    WHERE sol.order_id = $4
),
numbered AS (
    SELECT *, row_number() OVER (ORDER BY order_line_id) AS rn FROM picked WHERE qty > 0
)
INSERT INTO sales_invoice_lines
    (invoice_id, line_no, product_id, description, quantity, unit_price, revenue_account_id, tax_code, tax_rate, order_line_id)
SELECT $1, rn, product_id, description, qty, unit_price, revenue_account_id, tax_code, tax_rate, order_line_id
FROM numbered`,
		invoiceID, ids, qtys, orderID)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 0 {
		return 0, fmt.Errorf("orders: %s %d: %w", salesSide.noun, orderID, ErrNothingToFulfil)
	}
	return invoiceID, nil
}

// BillFromOrderInput is the fulfilment request for turning open purchase-order
// lines into a draft bill.
type BillFromOrderInput struct {
	BillNumber string    `json:"bill_number"`
	BillDate   string    `json:"bill_date"` // YYYY-MM-DD
	DueDate    *string   `json:"due_date"`
	Lines      []LineQty `json:"lines"` // empty = all remaining
}

func (in BillFromOrderInput) Validate() string {
	switch {
	case in.BillNumber == "":
		return "bill_number is required"
	case in.BillDate == "":
		return "bill_date is required"
	}
	return ""
}

// CreateBillFromPurchaseOrder is the purchasing-side mirror of
// CreateInvoiceFromSalesOrder.
func CreateBillFromPurchaseOrder(ctx context.Context, tx pgx.Tx, orderID int, in BillFromOrderInput) (int, error) {
	var supplierID int
	var currency, orderNumber string
	err := tx.QueryRow(ctx,
		`SELECT supplier_id, currency_code, order_number FROM purchase_orders WHERE id = $1 AND status = 'open'`,
		orderID).Scan(&supplierID, &currency, &orderNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, notFoundOrNotOpen(ctx, tx, purchaseSide, orderID)
	}
	if err != nil {
		return 0, err
	}

	var billID int
	if err := tx.QueryRow(ctx,
		`INSERT INTO purchase_bills (bill_number, supplier_id, bill_date, due_date, currency_code, reference)
		 VALUES ($1, $2, $3::date, $4::date, $5, $6)
		 RETURNING id`,
		in.BillNumber, supplierID, in.BillDate, in.DueDate, currency, orderNumber).Scan(&billID); err != nil {
		return 0, err
	}

	ids, qtys := split(in.Lines)
	tag, err := tx.Exec(ctx, `
WITH req AS (
    SELECT order_line_id, qty FROM unnest($2::int[], $3::numeric[]) AS r(order_line_id, qty)
),
picked AS (
    SELECT pol.id AS order_line_id, pol.product_id, pol.description, pol.unit_cost,
           pol.expense_account_id, pol.tax_code, pol.tax_rate,
           CASE WHEN (SELECT count(*) FROM req) = 0 THEN f.qty_to_bill
                ELSE LEAST(f.qty_to_bill, COALESCE(r.qty, 0)) END AS qty
    FROM purchase_order_lines pol
    JOIN purchase_order_line_fulfilment f ON f.order_line_id = pol.id
    LEFT JOIN req r ON r.order_line_id = pol.id
    WHERE pol.order_id = $4
),
numbered AS (
    SELECT *, row_number() OVER (ORDER BY order_line_id) AS rn FROM picked WHERE qty > 0
)
INSERT INTO purchase_bill_lines
    (bill_id, line_no, product_id, description, quantity, unit_cost, expense_account_id, tax_code, tax_rate, order_line_id)
SELECT $1, rn, product_id, description, qty, unit_cost, expense_account_id, tax_code, tax_rate, order_line_id
FROM numbered`,
		billID, ids, qtys, orderID)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 0 {
		return 0, fmt.Errorf("orders: %s %d: %w", purchaseSide.noun, orderID, ErrNothingToFulfil)
	}
	return billID, nil
}

// ReceiveInput is the request to receive goods against a purchase order into a
// warehouse.
type ReceiveInput struct {
	WarehouseID  int       `json:"warehouse_id"`
	MovementDate *string   `json:"movement_date"` // YYYY-MM-DD; defaults to today
	Reference    *string   `json:"reference"`
	Lines        []LineQty `json:"lines"` // empty = all remaining
}

func (in ReceiveInput) Validate() string {
	if in.WarehouseID <= 0 {
		return "warehouse_id is required"
	}
	return ""
}

// ReceivePurchaseOrder creates draft receipt stock movements for the
// inventory-tracked lines of an open purchase order, at the order's unit cost,
// linked back to their order lines. Non-stock lines (services) are skipped:
// they are billed, not received. It returns the ids of the movements created,
// which the caller posts to the GL (Dr inventory / Cr GRNI) as usual, or
// ErrNothingToFulfil when there is nothing left to receive.
func ReceivePurchaseOrder(ctx context.Context, tx pgx.Tx, orderID int, in ReceiveInput) ([]int, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM purchase_orders WHERE id = $1`, orderID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("orders: %s %d: %w", purchaseSide.noun, orderID, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	if status != "open" {
		return nil, fmt.Errorf("orders: %s %d: %w", purchaseSide.noun, orderID, ErrNotOpen)
	}

	ids, qtys := split(in.Lines)
	rows, err := tx.Query(ctx, `
WITH req AS (
    SELECT order_line_id, qty FROM unnest($1::int[], $2::numeric[]) AS r(order_line_id, qty)
),
picked AS (
    SELECT pol.id AS order_line_id, pol.product_id, pol.unit_cost,
           CASE WHEN (SELECT count(*) FROM req) = 0 THEN f.qty_to_receive
                ELSE LEAST(f.qty_to_receive, COALESCE(r.qty, 0)) END AS qty
    FROM purchase_order_lines pol
    JOIN purchase_order_line_fulfilment f ON f.order_line_id = pol.id
    JOIN products p ON p.id = pol.product_id AND p.track_inventory AND p.is_active
    LEFT JOIN req r ON r.order_line_id = pol.id
    WHERE pol.order_id = $4
)
INSERT INTO stock_movements
    (product_id, warehouse_id, movement_type, movement_date, quantity, unit_cost, source_type, source_id, reference)
SELECT product_id, $3, 'receipt', COALESCE($5::date, current_date), qty, unit_cost,
       'purchase_order_line', order_line_id, $6
FROM picked WHERE qty > 0
RETURNING id`,
		ids, qtys, in.WarehouseID, orderID, in.MovementDate, in.Reference)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var movements []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		movements = append(movements, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(movements) == 0 {
		return nil, fmt.Errorf("orders: %s %d: %w", purchaseSide.noun, orderID, ErrNothingToFulfil)
	}
	return movements, nil
}

// ShipInput is the request to ship goods against a sales order from a warehouse.
type ShipInput struct {
	WarehouseID  int       `json:"warehouse_id"`
	MovementDate *string   `json:"movement_date"` // YYYY-MM-DD; defaults to today
	Reference    *string   `json:"reference"`
	Lines        []LineQty `json:"lines"` // empty = all remaining
}

func (in ShipInput) Validate() string {
	if in.WarehouseID <= 0 {
		return "warehouse_id is required"
	}
	return ""
}

// ShipSalesOrder creates draft issue stock movements for the inventory-tracked
// lines of an open sales order, out of the given warehouse, at that warehouse's
// current moving-average cost, linked back to their order lines. It returns the
// ids of the movements created, which the caller posts to the GL (Dr COGS / Cr
// inventory), or ErrNothingToFulfil when there is nothing left to ship.
func ShipSalesOrder(ctx context.Context, tx pgx.Tx, orderID int, in ShipInput) ([]int, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM sales_orders WHERE id = $1`, orderID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("orders: %s %d: %w", salesSide.noun, orderID, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	if status != "open" {
		return nil, fmt.Errorf("orders: %s %d: %w", salesSide.noun, orderID, ErrNotOpen)
	}

	ids, qtys := split(in.Lines)
	rows, err := tx.Query(ctx, `
WITH req AS (
    SELECT order_line_id, qty FROM unnest($1::int[], $2::numeric[]) AS r(order_line_id, qty)
),
picked AS (
    SELECT sol.id AS order_line_id, sol.product_id,
           COALESCE(soh.avg_unit_cost, 0) AS unit_cost,
           CASE WHEN (SELECT count(*) FROM req) = 0 THEN f.qty_to_ship
                ELSE LEAST(f.qty_to_ship, COALESCE(r.qty, 0)) END AS qty
    FROM sales_order_lines sol
    JOIN sales_order_line_fulfilment f ON f.order_line_id = sol.id
    JOIN products p ON p.id = sol.product_id AND p.track_inventory AND p.is_active
    LEFT JOIN stock_on_hand soh ON soh.product_id = sol.product_id AND soh.warehouse_id = $3
    LEFT JOIN req r ON r.order_line_id = sol.id
    WHERE sol.order_id = $4
)
INSERT INTO stock_movements
    (product_id, warehouse_id, movement_type, movement_date, quantity, unit_cost, source_type, source_id, reference)
SELECT product_id, $3, 'issue', COALESCE($5::date, current_date), -qty, unit_cost,
       'sales_order_line', order_line_id, $6
FROM picked WHERE qty > 0
RETURNING id`,
		ids, qtys, in.WarehouseID, orderID, in.MovementDate, in.Reference)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var movements []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		movements = append(movements, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(movements) == 0 {
		return nil, fmt.Errorf("orders: %s %d: %w", salesSide.noun, orderID, ErrNothingToFulfil)
	}
	return movements, nil
}

// notFoundOrNotOpen distinguishes a missing order from a merely non-open one
// after a status-guarded lookup returned no row.
func notFoundOrNotOpen(ctx context.Context, tx pgx.Tx, s side, orderID int) error {
	status, err := statusOf(ctx, tx, s, orderID)
	if err != nil {
		return err
	}
	return fmt.Errorf("orders: %s %d is %s: %w", s.noun, orderID, status, ErrNotOpen)
}
