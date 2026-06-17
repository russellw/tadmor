package master

import "context"

// ---------------------------------------------------------------------------
// Fiscal years
// ---------------------------------------------------------------------------

type FiscalYear struct {
	ID        int    `db:"id" json:"id"`
	Name      string `db:"name" json:"name"`
	StartDate string `db:"start_date" json:"start_date"`
	EndDate   string `db:"end_date" json:"end_date"`
	Status    string `db:"status" json:"status"`
}

type FiscalYearInput struct {
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Status    string `json:"status"` // open|closed; used by Update
}

func (in FiscalYearInput) Validate() string {
	switch {
	case in.Name == "":
		return "name is required"
	case in.StartDate == "":
		return "start_date is required"
	case in.EndDate == "":
		return "end_date is required"
	}
	return ""
}

const fiscalYearColumns = `id, name, start_date::text AS start_date, end_date::text AS end_date, status`

func ListFiscalYears(ctx context.Context, q Querier) ([]FiscalYear, error) {
	return collectList[FiscalYear](ctx, q, `SELECT `+fiscalYearColumns+` FROM fiscal_years ORDER BY start_date`)
}

func GetFiscalYear(ctx context.Context, q Querier, id int) (FiscalYear, error) {
	return collectOne[FiscalYear](ctx, q, `SELECT `+fiscalYearColumns+` FROM fiscal_years WHERE id = $1`, id)
}

func CreateFiscalYear(ctx context.Context, q Querier, in FiscalYearInput) (int, error) {
	var id int
	err := q.QueryRow(ctx,
		`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ($1,$2::date,$3::date) RETURNING id`,
		in.Name, in.StartDate, in.EndDate).Scan(&id)
	return id, err
}

func UpdateFiscalYear(ctx context.Context, q Querier, id int, in FiscalYearInput) error {
	return affected(q.Exec(ctx,
		`UPDATE fiscal_years SET name=$2, start_date=$3::date, end_date=$4::date, status=$5 WHERE id=$1`,
		id, in.Name, in.StartDate, in.EndDate, orDefault(in.Status, "open")))
}

// ---------------------------------------------------------------------------
// Accounting periods
// ---------------------------------------------------------------------------

type AccountingPeriod struct {
	ID           int    `db:"id" json:"id"`
	FiscalYearID int    `db:"fiscal_year_id" json:"fiscal_year_id"`
	Name         string `db:"name" json:"name"`
	StartDate    string `db:"start_date" json:"start_date"`
	EndDate      string `db:"end_date" json:"end_date"`
	Status       string `db:"status" json:"status"`
}

type AccountingPeriodInput struct {
	FiscalYearID int    `json:"fiscal_year_id"`
	Name         string `json:"name"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	Status       string `json:"status"` // open|closed; used by Update
}

func (in AccountingPeriodInput) Validate() string {
	switch {
	case in.FiscalYearID <= 0:
		return "fiscal_year_id is required"
	case in.Name == "":
		return "name is required"
	case in.StartDate == "":
		return "start_date is required"
	case in.EndDate == "":
		return "end_date is required"
	}
	return ""
}

const periodColumns = `id, fiscal_year_id, name, start_date::text AS start_date, end_date::text AS end_date, status`

func ListAccountingPeriods(ctx context.Context, q Querier) ([]AccountingPeriod, error) {
	return collectList[AccountingPeriod](ctx, q, `SELECT `+periodColumns+` FROM accounting_periods ORDER BY start_date`)
}

func GetAccountingPeriod(ctx context.Context, q Querier, id int) (AccountingPeriod, error) {
	return collectOne[AccountingPeriod](ctx, q, `SELECT `+periodColumns+` FROM accounting_periods WHERE id = $1`, id)
}

func CreateAccountingPeriod(ctx context.Context, q Querier, in AccountingPeriodInput) (int, error) {
	var id int
	err := q.QueryRow(ctx,
		`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
		 VALUES ($1,$2,$3::date,$4::date) RETURNING id`,
		in.FiscalYearID, in.Name, in.StartDate, in.EndDate).Scan(&id)
	return id, err
}

func UpdateAccountingPeriod(ctx context.Context, q Querier, id int, in AccountingPeriodInput) error {
	return affected(q.Exec(ctx,
		`UPDATE accounting_periods SET fiscal_year_id=$2, name=$3, start_date=$4::date, end_date=$5::date, status=$6 WHERE id=$1`,
		id, in.FiscalYearID, in.Name, in.StartDate, in.EndDate, orDefault(in.Status, "open")))
}
