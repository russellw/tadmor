package master

import "context"

// ---------------------------------------------------------------------------
// Products
// ---------------------------------------------------------------------------

type Product struct {
	ID                 int     `db:"id" json:"id"`
	SKU                string  `db:"sku" json:"sku"`
	Name               string  `db:"name" json:"name"`
	Description        *string `db:"description" json:"description"`
	UnitPrice          string  `db:"unit_price" json:"unit_price"`
	CurrencyCode       *string `db:"currency_code" json:"currency_code"`
	RevenueAccountID   *int    `db:"revenue_account_id" json:"revenue_account_id"`
	TaxCode            *string `db:"tax_code" json:"tax_code"`
	TrackInventory     bool    `db:"track_inventory" json:"track_inventory"`
	InventoryAccountID *int    `db:"inventory_account_id" json:"inventory_account_id"`
	CogsAccountID      *int    `db:"cogs_account_id" json:"cogs_account_id"`
	IsActive           bool    `db:"is_active" json:"is_active"`
}

type ProductInput struct {
	SKU                string  `json:"sku"`
	Name               string  `json:"name"`
	Description        *string `json:"description"`
	UnitPrice          string  `json:"unit_price"` // decimal; default "0"
	CurrencyCode       *string `json:"currency_code"`
	RevenueAccountID   *int    `json:"revenue_account_id"`
	TaxCode            *string `json:"tax_code"`
	TrackInventory     bool    `json:"track_inventory"`
	InventoryAccountID *int    `json:"inventory_account_id"`
	CogsAccountID      *int    `json:"cogs_account_id"`
	IsActive           bool    `json:"is_active"` // used by Update (full replace)
}

func (in ProductInput) Validate() string {
	switch {
	case in.SKU == "":
		return "sku is required"
	case in.Name == "":
		return "name is required"
	}
	return ""
}

const productColumns = `id, sku, name, description, unit_price::text AS unit_price,
	currency_code, revenue_account_id, tax_code, track_inventory,
	inventory_account_id, cogs_account_id, is_active`

func ListProducts(ctx context.Context, q Querier) ([]Product, error) {
	return collectList[Product](ctx, q, `SELECT `+productColumns+` FROM products ORDER BY sku`)
}

func GetProduct(ctx context.Context, q Querier, id int) (Product, error) {
	return collectOne[Product](ctx, q, `SELECT `+productColumns+` FROM products WHERE id = $1`, id)
}

func CreateProduct(ctx context.Context, q Querier, in ProductInput) (int, error) {
	var id int
	err := q.QueryRow(ctx,
		`INSERT INTO products (sku, name, description, unit_price, currency_code,
		     revenue_account_id, tax_code, track_inventory, inventory_account_id, cogs_account_id)
		 VALUES ($1,$2,$3,$4::numeric,$5,$6,$7,$8,$9,$10) RETURNING id`,
		in.SKU, in.Name, in.Description, orDefault(in.UnitPrice, "0"), in.CurrencyCode,
		in.RevenueAccountID, in.TaxCode, in.TrackInventory, in.InventoryAccountID, in.CogsAccountID).Scan(&id)
	return id, err
}

func UpdateProduct(ctx context.Context, q Querier, id int, in ProductInput) error {
	return affected(q.Exec(ctx,
		`UPDATE products SET sku=$2, name=$3, description=$4, unit_price=$5::numeric, currency_code=$6,
		     revenue_account_id=$7, tax_code=$8, track_inventory=$9, inventory_account_id=$10,
		     cogs_account_id=$11, is_active=$12
		 WHERE id=$1`,
		id, in.SKU, in.Name, in.Description, orDefault(in.UnitPrice, "0"), in.CurrencyCode,
		in.RevenueAccountID, in.TaxCode, in.TrackInventory, in.InventoryAccountID, in.CogsAccountID, in.IsActive))
}

// ---------------------------------------------------------------------------
// Accounts (chart of accounts)
// ---------------------------------------------------------------------------

type Account struct {
	ID           int     `db:"id" json:"id"`
	Code         string  `db:"code" json:"code"`
	Name         string  `db:"name" json:"name"`
	AccountType  string  `db:"account_type" json:"account_type"`
	ParentID     *int    `db:"parent_id" json:"parent_id"`
	CurrencyCode *string `db:"currency_code" json:"currency_code"`
	IsPostable   bool    `db:"is_postable" json:"is_postable"`
	IsActive     bool    `db:"is_active" json:"is_active"`
}

type AccountInput struct {
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	AccountType  string  `json:"account_type"`
	ParentID     *int    `json:"parent_id"`
	CurrencyCode *string `json:"currency_code"`
	IsPostable   bool    `json:"is_postable"`
	IsActive     bool    `json:"is_active"`
}

func (in AccountInput) Validate() string {
	switch {
	case in.Code == "":
		return "code is required"
	case in.Name == "":
		return "name is required"
	case in.AccountType == "":
		return "account_type is required"
	}
	return ""
}

const accountColumns = `id, code, name, account_type, parent_id, currency_code, is_postable, is_active`

func ListAccounts(ctx context.Context, q Querier) ([]Account, error) {
	return collectList[Account](ctx, q, `SELECT `+accountColumns+` FROM accounts ORDER BY code`)
}

func GetAccount(ctx context.Context, q Querier, id int) (Account, error) {
	return collectOne[Account](ctx, q, `SELECT `+accountColumns+` FROM accounts WHERE id = $1`, id)
}

func CreateAccount(ctx context.Context, q Querier, in AccountInput) (int, error) {
	var id int
	err := q.QueryRow(ctx,
		`INSERT INTO accounts (code, name, account_type, parent_id, currency_code, is_postable)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		in.Code, in.Name, in.AccountType, in.ParentID, in.CurrencyCode, in.IsPostable).Scan(&id)
	return id, err
}

func UpdateAccount(ctx context.Context, q Querier, id int, in AccountInput) error {
	return affected(q.Exec(ctx,
		`UPDATE accounts SET code=$2, name=$3, account_type=$4, parent_id=$5,
		     currency_code=$6, is_postable=$7, is_active=$8
		 WHERE id=$1`,
		id, in.Code, in.Name, in.AccountType, in.ParentID, in.CurrencyCode, in.IsPostable, in.IsActive))
}

// ---------------------------------------------------------------------------
// Tax codes (natural key: code)
// ---------------------------------------------------------------------------

type TaxCode struct {
	Code         string `db:"code" json:"code"`
	Name         string `db:"name" json:"name"`
	Rate         string `db:"rate" json:"rate"`
	TaxAccountID *int   `db:"tax_account_id" json:"tax_account_id"`
	IsActive     bool   `db:"is_active" json:"is_active"`
}

type TaxCodeInput struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	Rate         string `json:"rate"` // percent; default "0"
	TaxAccountID *int   `json:"tax_account_id"`
	IsActive     bool   `json:"is_active"`
}

func (in TaxCodeInput) Validate() string {
	switch {
	case in.Code == "":
		return "code is required"
	case in.Name == "":
		return "name is required"
	}
	return ""
}

const taxCodeColumns = `code, name, rate::text AS rate, tax_account_id, is_active`

func ListTaxCodes(ctx context.Context, q Querier) ([]TaxCode, error) {
	return collectList[TaxCode](ctx, q, `SELECT `+taxCodeColumns+` FROM tax_codes ORDER BY code`)
}

func GetTaxCode(ctx context.Context, q Querier, code string) (TaxCode, error) {
	return collectOne[TaxCode](ctx, q, `SELECT `+taxCodeColumns+` FROM tax_codes WHERE code = $1`, code)
}

func CreateTaxCode(ctx context.Context, q Querier, in TaxCodeInput) (string, error) {
	_, err := q.Exec(ctx,
		`INSERT INTO tax_codes (code, name, rate, tax_account_id) VALUES ($1,$2,$3::numeric,$4)`,
		in.Code, in.Name, orDefault(in.Rate, "0"), in.TaxAccountID)
	return in.Code, err
}

func UpdateTaxCode(ctx context.Context, q Querier, code string, in TaxCodeInput) error {
	return affected(q.Exec(ctx,
		`UPDATE tax_codes SET name=$2, rate=$3::numeric, tax_account_id=$4, is_active=$5 WHERE code=$1`,
		code, in.Name, orDefault(in.Rate, "0"), in.TaxAccountID, in.IsActive))
}

// ---------------------------------------------------------------------------
// Warehouses
// ---------------------------------------------------------------------------

type Warehouse struct {
	ID        int    `db:"id" json:"id"`
	Code      string `db:"code" json:"code"`
	Name      string `db:"name" json:"name"`
	AddressID *int   `db:"address_id" json:"address_id"`
	IsActive  bool   `db:"is_active" json:"is_active"`
}

type WarehouseInput struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	AddressID *int   `json:"address_id"`
	IsActive  bool   `json:"is_active"`
}

func (in WarehouseInput) Validate() string {
	switch {
	case in.Code == "":
		return "code is required"
	case in.Name == "":
		return "name is required"
	}
	return ""
}

const warehouseColumns = `id, code, name, address_id, is_active`

func ListWarehouses(ctx context.Context, q Querier) ([]Warehouse, error) {
	return collectList[Warehouse](ctx, q, `SELECT `+warehouseColumns+` FROM warehouses ORDER BY code`)
}

func GetWarehouse(ctx context.Context, q Querier, id int) (Warehouse, error) {
	return collectOne[Warehouse](ctx, q, `SELECT `+warehouseColumns+` FROM warehouses WHERE id = $1`, id)
}

func CreateWarehouse(ctx context.Context, q Querier, in WarehouseInput) (int, error) {
	var id int
	err := q.QueryRow(ctx,
		`INSERT INTO warehouses (code, name, address_id) VALUES ($1,$2,$3) RETURNING id`,
		in.Code, in.Name, in.AddressID).Scan(&id)
	return id, err
}

func UpdateWarehouse(ctx context.Context, q Querier, id int, in WarehouseInput) error {
	return affected(q.Exec(ctx,
		`UPDATE warehouses SET code=$2, name=$3, address_id=$4, is_active=$5 WHERE id=$1`,
		id, in.Code, in.Name, in.AddressID, in.IsActive))
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
