// Package httpapi exposes the HTTP surface of the application.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tadmor/internal/db"
	"tadmor/internal/posting"
)

// Server holds the dependencies shared by the HTTP handlers.
type Server struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// NewServer builds a Server over the given pool.
func NewServer(pool *pgxpool.Pool, log *slog.Logger) *Server {
	return &Server{pool: pool, log: log}
}

// Handler wires the routes. Routing uses the standard library's method-aware
// ServeMux (Go 1.22+), so no third-party router is needed.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.pool.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "database unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	// Post a draft subledger document to the general ledger.
	mux.HandleFunc("POST /sales-invoices/{id}/post", s.postSalesInvoice)
	mux.HandleFunc("POST /purchase-bills/{id}/post", s.postPurchaseBill)
	mux.HandleFunc("POST /customer-payments/{id}/post", s.postCustomerPayment)
	mux.HandleFunc("POST /supplier-payments/{id}/post", s.postSupplierPayment)
	mux.HandleFunc("POST /stock-movements/{id}/post", s.postStockMovement)

	// Auto-apply a payment to the counterparty's open documents, oldest first.
	mux.HandleFunc("POST /customer-payments/{id}/apply", s.applyCustomerPayment)
	mux.HandleFunc("POST /supplier-payments/{id}/apply", s.applySupplierPayment)

	return mux
}

func (s *Server) postSalesInvoice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runPost(w, r, func(tx pgx.Tx) (int, error) {
		return posting.PostSalesInvoice(r.Context(), tx, id)
	})
}

func (s *Server) postPurchaseBill(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runPost(w, r, func(tx pgx.Tx) (int, error) {
		return posting.PostPurchaseBill(r.Context(), tx, id)
	})
}

func (s *Server) postCustomerPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runPost(w, r, func(tx pgx.Tx) (int, error) {
		return posting.PostCustomerPayment(r.Context(), tx, id)
	})
}

func (s *Server) postSupplierPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runPost(w, r, func(tx pgx.Tx) (int, error) {
		return posting.PostSupplierPayment(r.Context(), tx, id)
	})
}

func (s *Server) postStockMovement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	// Stock movements carry no currency of their own; the caller supplies the
	// functional currency. Receipts also need a credit (clearing) account.
	var body struct {
		Currency        string `json:"currency"`
		CreditAccountID int    `json:"credit_account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Currency == "" {
		writeError(w, http.StatusBadRequest, "currency is required")
		return
	}
	s.runPost(w, r, func(tx pgx.Tx) (int, error) {
		return posting.PostStockMovement(r.Context(), tx, id, body.Currency, body.CreditAccountID)
	})
}

func (s *Server) applyCustomerPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runApply(w, r, func(tx pgx.Tx) ([]posting.Application, error) {
		return posting.AutoApplyCustomerPayment(r.Context(), tx, id)
	})
}

func (s *Server) applySupplierPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runApply(w, r, func(tx pgx.Tx) ([]posting.Application, error) {
		return posting.AutoApplySupplierPayment(r.Context(), tx, id)
	})
}

// runApply executes an auto-application inside a transaction and writes the
// applications it created, or an appropriate error response.
func (s *Server) runApply(w http.ResponseWriter, r *http.Request, apply func(pgx.Tx) ([]posting.Application, error)) {
	var apps []posting.Application
	err := db.WithTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		var e error
		apps, e = apply(tx)
		return e
	})
	if err != nil {
		s.writePostingError(w, err)
		return
	}
	if apps == nil {
		apps = []posting.Application{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"applications": apps})
}

// runPost executes a posting function inside a transaction and writes the
// resulting journal-entry id, or an appropriate error response.
func (s *Server) runPost(w http.ResponseWriter, r *http.Request, post func(pgx.Tx) (int, error)) {
	var journalEntryID int
	err := db.WithTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		var e error
		journalEntryID, e = post(tx)
		return e
	})
	if err != nil {
		s.writePostingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"journal_entry_id": journalEntryID})
}

func (s *Server) writePostingError(w http.ResponseWriter, err error) {
	switch status := postingStatus(err); status {
	case http.StatusInternalServerError:
		s.log.Error("posting failed", "err", err)
		writeError(w, status, "internal error")
	default:
		writeError(w, status, err.Error())
	}
}

// postingStatus maps a posting error to an HTTP status code.
func postingStatus(err error) int {
	switch {
	case errors.Is(err, posting.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, posting.ErrNotDraft), errors.Is(err, posting.ErrAlreadyPosted):
		return http.StatusConflict
	case errors.Is(err, posting.ErrNotPostable),
		errors.Is(err, posting.ErrNoOpenPeriod),
		errors.Is(err, posting.ErrMissingAccount),
		errors.Is(err, posting.ErrNothingToPost):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

func pathID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
