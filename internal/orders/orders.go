// Package orders drives the sales-order and purchase-order lifecycle and the
// fulfilment documents (invoices, bills, stock receipts/shipments) that draw on
// them.
//
// Orders are commercial, not accounting: they never touch the GL. Their status
// moves draft -> open -> closed, with cancelled as an early exit. Fulfilment
// creates ordinary subledger documents whose lines carry an order_line_id back
// to the order; the database's fulfilment views and constraint triggers
// (migration 000013) keep the derived quantities honest, so this package's job
// is to pick the right remaining quantities and copy line attributes forward.
package orders

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var (
	ErrNotFound        = errors.New("order not found")
	ErrNotDraft        = errors.New("order is not in draft status")
	ErrNotOpen         = errors.New("order is not open")
	ErrNoLines         = errors.New("order has no lines")
	ErrFulfilled       = errors.New("order has been partially fulfilled")
	ErrNothingToFulfil = errors.New("order has nothing left to fulfil")
)

// side names the two order flavours so the shared lifecycle helper can build
// its messages and SQL against the right table.
type side struct {
	table      string // sales_orders | purchase_orders
	linesTable string // sales_order_lines | purchase_order_lines
	noun       string // "sales order" | "purchase order"
}

var (
	salesSide    = side{table: "sales_orders", linesTable: "sales_order_lines", noun: "sales order"}
	purchaseSide = side{table: "purchase_orders", linesTable: "purchase_order_lines", noun: "purchase order"}
)

// ConfirmSalesOrder moves a draft sales order to open, making it eligible for
// invoicing and shipping. The order must have at least one line.
func ConfirmSalesOrder(ctx context.Context, tx pgx.Tx, orderID int) error {
	return confirm(ctx, tx, salesSide, orderID)
}

// ConfirmPurchaseOrder is the purchasing-side mirror of ConfirmSalesOrder.
func ConfirmPurchaseOrder(ctx context.Context, tx pgx.Tx, orderID int) error {
	return confirm(ctx, tx, purchaseSide, orderID)
}

func confirm(ctx context.Context, tx pgx.Tx, s side, orderID int) error {
	status, err := statusOf(ctx, tx, s, orderID)
	if err != nil {
		return err
	}
	if status != "draft" {
		return fmt.Errorf("orders: %s %d: %w", s.noun, orderID, ErrNotDraft)
	}
	var lines int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM `+s.linesTable+` WHERE order_id = $1`, orderID).Scan(&lines); err != nil {
		return err
	}
	if lines == 0 {
		return fmt.Errorf("orders: %s %d: %w", s.noun, orderID, ErrNoLines)
	}
	_, err = tx.Exec(ctx, `UPDATE `+s.table+` SET status = 'open' WHERE id = $1`, orderID)
	return err
}

// CloseSalesOrder marks an open sales order complete. It is the manual finish
// for an order that will receive no further invoices or shipments (e.g. a
// short-ship the customer has accepted); fully-fulfilled orders can be closed
// too to take them off the open list.
func CloseSalesOrder(ctx context.Context, tx pgx.Tx, orderID int) error {
	return closeOrder(ctx, tx, salesSide, orderID)
}

// ClosePurchaseOrder is the purchasing-side mirror of CloseSalesOrder.
func ClosePurchaseOrder(ctx context.Context, tx pgx.Tx, orderID int) error {
	return closeOrder(ctx, tx, purchaseSide, orderID)
}

func closeOrder(ctx context.Context, tx pgx.Tx, s side, orderID int) error {
	status, err := statusOf(ctx, tx, s, orderID)
	if err != nil {
		return err
	}
	if status != "open" {
		return fmt.Errorf("orders: %s %d: %w", s.noun, orderID, ErrNotOpen)
	}
	_, err = tx.Exec(ctx, `UPDATE `+s.table+` SET status = 'closed' WHERE id = $1`, orderID)
	return err
}

// CancelSalesOrder terminates a sales order that has not been fulfilled. A
// draft cancels freely; an open order cancels only while nothing has been
// invoiced or shipped against it (otherwise close it instead).
func CancelSalesOrder(ctx context.Context, tx pgx.Tx, orderID int) error {
	return cancel(ctx, tx, salesSide, orderID,
		`SELECT bool_or(qty_invoiced > 0 OR qty_shipped > 0)
		 FROM sales_order_line_fulfilment WHERE order_id = $1`)
}

// CancelPurchaseOrder is the purchasing-side mirror of CancelSalesOrder.
func CancelPurchaseOrder(ctx context.Context, tx pgx.Tx, orderID int) error {
	return cancel(ctx, tx, purchaseSide, orderID,
		`SELECT bool_or(qty_billed > 0 OR qty_received > 0)
		 FROM purchase_order_line_fulfilment WHERE order_id = $1`)
}

func cancel(ctx context.Context, tx pgx.Tx, s side, orderID int, fulfilledSQL string) error {
	status, err := statusOf(ctx, tx, s, orderID)
	if err != nil {
		return err
	}
	switch status {
	case "draft":
		// always cancellable
	case "open":
		var fulfilled *bool
		if err := tx.QueryRow(ctx, fulfilledSQL, orderID).Scan(&fulfilled); err != nil {
			return err
		}
		if fulfilled != nil && *fulfilled {
			return fmt.Errorf("orders: %s %d: %w", s.noun, orderID, ErrFulfilled)
		}
	default:
		return fmt.Errorf("orders: %s %d is %s: %w", s.noun, orderID, status, ErrNotOpen)
	}
	_, err = tx.Exec(ctx, `UPDATE `+s.table+` SET status = 'cancelled' WHERE id = $1`, orderID)
	return err
}

// statusOf returns an order's status, or ErrNotFound.
func statusOf(ctx context.Context, tx pgx.Tx, s side, orderID int) (string, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM `+s.table+` WHERE id = $1`, orderID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("orders: %s %d: %w", s.noun, orderID, ErrNotFound)
	}
	return status, err
}
