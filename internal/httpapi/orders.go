package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"tadmor/internal/db"
	"tadmor/internal/documents"
	"tadmor/internal/orders"
	"tadmor/internal/reporting"
)

// Order handlers: draft creation mirrors the invoice/bill handlers; lifecycle
// (confirm/close/cancel) and fulfilment (invoice/bill/receive/ship) live in the
// orders service layer.

func (s *Server) createSalesOrder(w http.ResponseWriter, r *http.Request) {
	var in documents.SalesOrderInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runCreate(w, r, in.Validate, func(tx pgx.Tx) (int, error) {
		return documents.CreateSalesOrder(r.Context(), tx, in)
	})
}

func (s *Server) createPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	var in documents.PurchaseOrderInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runCreate(w, r, in.Validate, func(tx pgx.Tx) (int, error) {
		return documents.CreatePurchaseOrder(r.Context(), tx, in)
	})
}

// --- lifecycle -------------------------------------------------------------

func (s *Server) confirmSalesOrder(w http.ResponseWriter, r *http.Request) {
	s.runOrderAction(w, r, orders.ConfirmSalesOrder)
}
func (s *Server) closeSalesOrder(w http.ResponseWriter, r *http.Request) {
	s.runOrderAction(w, r, orders.CloseSalesOrder)
}
func (s *Server) cancelSalesOrder(w http.ResponseWriter, r *http.Request) {
	s.runOrderAction(w, r, orders.CancelSalesOrder)
}
func (s *Server) confirmPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	s.runOrderAction(w, r, orders.ConfirmPurchaseOrder)
}
func (s *Server) closePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	s.runOrderAction(w, r, orders.ClosePurchaseOrder)
}
func (s *Server) cancelPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	s.runOrderAction(w, r, orders.CancelPurchaseOrder)
}

// runOrderAction runs a lifecycle transition on the {id} order in a
// transaction and writes {"status":"ok"}.
func (s *Server) runOrderAction(w http.ResponseWriter, r *http.Request, action func(context.Context, pgx.Tx, int) error) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := db.WithTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		return action(r.Context(), tx, id)
	})
	if err != nil {
		s.writeOrderError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- fulfilment ------------------------------------------------------------

func (s *Server) invoiceSalesOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in orders.InvoiceFromOrderInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg := in.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	s.runOrderCreate(w, r, "invoice_id", func(tx pgx.Tx) (int, error) {
		return orders.CreateInvoiceFromSalesOrder(r.Context(), tx, id, in)
	})
}

func (s *Server) billPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in orders.BillFromOrderInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg := in.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	s.runOrderCreate(w, r, "bill_id", func(tx pgx.Tx) (int, error) {
		return orders.CreateBillFromPurchaseOrder(r.Context(), tx, id, in)
	})
}

func (s *Server) receivePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in orders.ReceiveInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg := in.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	s.runOrderMovements(w, r, func(tx pgx.Tx) ([]int, error) {
		return orders.ReceivePurchaseOrder(r.Context(), tx, id, in)
	})
}

func (s *Server) shipSalesOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in orders.ShipInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg := in.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	s.runOrderMovements(w, r, func(tx pgx.Tx) ([]int, error) {
		return orders.ShipSalesOrder(r.Context(), tx, id, in)
	})
}

// runOrderCreate runs a fulfilment that produces one document (an invoice or
// bill) and writes 201 with its id under the given key.
func (s *Server) runOrderCreate(w http.ResponseWriter, r *http.Request, key string, create func(pgx.Tx) (int, error)) {
	var docID int
	err := db.WithTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		var e error
		docID, e = create(tx)
		return e
	})
	if err != nil {
		s.writeOrderError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{key: docID})
}

// runOrderMovements runs a fulfilment that produces stock movements and writes
// 201 with the created movement ids.
func (s *Server) runOrderMovements(w http.ResponseWriter, r *http.Request, create func(pgx.Tx) ([]int, error)) {
	var ids []int
	err := db.WithTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		var e error
		ids, e = create(tx)
		return e
	})
	if err != nil {
		s.writeOrderError(w, err)
		return
	}
	if ids == nil {
		ids = []int{}
	}
	writeJSON(w, http.StatusCreated, map[string][]int{"movement_ids": ids})
}

// writeOrderError maps orders-package sentinels to HTTP codes, falling back to
// the create-error mapper for database constraint violations (e.g. a duplicate
// invoice number produced by a fulfilment).
func (s *Server) writeOrderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, orders.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, orders.ErrNotDraft),
		errors.Is(err, orders.ErrNotOpen),
		errors.Is(err, orders.ErrFulfilled):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, orders.ErrNoLines),
		errors.Is(err, orders.ErrNothingToFulfil):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		s.writeCreateError(w, err)
	}
}

// --- reads -----------------------------------------------------------------

func (s *Server) listSalesOrders(w http.ResponseWriter, r *http.Request) {
	rows, err := reporting.SalesOrders(r.Context(), s.pool)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getSalesOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	o, err := reporting.SalesOrder(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) getSalesOrderLines(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	lines, err := reporting.SalesOrderLines(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

func (s *Server) listPurchaseOrders(w http.ResponseWriter, r *http.Request) {
	rows, err := reporting.PurchaseOrders(r.Context(), s.pool)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	o, err := reporting.PurchaseOrder(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) getPurchaseOrderLines(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	lines, err := reporting.PurchaseOrderLines(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}
