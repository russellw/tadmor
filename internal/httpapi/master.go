package httpapi

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"

	"tadmor/internal/master"
)

// registerMasterRoutes wires CRUD endpoints for master data. Updates use PUT
// (full replace) and return 204; creates return 201 with the new id.
func (s *Server) registerMasterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /organizations", s.listOrganizations)
	mux.HandleFunc("POST /organizations", s.createOrganization)
	mux.HandleFunc("GET /organizations/{id}", s.getOrganization)
	mux.HandleFunc("PUT /organizations/{id}", s.updateOrganization)

	mux.HandleFunc("GET /customers", s.listCustomers)
	mux.HandleFunc("POST /customers", s.createCustomer)
	mux.HandleFunc("GET /customers/{id}", s.getCustomer)
	mux.HandleFunc("PUT /customers/{id}", s.updateCustomer)

	mux.HandleFunc("GET /suppliers", s.listSuppliers)
	mux.HandleFunc("POST /suppliers", s.createSupplier)
	mux.HandleFunc("GET /suppliers/{id}", s.getSupplier)
	mux.HandleFunc("PUT /suppliers/{id}", s.updateSupplier)

	mux.HandleFunc("GET /products", s.listProducts)
	mux.HandleFunc("POST /products", s.createProduct)
	mux.HandleFunc("GET /products/{id}", s.getProduct)
	mux.HandleFunc("PUT /products/{id}", s.updateProduct)

	mux.HandleFunc("GET /accounts", s.listAccounts)
	mux.HandleFunc("POST /accounts", s.createAccount)
	mux.HandleFunc("GET /accounts/{id}", s.getAccount)
	mux.HandleFunc("PUT /accounts/{id}", s.updateAccount)

	mux.HandleFunc("GET /tax-codes", s.listTaxCodes)
	mux.HandleFunc("POST /tax-codes", s.createTaxCode)
	mux.HandleFunc("GET /tax-codes/{code}", s.getTaxCode)
	mux.HandleFunc("PUT /tax-codes/{code}", s.updateTaxCode)

	mux.HandleFunc("GET /payment-terms", s.listPaymentTerms)
	mux.HandleFunc("POST /payment-terms", s.createPaymentTerm)
	mux.HandleFunc("GET /payment-terms/{code}", s.getPaymentTerm)
	mux.HandleFunc("PUT /payment-terms/{code}", s.updatePaymentTerm)

	mux.HandleFunc("GET /warehouses", s.listWarehouses)
	mux.HandleFunc("POST /warehouses", s.createWarehouse)
	mux.HandleFunc("GET /warehouses/{id}", s.getWarehouse)
	mux.HandleFunc("PUT /warehouses/{id}", s.updateWarehouse)

	mux.HandleFunc("GET /fiscal-years", s.listFiscalYears)
	mux.HandleFunc("POST /fiscal-years", s.createFiscalYear)
	mux.HandleFunc("GET /fiscal-years/{id}", s.getFiscalYear)
	mux.HandleFunc("PUT /fiscal-years/{id}", s.updateFiscalYear)

	mux.HandleFunc("GET /accounting-periods", s.listAccountingPeriods)
	mux.HandleFunc("POST /accounting-periods", s.createAccountingPeriod)
	mux.HandleFunc("GET /accounting-periods/{id}", s.getAccountingPeriod)
	mux.HandleFunc("PUT /accounting-periods/{id}", s.updateAccountingPeriod)

	// Ledger settings (base currency + FX account). Changing them rewrites how
	// every report reads, so the update is admin-only, like unpost.
	mux.HandleFunc("GET /settings", s.getSettings)
	mux.HandleFunc("PUT /settings", s.admin(s.updateSettings))

	// Exchange rates, keyed by currency + date. Any authenticated user may
	// maintain them; posting reads the latest on or before the document date.
	mux.HandleFunc("GET /exchange-rates", s.listExchangeRates)
	mux.HandleFunc("POST /exchange-rates", s.createExchangeRate)
	mux.HandleFunc("PUT /exchange-rates/{currency}/{date}", s.updateExchangeRate)
	mux.HandleFunc("DELETE /exchange-rates/{currency}/{date}", s.deleteExchangeRate)
}

// ---- ledger settings (singleton) ----

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	v, err := master.GetSettings(r.Context(), s.pool)
	s.okJSON(w, v, err)
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var in master.Settings
	if !decodeValid(w, r, &in) {
		return
	}
	s.updated(w, master.UpdateSettings(r.Context(), s.pool, in))
}

// ---- exchange rates (natural key: currency + date) ----

func (s *Server) listExchangeRates(w http.ResponseWriter, r *http.Request) {
	v, err := master.ListExchangeRates(r.Context(), s.pool)
	s.okJSON(w, v, err)
}

func (s *Server) createExchangeRate(w http.ResponseWriter, r *http.Request) {
	var in master.ExchangeRateInput
	if !decodeValid(w, r, &in) {
		return
	}
	if err := master.CreateExchangeRate(r.Context(), s.pool, in); err != nil {
		s.writeMasterError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"currency_code": in.CurrencyCode, "rate_date": in.RateDate})
}

func (s *Server) updateExchangeRate(w http.ResponseWriter, r *http.Request) {
	currency, date := r.PathValue("currency"), r.PathValue("date")
	var in master.ExchangeRateInput
	in.CurrencyCode, in.RateDate = currency, date
	if !decodeJSON(w, r, &in) {
		return
	}
	in.CurrencyCode, in.RateDate = currency, date // path wins over body
	if msg := in.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	s.updated(w, master.UpdateExchangeRate(r.Context(), s.pool, currency, date, in))
}

func (s *Server) deleteExchangeRate(w http.ResponseWriter, r *http.Request) {
	s.updated(w, master.DeleteExchangeRate(r.Context(), s.pool,
		r.PathValue("currency"), r.PathValue("date")))
}

// ---- response helpers ----

type validator interface{ Validate() string }

// decodeValid decodes the body into in and validates it; on failure it writes
// the response and returns false.
func decodeValid[T validator](w http.ResponseWriter, r *http.Request, in *T) bool {
	if !decodeJSON(w, r, in) {
		return false
	}
	if msg := (*in).Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return false
	}
	return true
}

func (s *Server) okJSON(w http.ResponseWriter, v any, err error) {
	if err != nil {
		s.writeMasterError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) created(w http.ResponseWriter, id int, err error) {
	if err != nil {
		s.writeMasterError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

func (s *Server) createdKey(w http.ResponseWriter, key string, err error) {
	if err != nil {
		s.writeMasterError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"code": key})
}

func (s *Server) updated(w http.ResponseWriter, err error) {
	if err != nil {
		s.writeMasterError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeMasterError(w http.ResponseWriter, err error) {
	if errors.Is(err, master.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, master.ErrInvalid) {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			writeError(w, http.StatusConflict, pgErr.Message)
			return
		case "23503", "23514", "23502", "22P02", "23P01", "P0001":
			writeError(w, http.StatusUnprocessableEntity, pgErr.Message)
			return
		}
	}
	s.log.Error("master data operation failed", "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// ---- organizations ----

func (s *Server) listOrganizations(w http.ResponseWriter, r *http.Request) {
	v, err := master.ListOrganizations(r.Context(), s.pool)
	s.okJSON(w, v, err)
}

func (s *Server) getOrganization(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := master.GetOrganization(r.Context(), s.pool, id)
	s.okJSON(w, v, err)
}

func (s *Server) createOrganization(w http.ResponseWriter, r *http.Request) {
	var in master.OrganizationInput
	if !decodeValid(w, r, &in) {
		return
	}
	id, err := master.CreateOrganization(r.Context(), s.pool, in)
	s.created(w, id, err)
}

func (s *Server) updateOrganization(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in master.OrganizationInput
	if !decodeValid(w, r, &in) {
		return
	}
	s.updated(w, master.UpdateOrganization(r.Context(), s.pool, id, in))
}

// ---- customers ----

func (s *Server) listCustomers(w http.ResponseWriter, r *http.Request) {
	v, err := master.ListCustomers(r.Context(), s.pool)
	s.okJSON(w, v, err)
}

func (s *Server) getCustomer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := master.GetCustomer(r.Context(), s.pool, id)
	s.okJSON(w, v, err)
}

func (s *Server) createCustomer(w http.ResponseWriter, r *http.Request) {
	var in master.CustomerInput
	if !decodeValid(w, r, &in) {
		return
	}
	id, err := master.CreateCustomer(r.Context(), s.pool, in)
	s.created(w, id, err)
}

func (s *Server) updateCustomer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in master.CustomerInput
	if !decodeValid(w, r, &in) {
		return
	}
	s.updated(w, master.UpdateCustomer(r.Context(), s.pool, id, in))
}

// ---- suppliers ----

func (s *Server) listSuppliers(w http.ResponseWriter, r *http.Request) {
	v, err := master.ListSuppliers(r.Context(), s.pool)
	s.okJSON(w, v, err)
}

func (s *Server) getSupplier(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := master.GetSupplier(r.Context(), s.pool, id)
	s.okJSON(w, v, err)
}

func (s *Server) createSupplier(w http.ResponseWriter, r *http.Request) {
	var in master.SupplierInput
	if !decodeValid(w, r, &in) {
		return
	}
	id, err := master.CreateSupplier(r.Context(), s.pool, in)
	s.created(w, id, err)
}

func (s *Server) updateSupplier(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in master.SupplierInput
	if !decodeValid(w, r, &in) {
		return
	}
	s.updated(w, master.UpdateSupplier(r.Context(), s.pool, id, in))
}

// ---- products ----

func (s *Server) listProducts(w http.ResponseWriter, r *http.Request) {
	v, err := master.ListProducts(r.Context(), s.pool)
	s.okJSON(w, v, err)
}

func (s *Server) getProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := master.GetProduct(r.Context(), s.pool, id)
	s.okJSON(w, v, err)
}

func (s *Server) createProduct(w http.ResponseWriter, r *http.Request) {
	var in master.ProductInput
	if !decodeValid(w, r, &in) {
		return
	}
	id, err := master.CreateProduct(r.Context(), s.pool, in)
	s.created(w, id, err)
}

func (s *Server) updateProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in master.ProductInput
	if !decodeValid(w, r, &in) {
		return
	}
	s.updated(w, master.UpdateProduct(r.Context(), s.pool, id, in))
}

// ---- accounts ----

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	v, err := master.ListAccounts(r.Context(), s.pool)
	s.okJSON(w, v, err)
}

func (s *Server) getAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := master.GetAccount(r.Context(), s.pool, id)
	s.okJSON(w, v, err)
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var in master.AccountInput
	if !decodeValid(w, r, &in) {
		return
	}
	id, err := master.CreateAccount(r.Context(), s.pool, in)
	s.created(w, id, err)
}

func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in master.AccountInput
	if !decodeValid(w, r, &in) {
		return
	}
	s.updated(w, master.UpdateAccount(r.Context(), s.pool, id, in))
}

// ---- tax codes (natural key: code) ----

func (s *Server) listTaxCodes(w http.ResponseWriter, r *http.Request) {
	v, err := master.ListTaxCodes(r.Context(), s.pool)
	s.okJSON(w, v, err)
}

func (s *Server) getTaxCode(w http.ResponseWriter, r *http.Request) {
	v, err := master.GetTaxCode(r.Context(), s.pool, r.PathValue("code"))
	s.okJSON(w, v, err)
}

func (s *Server) createTaxCode(w http.ResponseWriter, r *http.Request) {
	var in master.TaxCodeInput
	if !decodeValid(w, r, &in) {
		return
	}
	code, err := master.CreateTaxCode(r.Context(), s.pool, in)
	s.createdKey(w, code, err)
}

func (s *Server) updateTaxCode(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	var in master.TaxCodeInput
	in.Code = code
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Code = code // path wins over body
	if msg := in.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	s.updated(w, master.UpdateTaxCode(r.Context(), s.pool, code, in))
}

// ---- payment terms (natural key: code) ----

func (s *Server) listPaymentTerms(w http.ResponseWriter, r *http.Request) {
	v, err := master.ListPaymentTerms(r.Context(), s.pool)
	s.okJSON(w, v, err)
}

func (s *Server) getPaymentTerm(w http.ResponseWriter, r *http.Request) {
	v, err := master.GetPaymentTerm(r.Context(), s.pool, r.PathValue("code"))
	s.okJSON(w, v, err)
}

func (s *Server) createPaymentTerm(w http.ResponseWriter, r *http.Request) {
	var in master.PaymentTermInput
	if !decodeValid(w, r, &in) {
		return
	}
	code, err := master.CreatePaymentTerm(r.Context(), s.pool, in)
	s.createdKey(w, code, err)
}

func (s *Server) updatePaymentTerm(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	var in master.PaymentTermInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Code = code // path wins over body
	if msg := in.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	s.updated(w, master.UpdatePaymentTerm(r.Context(), s.pool, code, in))
}

// ---- warehouses ----

func (s *Server) listWarehouses(w http.ResponseWriter, r *http.Request) {
	v, err := master.ListWarehouses(r.Context(), s.pool)
	s.okJSON(w, v, err)
}

func (s *Server) getWarehouse(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := master.GetWarehouse(r.Context(), s.pool, id)
	s.okJSON(w, v, err)
}

func (s *Server) createWarehouse(w http.ResponseWriter, r *http.Request) {
	var in master.WarehouseInput
	if !decodeValid(w, r, &in) {
		return
	}
	id, err := master.CreateWarehouse(r.Context(), s.pool, in)
	s.created(w, id, err)
}

func (s *Server) updateWarehouse(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in master.WarehouseInput
	if !decodeValid(w, r, &in) {
		return
	}
	s.updated(w, master.UpdateWarehouse(r.Context(), s.pool, id, in))
}

// ---- fiscal years ----

func (s *Server) listFiscalYears(w http.ResponseWriter, r *http.Request) {
	v, err := master.ListFiscalYears(r.Context(), s.pool)
	s.okJSON(w, v, err)
}

func (s *Server) getFiscalYear(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := master.GetFiscalYear(r.Context(), s.pool, id)
	s.okJSON(w, v, err)
}

func (s *Server) createFiscalYear(w http.ResponseWriter, r *http.Request) {
	var in master.FiscalYearInput
	if !decodeValid(w, r, &in) {
		return
	}
	id, err := master.CreateFiscalYear(r.Context(), s.pool, in)
	s.created(w, id, err)
}

func (s *Server) updateFiscalYear(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in master.FiscalYearInput
	if !decodeValid(w, r, &in) {
		return
	}
	s.updated(w, master.UpdateFiscalYear(r.Context(), s.pool, id, in))
}

// ---- accounting periods ----

func (s *Server) listAccountingPeriods(w http.ResponseWriter, r *http.Request) {
	v, err := master.ListAccountingPeriods(r.Context(), s.pool)
	s.okJSON(w, v, err)
}

func (s *Server) getAccountingPeriod(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := master.GetAccountingPeriod(r.Context(), s.pool, id)
	s.okJSON(w, v, err)
}

func (s *Server) createAccountingPeriod(w http.ResponseWriter, r *http.Request) {
	var in master.AccountingPeriodInput
	if !decodeValid(w, r, &in) {
		return
	}
	id, err := master.CreateAccountingPeriod(r.Context(), s.pool, in)
	s.created(w, id, err)
}

func (s *Server) updateAccountingPeriod(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in master.AccountingPeriodInput
	if !decodeValid(w, r, &in) {
		return
	}
	s.updated(w, master.UpdateAccountingPeriod(r.Context(), s.pool, id, in))
}
