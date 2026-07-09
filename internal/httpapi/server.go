// Package httpapi exposes the HTTP surface of the application.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"tadmor/internal/db"
	"tadmor/internal/documents"
	"tadmor/internal/mailer"
	"tadmor/internal/posting"
	"tadmor/internal/printing"
	"tadmor/internal/reporting"
)

// Server holds the dependencies shared by the HTTP handlers.
type Server struct {
	pool   *pgxpool.Pool
	log    *slog.Logger
	mailer mailer.Mailer
}

// Option configures a Server built by NewServer.
type Option func(*Server)

// WithMailer sets the mailer the email endpoints use. Without it a Server
// defaults to a no-op mailer, so email attempts report "not configured".
func WithMailer(m mailer.Mailer) Option {
	return func(s *Server) { s.mailer = m }
}

// NewServer builds a Server over the given pool. Email sending defaults to the
// no-op mailer; pass WithMailer to enable it.
func NewServer(pool *pgxpool.Pool, log *slog.Logger, opts ...Option) *Server {
	s := &Server{pool: pool, log: log, mailer: mailer.New(mailer.Config{}, log)}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Handler wires the routes. Routing uses the standard library's method-aware
// ServeMux (Go 1.22+), so no third-party router is needed.
//
// distFS, when non-nil, is the built front-end (the contents of web/dist); it is
// served for any path outside /api/, with an index.html fallback so client-side
// routing works. Pass nil to disable SPA serving (e.g. in API tests).
func (s *Server) Handler(distFS fs.FS) http.Handler {
	// API routes are registered unprefixed and mounted under /api/ below via
	// StripPrefix, so the handler patterns stay short and the front-end's own
	// client-side routes can't collide with API paths.
	api := http.NewServeMux()

	// Create draft subledger documents.
	api.HandleFunc("POST /sales-invoices", s.createSalesInvoice)
	api.HandleFunc("POST /purchase-bills", s.createPurchaseBill)
	api.HandleFunc("POST /sales-credit-notes", s.createSalesCreditNote)
	api.HandleFunc("POST /purchase-credit-notes", s.createPurchaseCreditNote)
	api.HandleFunc("POST /customer-payments", s.createCustomerPayment)
	api.HandleFunc("POST /supplier-payments", s.createSupplierPayment)
	api.HandleFunc("POST /stock-movements", s.createStockMovement)
	api.HandleFunc("POST /sales-orders", s.createSalesOrder)
	api.HandleFunc("POST /purchase-orders", s.createPurchaseOrder)

	// Edit or delete a subledger document while it is still draft.
	api.HandleFunc("PUT /sales-invoices/{id}", s.updateSalesInvoice)
	api.HandleFunc("DELETE /sales-invoices/{id}", s.deleteSalesInvoice)
	api.HandleFunc("PUT /purchase-bills/{id}", s.updatePurchaseBill)
	api.HandleFunc("DELETE /purchase-bills/{id}", s.deletePurchaseBill)
	api.HandleFunc("PUT /sales-credit-notes/{id}", s.updateSalesCreditNote)
	api.HandleFunc("DELETE /sales-credit-notes/{id}", s.deleteSalesCreditNote)
	api.HandleFunc("PUT /purchase-credit-notes/{id}", s.updatePurchaseCreditNote)
	api.HandleFunc("DELETE /purchase-credit-notes/{id}", s.deletePurchaseCreditNote)
	api.HandleFunc("PUT /sales-orders/{id}", s.updateSalesOrder)
	api.HandleFunc("DELETE /sales-orders/{id}", s.deleteSalesOrder)
	api.HandleFunc("PUT /purchase-orders/{id}", s.updatePurchaseOrder)
	api.HandleFunc("DELETE /purchase-orders/{id}", s.deletePurchaseOrder)
	api.HandleFunc("PUT /customer-payments/{id}", s.updateCustomerPayment)
	api.HandleFunc("DELETE /customer-payments/{id}", s.deleteCustomerPayment)
	api.HandleFunc("PUT /supplier-payments/{id}", s.updateSupplierPayment)
	api.HandleFunc("DELETE /supplier-payments/{id}", s.deleteSupplierPayment)
	api.HandleFunc("PUT /stock-movements/{id}", s.updateStockMovement)
	api.HandleFunc("DELETE /stock-movements/{id}", s.deleteStockMovement)

	// Order lifecycle transitions (draft -> open -> closed, or cancelled).
	api.HandleFunc("POST /sales-orders/{id}/confirm", s.confirmSalesOrder)
	api.HandleFunc("POST /sales-orders/{id}/close", s.closeSalesOrder)
	api.HandleFunc("POST /sales-orders/{id}/cancel", s.cancelSalesOrder)
	api.HandleFunc("POST /purchase-orders/{id}/confirm", s.confirmPurchaseOrder)
	api.HandleFunc("POST /purchase-orders/{id}/close", s.closePurchaseOrder)
	api.HandleFunc("POST /purchase-orders/{id}/cancel", s.cancelPurchaseOrder)

	// Order fulfilment: create the subledger document that draws down the order.
	api.HandleFunc("POST /sales-orders/{id}/invoice", s.invoiceSalesOrder)
	api.HandleFunc("POST /sales-orders/{id}/ship", s.shipSalesOrder)
	api.HandleFunc("POST /purchase-orders/{id}/bill", s.billPurchaseOrder)
	api.HandleFunc("POST /purchase-orders/{id}/receive", s.receivePurchaseOrder)

	// Post a draft subledger document to the general ledger.
	api.HandleFunc("POST /sales-invoices/{id}/post", s.postSalesInvoice)
	api.HandleFunc("POST /purchase-bills/{id}/post", s.postPurchaseBill)
	api.HandleFunc("POST /sales-credit-notes/{id}/post", s.postSalesCreditNote)
	api.HandleFunc("POST /purchase-credit-notes/{id}/post", s.postPurchaseCreditNote)
	api.HandleFunc("POST /customer-payments/{id}/post", s.postCustomerPayment)
	api.HandleFunc("POST /supplier-payments/{id}/post", s.postSupplierPayment)
	api.HandleFunc("POST /stock-movements/{id}/post", s.postStockMovement)

	// Email a printable document to its counterparty as a PDF attachment.
	// Inert unless SMTP is configured: without it these report 501 "not
	// configured" and send nothing.
	api.HandleFunc("POST /sales-invoices/{id}/email", s.emailHandler("invoice", "Invoice", printing.SalesInvoicePDF, printing.SalesInvoiceRecipient))
	api.HandleFunc("POST /purchase-bills/{id}/email", s.emailHandler("bill", "Bill", printing.PurchaseBillPDF, printing.PurchaseBillRecipient))
	api.HandleFunc("POST /sales-credit-notes/{id}/email", s.emailHandler("credit-note", "Credit Note", printing.SalesCreditNotePDF, printing.SalesCreditNoteRecipient))
	api.HandleFunc("POST /purchase-credit-notes/{id}/email", s.emailHandler("supplier-credit", "Credit Note", printing.PurchaseCreditNotePDF, printing.PurchaseCreditNoteRecipient))
	api.HandleFunc("POST /sales-orders/{id}/email", s.emailHandler("sales-order", "Sales Order", printing.SalesOrderPDF, printing.SalesOrderRecipient))
	api.HandleFunc("POST /purchase-orders/{id}/email", s.emailHandler("purchase-order", "Purchase Order", printing.PurchaseOrderPDF, printing.PurchaseOrderRecipient))

	// Auto-apply a payment or credit note to the counterparty's open
	// documents, oldest first.
	api.HandleFunc("POST /customer-payments/{id}/apply", s.applyCustomerPayment)
	api.HandleFunc("POST /supplier-payments/{id}/apply", s.applySupplierPayment)
	api.HandleFunc("POST /sales-credit-notes/{id}/apply", s.applySalesCreditNote)
	api.HandleFunc("POST /purchase-credit-notes/{id}/apply", s.applyPurchaseCreditNote)

	// Unpost a document: reverse its journal entry and return it to draft.
	// Admin-only — it is the escape hatch that rewrites posted history.
	api.HandleFunc("POST /sales-invoices/{id}/unpost", s.admin(s.unpostSalesInvoice))
	api.HandleFunc("POST /purchase-bills/{id}/unpost", s.admin(s.unpostPurchaseBill))
	api.HandleFunc("POST /sales-credit-notes/{id}/unpost", s.admin(s.unpostSalesCreditNote))
	api.HandleFunc("POST /purchase-credit-notes/{id}/unpost", s.admin(s.unpostPurchaseCreditNote))
	api.HandleFunc("POST /customer-payments/{id}/unpost", s.admin(s.unpostCustomerPayment))
	api.HandleFunc("POST /supplier-payments/{id}/unpost", s.admin(s.unpostSupplierPayment))
	api.HandleFunc("POST /stock-movements/{id}/unpost", s.admin(s.unpostStockMovement))

	// Year-end close: sweep revenue/expense into retained earnings and lock
	// the year; reopen undoes it. Admin-only — like unpost, these rewrite
	// posted history.
	api.HandleFunc("POST /fiscal-years/{id}/close", s.admin(s.closeFiscalYear))
	api.HandleFunc("POST /fiscal-years/{id}/reopen", s.admin(s.reopenFiscalYear))

	// Bank reconciliation: capture a statement for a cash account (manually
	// or by CSV import), match its lines against posted journal lines, and
	// reconcile it. Reopen is admin-only — like unpost, it unwinds a
	// finalized state.
	api.HandleFunc("POST /bank-statements", s.createBankStatement)
	api.HandleFunc("GET /bank-statements", s.listBankStatements)
	api.HandleFunc("GET /bank-statements/{id}", s.getBankStatement)
	api.HandleFunc("PUT /bank-statements/{id}", s.updateBankStatement)
	api.HandleFunc("DELETE /bank-statements/{id}", s.deleteBankStatement)
	api.HandleFunc("GET /bank-statements/{id}/lines", s.getBankStatementLines)
	api.HandleFunc("POST /bank-statements/{id}/lines", s.addBankStatementLine)
	api.HandleFunc("GET /bank-statements/{id}/candidates", s.getBankMatchCandidates)
	api.HandleFunc("POST /bank-statements/{id}/import", s.importBankStatement)
	api.HandleFunc("POST /bank-statements/{id}/auto-match", s.autoMatchBankStatement)
	api.HandleFunc("POST /bank-statements/{id}/reconcile", s.reconcileBankStatement)
	api.HandleFunc("POST /bank-statements/{id}/reopen", s.admin(s.reopenBankStatement))
	api.HandleFunc("POST /bank-statement-lines/{id}/match", s.matchBankStatementLine)
	api.HandleFunc("POST /bank-statement-lines/{id}/unmatch", s.unmatchBankStatementLine)
	api.HandleFunc("DELETE /bank-statement-lines/{id}", s.deleteBankStatementLine)

	// Master data CRUD.
	s.registerMasterRoutes(api)

	// Read / reporting.
	api.HandleFunc("GET /stock-movements", s.listStockMovements)
	api.HandleFunc("GET /stock-movements/{id}", s.getStockMovement)
	api.HandleFunc("GET /trial-balance", s.getTrialBalance)
	api.HandleFunc("GET /profit-and-loss", s.getProfitAndLoss)
	api.HandleFunc("GET /balance-sheet", s.getBalanceSheet)
	api.HandleFunc("GET /cash-flow", s.getCashFlow)
	api.HandleFunc("GET /accounts/{id}/ledger", s.getAccountLedger)
	api.HandleFunc("GET /journal-entries/{id}", s.getJournalEntry)
	api.HandleFunc("GET /ar-aging", s.getARaging)
	api.HandleFunc("GET /ap-aging", s.getAPaging)
	api.HandleFunc("GET /inventory/valuation", s.getInventoryValuation)
	api.HandleFunc("GET /sales-invoices", s.listSalesInvoices)
	api.HandleFunc("GET /sales-invoices/{id}", s.getSalesInvoice)
	api.HandleFunc("GET /sales-invoices/{id}/lines", s.getSalesInvoiceLines)
	api.HandleFunc("GET /sales-invoices/{id}/pdf", s.pdfHandler("invoice", printing.SalesInvoicePDF))
	api.HandleFunc("GET /customer-payments", s.listCustomerPayments)
	api.HandleFunc("GET /customer-payments/{id}", s.getCustomerPayment)
	api.HandleFunc("GET /customer-payments/{id}/applications", s.getCustomerPaymentApplications)
	api.HandleFunc("GET /supplier-payments", s.listSupplierPayments)
	api.HandleFunc("GET /supplier-payments/{id}", s.getSupplierPayment)
	api.HandleFunc("GET /supplier-payments/{id}/applications", s.getSupplierPaymentApplications)
	api.HandleFunc("GET /purchase-bills", s.listPurchaseBills)
	api.HandleFunc("GET /purchase-bills/{id}", s.getPurchaseBill)
	api.HandleFunc("GET /purchase-bills/{id}/lines", s.getPurchaseBillLines)
	api.HandleFunc("GET /purchase-bills/{id}/pdf", s.pdfHandler("bill", printing.PurchaseBillPDF))
	api.HandleFunc("GET /sales-credit-notes", s.listSalesCreditNotes)
	api.HandleFunc("GET /sales-credit-notes/{id}", s.getSalesCreditNote)
	api.HandleFunc("GET /sales-credit-notes/{id}/lines", s.getSalesCreditNoteLines)
	api.HandleFunc("GET /sales-credit-notes/{id}/applications", s.getSalesCreditNoteApplications)
	api.HandleFunc("GET /sales-credit-notes/{id}/pdf", s.pdfHandler("credit-note", printing.SalesCreditNotePDF))
	api.HandleFunc("GET /purchase-credit-notes", s.listPurchaseCreditNotes)
	api.HandleFunc("GET /purchase-credit-notes/{id}", s.getPurchaseCreditNote)
	api.HandleFunc("GET /purchase-credit-notes/{id}/lines", s.getPurchaseCreditNoteLines)
	api.HandleFunc("GET /purchase-credit-notes/{id}/applications", s.getPurchaseCreditNoteApplications)
	api.HandleFunc("GET /purchase-credit-notes/{id}/pdf", s.pdfHandler("supplier-credit", printing.PurchaseCreditNotePDF))
	api.HandleFunc("GET /sales-orders", s.listSalesOrders)
	api.HandleFunc("GET /sales-orders/{id}", s.getSalesOrder)
	api.HandleFunc("GET /sales-orders/{id}/lines", s.getSalesOrderLines)
	api.HandleFunc("GET /sales-orders/{id}/pdf", s.pdfHandler("sales-order", printing.SalesOrderPDF))
	api.HandleFunc("GET /purchase-orders", s.listPurchaseOrders)
	api.HandleFunc("GET /purchase-orders/{id}", s.getPurchaseOrder)
	api.HandleFunc("GET /purchase-orders/{id}/lines", s.getPurchaseOrderLines)
	api.HandleFunc("GET /purchase-orders/{id}/pdf", s.pdfHandler("purchase-order", printing.PurchaseOrderPDF))

	// User administration (admins only).
	api.HandleFunc("GET /users", s.admin(s.listUsers))
	api.HandleFunc("POST /users", s.admin(s.createUser))
	api.HandleFunc("GET /users/{id}", s.admin(s.getUser))
	api.HandleFunc("PUT /users/{id}", s.admin(s.updateUser))
	api.HandleFunc("POST /users/{id}/password", s.admin(s.setUserPassword))

	// Who am I (the SPA's session probe).
	api.HandleFunc("GET /auth/me", s.me)

	// Everything above requires a session. Login mints one; logout is public
	// too so an expired session can still clear its cookie idempotently.
	public := http.NewServeMux()
	public.HandleFunc("POST /auth/login", s.login)
	public.HandleFunc("POST /auth/logout", s.logout)
	public.Handle("/", s.requireAuth(api))

	mux := http.NewServeMux()

	// Liveness/readiness probes stay at the root for load balancers/orchestrators.
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

	// JSON API under /api/; everything else falls through to the embedded SPA.
	// (The SPA shell itself stays public — it renders the login screen.)
	mux.Handle("/api/", http.StripPrefix("/api", public))
	if distFS != nil {
		mux.Handle("/", spaHandler(distFS))
	}

	return mux
}

func (s *Server) listStockMovements(w http.ResponseWriter, r *http.Request) {
	rows, err := reporting.StockMovements(r.Context(), s.pool)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getStockMovement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	m, err := reporting.StockMovementByID(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) getTrialBalance(w http.ResponseWriter, r *http.Request) {
	rows, err := reporting.TrialBalance(r.Context(), s.pool)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// dateParam reads an optional YYYY-MM-DD query parameter. nil means the
// parameter was absent; ok=false means it was malformed and a 400 was written.
func dateParam(w http.ResponseWriter, r *http.Request, name string) (value *string, ok bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return nil, true
	}
	if _, err := time.Parse("2006-01-02", v); err != nil {
		writeError(w, http.StatusBadRequest, name+" must be a YYYY-MM-DD date")
		return nil, false
	}
	return &v, true
}

func (s *Server) getProfitAndLoss(w http.ResponseWriter, r *http.Request) {
	from, ok := dateParam(w, r, "from")
	if !ok {
		return
	}
	to, ok := dateParam(w, r, "to")
	if !ok {
		return
	}
	rows, err := reporting.ProfitAndLoss(r.Context(), s.pool, from, to)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getBalanceSheet(w http.ResponseWriter, r *http.Request) {
	asOf, ok := dateParam(w, r, "as_of")
	if !ok {
		return
	}
	bs, err := reporting.BalanceSheetAsOf(r.Context(), s.pool, asOf)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bs)
}

func (s *Server) getCashFlow(w http.ResponseWriter, r *http.Request) {
	from, ok := dateParam(w, r, "from")
	if !ok {
		return
	}
	to, ok := dateParam(w, r, "to")
	if !ok {
		return
	}
	cf, err := reporting.CashFlowStatement(r.Context(), s.pool, from, to)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cf)
}

func (s *Server) getAccountLedger(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	from, ok := dateParam(w, r, "from")
	if !ok {
		return
	}
	to, ok := dateParam(w, r, "to")
	if !ok {
		return
	}
	rows, err := reporting.AccountLedger(r.Context(), s.pool, id, from, to)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getJournalEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	e, err := reporting.JournalEntryByID(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) getARaging(w http.ResponseWriter, r *http.Request) {
	rows, err := reporting.ARaging(r.Context(), s.pool)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getAPaging(w http.ResponseWriter, r *http.Request) {
	rows, err := reporting.APaging(r.Context(), s.pool)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getInventoryValuation(w http.ResponseWriter, r *http.Request) {
	rows, err := reporting.StockValuation(r.Context(), s.pool)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) listSalesInvoices(w http.ResponseWriter, r *http.Request) {
	rows, err := reporting.SalesInvoiceBalances(r.Context(), s.pool)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getSalesInvoiceLines(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	lines, err := reporting.SalesInvoiceLines(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

func (s *Server) getSalesInvoice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	inv, err := reporting.SalesInvoiceBalance(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

// pdfHandler serves a printable document: render produces the bytes and the
// document number, prefix names the download file.
func (s *Server) pdfHandler(prefix string, render func(context.Context, reporting.Querier, int) ([]byte, string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		out, number, err := render(r.Context(), s.pool, id)
		if err != nil {
			s.writeReadError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `inline; filename="`+pdfFilename(prefix, number)+`"`)
		_, _ = w.Write(out)
	}
}

// emailHandler emails a printable document to its counterparty as a PDF
// attachment. render produces the same bytes as the PDF endpoint; recipient
// resolves the counterparty organization's email; prefix names the attachment
// file and label is the human document name for the subject.
//
// Recipients come from an optional {"to": [...]} request body; when it is
// empty they fall back to the counterparty's organizations.email address. A
// document whose counterparty has no email on file and no explicit "to" is a
// 422. When no mailer is configured the send reports ErrNotConfigured, so this
// stays inert on the demo.
func (s *Server) emailHandler(prefix, label string, render func(context.Context, reporting.Querier, int) ([]byte, string, error), recipient func(context.Context, reporting.Querier, int) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var body struct {
			To []string `json:"to"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		to := body.To
		if len(to) == 0 {
			addr, err := recipient(r.Context(), s.pool, id)
			if err != nil {
				s.writeReadError(w, err)
				return
			}
			if addr == "" {
				writeError(w, http.StatusUnprocessableEntity,
					"this counterparty has no email address on file; supply a recipient or set one on the organization")
				return
			}
			to = []string{addr}
		}
		out, number, err := render(r.Context(), s.pool, id)
		if err != nil {
			s.writeReadError(w, err)
			return
		}
		msg := mailer.Message{
			To:      to,
			Subject: fmt.Sprintf("%s %s", label, number),
			Body:    fmt.Sprintf("Please find attached %s %s.", strings.ToLower(label), number),
			Attachments: []mailer.Attachment{{
				Filename:    pdfFilename(prefix, number),
				ContentType: "application/pdf",
				Data:        out,
			}},
		}
		if err := s.mailer.Send(r.Context(), msg); err != nil {
			s.writeMailError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "sent", "to": to})
	}
}

// writeMailError maps a mailer error to an HTTP response. A not-configured
// mailer is a 501, not a server fault: the endpoint exists but sending is off.
func (s *Server) writeMailError(w http.ResponseWriter, err error) {
	if errors.Is(err, mailer.ErrNotConfigured) {
		writeError(w, http.StatusNotImplemented, "email sending is not configured")
		return
	}
	s.log.Error("email send failed", "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// pdfFilename derives a safe download filename from a document number.
func pdfFilename(prefix, number string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			return r
		default:
			return '-'
		}
	}, number)
	return prefix + "-" + safe + ".pdf"
}

func (s *Server) listCustomerPayments(w http.ResponseWriter, r *http.Request) {
	rows, err := reporting.CustomerPayments(r.Context(), s.pool)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getCustomerPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := reporting.CustomerPayment(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) getCustomerPaymentApplications(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	apps, err := reporting.CustomerPaymentApplications(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apps)
}

func (s *Server) listSupplierPayments(w http.ResponseWriter, r *http.Request) {
	rows, err := reporting.SupplierPayments(r.Context(), s.pool)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getSupplierPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := reporting.SupplierPayment(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) getSupplierPaymentApplications(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	apps, err := reporting.SupplierPaymentApplications(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apps)
}

func (s *Server) listPurchaseBills(w http.ResponseWriter, r *http.Request) {
	rows, err := reporting.PurchaseBillBalances(r.Context(), s.pool)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getPurchaseBillLines(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	lines, err := reporting.PurchaseBillLines(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

func (s *Server) getPurchaseBill(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	bill, err := reporting.PurchaseBillBalance(r.Context(), s.pool, id)
	if err != nil {
		s.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bill)
}

func (s *Server) writeReadError(w http.ResponseWriter, err error) {
	if errors.Is(err, reporting.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	s.log.Error("query failed", "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func (s *Server) createSalesInvoice(w http.ResponseWriter, r *http.Request) {
	var in documents.SalesInvoiceInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runCreate(w, r, in.Validate, func(tx pgx.Tx) (int, error) {
		return documents.CreateSalesInvoice(r.Context(), tx, in)
	})
}

func (s *Server) createPurchaseBill(w http.ResponseWriter, r *http.Request) {
	var in documents.PurchaseBillInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runCreate(w, r, in.Validate, func(tx pgx.Tx) (int, error) {
		return documents.CreatePurchaseBill(r.Context(), tx, in)
	})
}

func (s *Server) createCustomerPayment(w http.ResponseWriter, r *http.Request) {
	var in documents.CustomerPaymentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runCreate(w, r, in.Validate, func(tx pgx.Tx) (int, error) {
		return documents.CreateCustomerPayment(r.Context(), tx, in)
	})
}

func (s *Server) createSupplierPayment(w http.ResponseWriter, r *http.Request) {
	var in documents.SupplierPaymentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runCreate(w, r, in.Validate, func(tx pgx.Tx) (int, error) {
		return documents.CreateSupplierPayment(r.Context(), tx, in)
	})
}

func (s *Server) createStockMovement(w http.ResponseWriter, r *http.Request) {
	var in documents.StockMovementInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.runCreate(w, r, in.Validate, func(tx pgx.Tx) (int, error) {
		return documents.CreateStockMovement(r.Context(), tx, in)
	})
}

// runCreate validates the request, creates the document in a transaction, and
// writes 201 with its id.
func (s *Server) runCreate(w http.ResponseWriter, r *http.Request, validate func() string, create func(pgx.Tx) (int, error)) {
	if msg := validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	var id int
	err := db.WithTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		var e error
		id, e = create(tx)
		return e
	})
	if err != nil {
		s.writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

// writeCreateError maps database constraint violations to client errors.
func (s *Server) writeCreateError(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			writeError(w, http.StatusConflict, pgErr.Message)
			return
		case "23503", "23514", "23502", "22P02", "23P01", "P0001":
			// foreign key, check, not-null, invalid text, exclusion, raised
			writeError(w, http.StatusUnprocessableEntity, pgErr.Message)
			return
		}
	}
	s.log.Error("create failed", "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func (s *Server) unpostSalesInvoice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runReverse(w, r, func(tx pgx.Tx) (int, error) {
		return posting.UnpostSalesInvoice(r.Context(), tx, id)
	})
}

func (s *Server) unpostPurchaseBill(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runReverse(w, r, func(tx pgx.Tx) (int, error) {
		return posting.UnpostPurchaseBill(r.Context(), tx, id)
	})
}

func (s *Server) unpostCustomerPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runReverse(w, r, func(tx pgx.Tx) (int, error) {
		return posting.UnpostCustomerPayment(r.Context(), tx, id)
	})
}

func (s *Server) unpostSupplierPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runReverse(w, r, func(tx pgx.Tx) (int, error) {
		return posting.UnpostSupplierPayment(r.Context(), tx, id)
	})
}

func (s *Server) unpostStockMovement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.runReverse(w, r, func(tx pgx.Tx) (int, error) {
		return posting.UnpostStockMovement(r.Context(), tx, id)
	})
}

// runReverse executes an unpost inside a transaction and writes the id of the
// reversing journal entry, or an appropriate error response.
func (s *Server) runReverse(w http.ResponseWriter, r *http.Request, reverse func(pgx.Tx) (int, error)) {
	var reversalID int
	err := db.WithTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		var e error
		reversalID, e = reverse(tx)
		return e
	})
	if err != nil {
		s.writePostingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"reversal_entry_id": reversalID})
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
	// Stock movements carry no currency of their own and post in the base
	// currency. Receipts also need a credit (clearing) account.
	var body struct {
		CreditAccountID int `json:"credit_account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	s.runPost(w, r, func(tx pgx.Tx) (int, error) {
		return posting.PostStockMovement(r.Context(), tx, id, body.CreditAccountID)
	})
}

func (s *Server) closeFiscalYear(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		RetainedEarningsAccountID int `json:"retained_earnings_account_id"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.RetainedEarningsAccountID <= 0 {
		writeError(w, http.StatusBadRequest, "retained_earnings_account_id is required")
		return
	}
	var res posting.CloseResult
	err := db.WithTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		var e error
		res, e = posting.CloseFiscalYear(r.Context(), tx, id, body.RetainedEarningsAccountID)
		return e
	})
	if err != nil {
		s.writePostingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]*int{
		"closing_entry_id":    nilIfZero(res.ClosingEntryID),
		"next_fiscal_year_id": nilIfZero(res.NextFiscalYearID),
	})
}

func (s *Server) reopenFiscalYear(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var reversalID int
	err := db.WithTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		var e error
		reversalID, e = posting.ReopenFiscalYear(r.Context(), tx, id)
		return e
	})
	if err != nil {
		s.writePostingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]*int{"reversal_entry_id": nilIfZero(reversalID)})
}

// nilIfZero renders "nothing was produced" ids as JSON null.
func nilIfZero(id int) *int {
	if id == 0 {
		return nil
	}
	return &id
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
	case errors.Is(err, posting.ErrNotDraft),
		errors.Is(err, posting.ErrNotPosted),
		errors.Is(err, posting.ErrAlreadyPosted),
		errors.Is(err, posting.ErrAlreadyReversed),
		errors.Is(err, posting.ErrHasApplications),
		errors.Is(err, posting.ErrBankMatched),
		errors.Is(err, posting.ErrYearNotOpen),
		errors.Is(err, posting.ErrYearNotClosed):
		return http.StatusConflict
	case errors.Is(err, posting.ErrNotPostable),
		errors.Is(err, posting.ErrNoOpenPeriod),
		errors.Is(err, posting.ErrMissingAccount),
		errors.Is(err, posting.ErrNothingToPost),
		errors.Is(err, posting.ErrPriorYearOpen),
		errors.Is(err, posting.ErrLaterYearClosed):
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
