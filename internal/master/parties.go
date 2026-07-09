package master

import "context"

// ---------------------------------------------------------------------------
// Organizations
// ---------------------------------------------------------------------------

type Organization struct {
	ID              int     `db:"id" json:"id"`
	Name            string  `db:"name" json:"name"`
	LegalName       *string `db:"legal_name" json:"legal_name"`
	TaxID           *string `db:"tax_id" json:"tax_id"`
	CountryCode     *string `db:"country_code" json:"country_code"`
	DefaultCurrency *string `db:"default_currency" json:"default_currency"`
	Email           *string `db:"email" json:"email"`
	IsSelf          bool    `db:"is_self" json:"is_self"`
}

type OrganizationInput struct {
	Name            string  `json:"name"`
	LegalName       *string `json:"legal_name"`
	TaxID           *string `json:"tax_id"`
	CountryCode     *string `json:"country_code"`
	DefaultCurrency *string `json:"default_currency"`
	Email           *string `json:"email"`
	IsSelf          bool    `json:"is_self"`
}

func (in OrganizationInput) Validate() string {
	if in.Name == "" {
		return "name is required"
	}
	return ""
}

const organizationColumns = `id, name, legal_name, tax_id, country_code, default_currency, email, is_self`

func ListOrganizations(ctx context.Context, q Querier) ([]Organization, error) {
	return collectList[Organization](ctx, q, `SELECT `+organizationColumns+` FROM organizations ORDER BY name`)
}

func GetOrganization(ctx context.Context, q Querier, id int) (Organization, error) {
	return collectOne[Organization](ctx, q, `SELECT `+organizationColumns+` FROM organizations WHERE id = $1`, id)
}

func CreateOrganization(ctx context.Context, q Querier, in OrganizationInput) (int, error) {
	var id int
	err := q.QueryRow(ctx,
		`INSERT INTO organizations (name, legal_name, tax_id, country_code, default_currency, email, is_self)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		in.Name, in.LegalName, in.TaxID, in.CountryCode, in.DefaultCurrency, in.Email, in.IsSelf).Scan(&id)
	return id, err
}

func UpdateOrganization(ctx context.Context, q Querier, id int, in OrganizationInput) error {
	return affected(q.Exec(ctx,
		`UPDATE organizations SET name=$2, legal_name=$3, tax_id=$4, country_code=$5, default_currency=$6, email=$7, is_self=$8 WHERE id=$1`,
		id, in.Name, in.LegalName, in.TaxID, in.CountryCode, in.DefaultCurrency, in.Email, in.IsSelf))
}

// ---------------------------------------------------------------------------
// Customers (a role on an organization)
// ---------------------------------------------------------------------------

type Customer struct {
	ID               int     `db:"id" json:"id"`
	OrganizationID   int     `db:"organization_id" json:"organization_id"`
	CustomerNumber   *string `db:"customer_number" json:"customer_number"`
	ARAccountID      *int    `db:"ar_account_id" json:"ar_account_id"`
	PaymentTermsCode *string `db:"payment_terms_code" json:"payment_terms_code"`
	CurrencyCode     *string `db:"currency_code" json:"currency_code"`
	TaxCode          *string `db:"tax_code" json:"tax_code"`
	CreditLimit      *string `db:"credit_limit" json:"credit_limit"`
	IsActive         bool    `db:"is_active" json:"is_active"`
}

type CustomerInput struct {
	OrganizationID   int     `json:"organization_id"`
	CustomerNumber   *string `json:"customer_number"`
	ARAccountID      *int    `json:"ar_account_id"`
	PaymentTermsCode *string `json:"payment_terms_code"`
	CurrencyCode     *string `json:"currency_code"`
	TaxCode          *string `json:"tax_code"`
	CreditLimit      *string `json:"credit_limit"` // decimal or null
	IsActive         bool    `json:"is_active"`
}

func (in CustomerInput) Validate() string {
	if in.OrganizationID <= 0 {
		return "organization_id is required"
	}
	return ""
}

const customerColumns = `id, organization_id, customer_number, ar_account_id, payment_terms_code,
	currency_code, tax_code, credit_limit::text AS credit_limit, is_active`

func ListCustomers(ctx context.Context, q Querier) ([]Customer, error) {
	return collectList[Customer](ctx, q, `SELECT `+customerColumns+` FROM customers ORDER BY id`)
}

func GetCustomer(ctx context.Context, q Querier, id int) (Customer, error) {
	return collectOne[Customer](ctx, q, `SELECT `+customerColumns+` FROM customers WHERE id = $1`, id)
}

func CreateCustomer(ctx context.Context, q Querier, in CustomerInput) (int, error) {
	var id int
	err := q.QueryRow(ctx,
		`INSERT INTO customers (organization_id, customer_number, ar_account_id, payment_terms_code,
		     currency_code, tax_code, credit_limit)
		 VALUES ($1,$2,$3,$4,$5,$6,$7::numeric) RETURNING id`,
		in.OrganizationID, in.CustomerNumber, in.ARAccountID, in.PaymentTermsCode,
		in.CurrencyCode, in.TaxCode, in.CreditLimit).Scan(&id)
	return id, err
}

func UpdateCustomer(ctx context.Context, q Querier, id int, in CustomerInput) error {
	return affected(q.Exec(ctx,
		`UPDATE customers SET organization_id=$2, customer_number=$3, ar_account_id=$4,
		     payment_terms_code=$5, currency_code=$6, tax_code=$7, credit_limit=$8::numeric, is_active=$9
		 WHERE id=$1`,
		id, in.OrganizationID, in.CustomerNumber, in.ARAccountID, in.PaymentTermsCode,
		in.CurrencyCode, in.TaxCode, in.CreditLimit, in.IsActive))
}

// ---------------------------------------------------------------------------
// Suppliers (a role on an organization)
// ---------------------------------------------------------------------------

type Supplier struct {
	ID               int     `db:"id" json:"id"`
	OrganizationID   int     `db:"organization_id" json:"organization_id"`
	SupplierNumber   *string `db:"supplier_number" json:"supplier_number"`
	APAccountID      *int    `db:"ap_account_id" json:"ap_account_id"`
	PaymentTermsCode *string `db:"payment_terms_code" json:"payment_terms_code"`
	CurrencyCode     *string `db:"currency_code" json:"currency_code"`
	TaxCode          *string `db:"tax_code" json:"tax_code"`
	IsActive         bool    `db:"is_active" json:"is_active"`
}

type SupplierInput struct {
	OrganizationID   int     `json:"organization_id"`
	SupplierNumber   *string `json:"supplier_number"`
	APAccountID      *int    `json:"ap_account_id"`
	PaymentTermsCode *string `json:"payment_terms_code"`
	CurrencyCode     *string `json:"currency_code"`
	TaxCode          *string `json:"tax_code"`
	IsActive         bool    `json:"is_active"`
}

func (in SupplierInput) Validate() string {
	if in.OrganizationID <= 0 {
		return "organization_id is required"
	}
	return ""
}

const supplierColumns = `id, organization_id, supplier_number, ap_account_id, payment_terms_code,
	currency_code, tax_code, is_active`

func ListSuppliers(ctx context.Context, q Querier) ([]Supplier, error) {
	return collectList[Supplier](ctx, q, `SELECT `+supplierColumns+` FROM suppliers ORDER BY id`)
}

func GetSupplier(ctx context.Context, q Querier, id int) (Supplier, error) {
	return collectOne[Supplier](ctx, q, `SELECT `+supplierColumns+` FROM suppliers WHERE id = $1`, id)
}

func CreateSupplier(ctx context.Context, q Querier, in SupplierInput) (int, error) {
	var id int
	err := q.QueryRow(ctx,
		`INSERT INTO suppliers (organization_id, supplier_number, ap_account_id, payment_terms_code,
		     currency_code, tax_code)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		in.OrganizationID, in.SupplierNumber, in.APAccountID, in.PaymentTermsCode,
		in.CurrencyCode, in.TaxCode).Scan(&id)
	return id, err
}

func UpdateSupplier(ctx context.Context, q Querier, id int, in SupplierInput) error {
	return affected(q.Exec(ctx,
		`UPDATE suppliers SET organization_id=$2, supplier_number=$3, ap_account_id=$4,
		     payment_terms_code=$5, currency_code=$6, tax_code=$7, is_active=$8
		 WHERE id=$1`,
		id, in.OrganizationID, in.SupplierNumber, in.APAccountID, in.PaymentTermsCode,
		in.CurrencyCode, in.TaxCode, in.IsActive))
}
