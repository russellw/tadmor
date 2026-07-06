package httpapi

import (
	"net/http"

	"github.com/jackc/pgx/v5"

	"tadmor/internal/documents"
	"tadmor/internal/posting"
	"tadmor/internal/reporting"
)

// Credit-note handlers: thin wrappers over the documents/posting/reporting
// layers, mirroring the invoice and bill handlers in server.go.

func (s *Server) createSalesCreditNote(w http.ResponseWriter, r *http.Request) {
	var in documents.SalesCreditNoteInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runCreate(w, r, in.Validate, func(tx pgx.Tx) (int, error) {
		return documents.CreateSalesCreditNote(r.Context(), tx, in)
	})
}

func (s *Server) createPurchaseCreditNote(w http.ResponseWriter, r *http.Request) {
	var in documents.PurchaseCreditNoteInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runCreate(w, r, in.Validate, func(tx pgx.Tx) (int, error) {
		return documents.CreatePurchaseCreditNote(r.Context(), tx, in)
	})
}

func (s *Server) postSalesCreditNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runPost(w, r, func(tx pgx.Tx) (int, error) {
		return posting.PostSalesCreditNote(r.Context(), tx, id)
	})
}

func (s *Server) postPurchaseCreditNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runPost(w, r, func(tx pgx.Tx) (int, error) {
		return posting.PostPurchaseCreditNote(r.Context(), tx, id)
	})
}

func (s *Server) applySalesCreditNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runApply(w, r, func(tx pgx.Tx) ([]posting.Application, error) {
		return posting.AutoApplySalesCreditNote(r.Context(), tx, id)
	})
}

func (s *Server) applyPurchaseCreditNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runApply(w, r, func(tx pgx.Tx) ([]posting.Application, error) {
		return posting.AutoApplyPurchaseCreditNote(r.Context(), tx, id)
	})
}

func (s *Server) unpostSalesCreditNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runReverse(w, r, func(tx pgx.Tx) (int, error) {
		return posting.UnpostSalesCreditNote(r.Context(), tx, id)
	})
}

func (s *Server) unpostPurchaseCreditNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runReverse(w, r, func(tx pgx.Tx) (int, error) {
		return posting.UnpostPurchaseCreditNote(r.Context(), tx, id)
	})
}

func (s *Server) listSalesCreditNotes(w http.ResponseWriter, r *http.Request) {
	rows, err := reporting.SalesCreditNoteBalances(r.Context(), s.pool)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getSalesCreditNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	note, err := reporting.SalesCreditNoteBalance(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, note)
}

func (s *Server) getSalesCreditNoteLines(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	lines, err := reporting.SalesCreditNoteLines(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

func (s *Server) getSalesCreditNoteApplications(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	apps, err := reporting.SalesCreditNoteApplications(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apps)
}

func (s *Server) listPurchaseCreditNotes(w http.ResponseWriter, r *http.Request) {
	rows, err := reporting.PurchaseCreditNoteBalances(r.Context(), s.pool)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getPurchaseCreditNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	note, err := reporting.PurchaseCreditNoteBalance(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, note)
}

func (s *Server) getPurchaseCreditNoteLines(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	lines, err := reporting.PurchaseCreditNoteLines(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

func (s *Server) getPurchaseCreditNoteApplications(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	apps, err := reporting.PurchaseCreditNoteApplications(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apps)
}
