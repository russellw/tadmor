package httpapi

// Bank reconciliation: statement CRUD, line capture (manual and CSV import),
// matching against posted journal lines, and the reconcile/reopen lifecycle.

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"tadmor/internal/banking"
	"tadmor/internal/db"
	"tadmor/internal/reporting"
)

func (s *Server) listBankStatements(w http.ResponseWriter, r *http.Request) {
	rows, err := reporting.BankStatements(r.Context(), s.pool)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getBankStatement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	st, err := reporting.BankStatementByID(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) getBankStatementLines(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	lines, err := reporting.BankStatementLines(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

func (s *Server) getBankMatchCandidates(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	cands, err := reporting.BankMatchCandidates(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cands)
}

func (s *Server) createBankStatement(w http.ResponseWriter, r *http.Request) {
	var in banking.StatementInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runCreate(w, r, in.Validate, func(tx pgx.Tx) (int, error) {
		return banking.CreateStatement(r.Context(), tx, in)
	})
}

func (s *Server) updateBankStatement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in banking.StatementInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runBanking(w, r, in.Validate, func(tx pgx.Tx) error {
		return banking.UpdateStatement(r.Context(), tx, id, in)
	})
}

func (s *Server) deleteBankStatement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runBanking(w, r, nil, func(tx pgx.Tx) error {
		return banking.DeleteStatement(r.Context(), tx, id)
	})
}

func (s *Server) addBankStatementLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in banking.LineInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg := in.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	var lineID int
	err := db.WithTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		var e error
		lineID, e = banking.AddLine(r.Context(), tx, id, in)
		return e
	})
	if err != nil {
		s.writeBankingError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": lineID})
}

func (s *Server) importBankStatement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		CSV string `json:"csv"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.CSV == "" {
		writeError(w, http.StatusBadRequest, "csv is required")
		return
	}
	var imported int
	err := db.WithTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		var e error
		imported, e = banking.ImportCSV(r.Context(), tx, id, body.CSV)
		return e
	})
	if err != nil {
		s.writeBankingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"imported": imported})
}

func (s *Server) matchBankStatementLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		JournalLineID int `json:"journal_line_id"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.JournalLineID <= 0 {
		writeError(w, http.StatusBadRequest, "journal_line_id is required")
		return
	}
	s.runBanking(w, r, nil, func(tx pgx.Tx) error {
		return banking.MatchLine(r.Context(), tx, id, body.JournalLineID)
	})
}

func (s *Server) unmatchBankStatementLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runBanking(w, r, nil, func(tx pgx.Tx) error {
		return banking.UnmatchLine(r.Context(), tx, id)
	})
}

func (s *Server) deleteBankStatementLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runBanking(w, r, nil, func(tx pgx.Tx) error {
		return banking.DeleteLine(r.Context(), tx, id)
	})
}

func (s *Server) autoMatchBankStatement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var matched int
	err := db.WithTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		var e error
		matched, e = banking.AutoMatch(r.Context(), tx, id)
		return e
	})
	if err != nil {
		s.writeBankingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"matched": matched})
}

func (s *Server) reconcileBankStatement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runBanking(w, r, nil, func(tx pgx.Tx) error {
		return banking.Reconcile(r.Context(), tx, id)
	})
}

func (s *Server) reopenBankStatement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runBanking(w, r, nil, func(tx pgx.Tx) error {
		return banking.Reopen(r.Context(), tx, id)
	})
}

// runBanking validates the request (when validate is non-nil), runs the
// mutation in a transaction, and writes {"status":"ok"}.
func (s *Server) runBanking(w http.ResponseWriter, r *http.Request, validate func() string, mutate func(pgx.Tx) error) {
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
		s.writeBankingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// writeBankingError maps banking-package sentinels to HTTP codes, falling
// back to the create-error mapper for database constraint violations (e.g. a
// match rejected by the integrity triggers).
func (s *Server) writeBankingError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, banking.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, banking.ErrBadCSV):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, banking.ErrNotOpen),
		errors.Is(err, banking.ErrNotReconciled),
		errors.Is(err, banking.ErrAlreadyMatched):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, banking.ErrUnmatchedLines),
		errors.Is(err, banking.ErrUnbalanced):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		s.writeCreateError(w, err)
	}
}
