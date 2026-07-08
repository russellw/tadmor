package httpapi

// Draft-document edit and delete handlers. Each PUT replaces the document's
// header and full line set; each DELETE removes it. The documents package
// enforces that only drafts (and no order-linked documents, for edits) are
// touched.

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"tadmor/internal/db"
	"tadmor/internal/documents"
)

func (s *Server) updateSalesInvoice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in documents.SalesInvoiceInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runMutate(w, r, in.Validate, func(tx pgx.Tx) error {
		return documents.UpdateSalesInvoice(r.Context(), tx, id, in)
	})
}

func (s *Server) deleteSalesInvoice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runMutate(w, r, nil, func(tx pgx.Tx) error {
		return documents.DeleteSalesInvoice(r.Context(), tx, id)
	})
}

func (s *Server) updatePurchaseBill(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in documents.PurchaseBillInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runMutate(w, r, in.Validate, func(tx pgx.Tx) error {
		return documents.UpdatePurchaseBill(r.Context(), tx, id, in)
	})
}

func (s *Server) deletePurchaseBill(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runMutate(w, r, nil, func(tx pgx.Tx) error {
		return documents.DeletePurchaseBill(r.Context(), tx, id)
	})
}

func (s *Server) updateSalesCreditNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in documents.SalesCreditNoteInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runMutate(w, r, in.Validate, func(tx pgx.Tx) error {
		return documents.UpdateSalesCreditNote(r.Context(), tx, id, in)
	})
}

func (s *Server) deleteSalesCreditNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runMutate(w, r, nil, func(tx pgx.Tx) error {
		return documents.DeleteSalesCreditNote(r.Context(), tx, id)
	})
}

func (s *Server) updatePurchaseCreditNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in documents.PurchaseCreditNoteInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runMutate(w, r, in.Validate, func(tx pgx.Tx) error {
		return documents.UpdatePurchaseCreditNote(r.Context(), tx, id, in)
	})
}

func (s *Server) deletePurchaseCreditNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runMutate(w, r, nil, func(tx pgx.Tx) error {
		return documents.DeletePurchaseCreditNote(r.Context(), tx, id)
	})
}

func (s *Server) updateSalesOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in documents.SalesOrderInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runMutate(w, r, in.Validate, func(tx pgx.Tx) error {
		return documents.UpdateSalesOrder(r.Context(), tx, id, in)
	})
}

func (s *Server) deleteSalesOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runMutate(w, r, nil, func(tx pgx.Tx) error {
		return documents.DeleteSalesOrder(r.Context(), tx, id)
	})
}

func (s *Server) updatePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in documents.PurchaseOrderInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runMutate(w, r, in.Validate, func(tx pgx.Tx) error {
		return documents.UpdatePurchaseOrder(r.Context(), tx, id, in)
	})
}

func (s *Server) deletePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runMutate(w, r, nil, func(tx pgx.Tx) error {
		return documents.DeletePurchaseOrder(r.Context(), tx, id)
	})
}

func (s *Server) updateCustomerPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in documents.CustomerPaymentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runMutate(w, r, in.Validate, func(tx pgx.Tx) error {
		return documents.UpdateCustomerPayment(r.Context(), tx, id, in)
	})
}

func (s *Server) deleteCustomerPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runMutate(w, r, nil, func(tx pgx.Tx) error {
		return documents.DeleteCustomerPayment(r.Context(), tx, id)
	})
}

func (s *Server) updateSupplierPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in documents.SupplierPaymentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runMutate(w, r, in.Validate, func(tx pgx.Tx) error {
		return documents.UpdateSupplierPayment(r.Context(), tx, id, in)
	})
}

func (s *Server) deleteSupplierPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runMutate(w, r, nil, func(tx pgx.Tx) error {
		return documents.DeleteSupplierPayment(r.Context(), tx, id)
	})
}

func (s *Server) updateStockMovement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in documents.StockMovementInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runMutate(w, r, in.Validate, func(tx pgx.Tx) error {
		return documents.UpdateStockMovement(r.Context(), tx, id, in)
	})
}

func (s *Server) deleteStockMovement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runMutate(w, r, nil, func(tx pgx.Tx) error {
		return documents.DeleteStockMovement(r.Context(), tx, id)
	})
}

// runMutate validates the request (when validate is non-nil), runs the
// mutation in a transaction, and writes {"status":"ok"}.
func (s *Server) runMutate(w http.ResponseWriter, r *http.Request, validate func() string, mutate func(pgx.Tx) error) {
	if validate != nil {
		if msg := validate(); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
	}
	err := db.WithTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		return mutate(tx)
	})
	if err != nil {
		s.writeMutateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// writeMutateError maps documents-package sentinels to HTTP codes, falling
// back to the create-error mapper for database constraint violations (e.g. a
// duplicate document number introduced by an edit).
func (s *Server) writeMutateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, documents.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, documents.ErrNotDraft),
		errors.Is(err, documents.ErrOrderLinked):
		writeError(w, http.StatusConflict, err.Error())
	default:
		s.writeCreateError(w, err)
	}
}
