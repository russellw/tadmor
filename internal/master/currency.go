package master

import "context"

// ---------------------------------------------------------------------------
// Exchange rates (natural key: currency_code + rate_date)
// ---------------------------------------------------------------------------

// ExchangeRate is one manually maintained rate: how many base-currency units
// one unit of CurrencyCode buys on RateDate. Posting picks the latest rate on
// or before the document date.
type ExchangeRate struct {
	CurrencyCode string `db:"currency_code" json:"currency_code"`
	RateDate     string `db:"rate_date" json:"rate_date"`
	Rate         string `db:"rate" json:"rate"`
}

type ExchangeRateInput struct {
	CurrencyCode string `json:"currency_code"`
	RateDate     string `json:"rate_date"`
	Rate         string `json:"rate"`
}

func (in ExchangeRateInput) Validate() string {
	switch {
	case len(in.CurrencyCode) != 3:
		return "currency_code must be a 3-letter ISO code"
	case in.RateDate == "":
		return "rate_date is required"
	case in.Rate == "":
		return "rate is required"
	}
	return ""
}

const exchangeRateColumns = `currency_code, rate_date::text AS rate_date, trim_scale(rate)::text AS rate`

// ListExchangeRates returns every rate, newest first within each currency.
func ListExchangeRates(ctx context.Context, q Querier) ([]ExchangeRate, error) {
	return collectList[ExchangeRate](ctx, q,
		`SELECT `+exchangeRateColumns+` FROM exchange_rates ORDER BY currency_code, rate_date DESC`)
}

func CreateExchangeRate(ctx context.Context, q Querier, in ExchangeRateInput) error {
	_, err := q.Exec(ctx,
		`INSERT INTO exchange_rates (currency_code, rate_date, rate) VALUES (upper($1),$2::date,$3::numeric)`,
		in.CurrencyCode, in.RateDate, in.Rate)
	return err
}

func UpdateExchangeRate(ctx context.Context, q Querier, currency, date string, in ExchangeRateInput) error {
	return affected(q.Exec(ctx,
		`UPDATE exchange_rates SET rate = $3::numeric WHERE currency_code = upper($1) AND rate_date = $2::date`,
		currency, date, in.Rate))
}

func DeleteExchangeRate(ctx context.Context, q Querier, currency, date string) error {
	return affected(q.Exec(ctx,
		`DELETE FROM exchange_rates WHERE currency_code = upper($1) AND rate_date = $2::date`,
		currency, date))
}

// ---------------------------------------------------------------------------
// GL settings (singleton)
// ---------------------------------------------------------------------------

// Settings is the one-row ledger configuration. BaseCurrency is frozen by a
// database trigger once journal entries exist; FxGainLossAccountID absorbs
// realized exchange differences and must be set before applying documents
// posted at different rates.
type Settings struct {
	BaseCurrency        string `db:"base_currency" json:"base_currency"`
	FxGainLossAccountID *int   `db:"fx_gain_loss_account_id" json:"fx_gain_loss_account_id"`
}

func (in Settings) Validate() string {
	if len(in.BaseCurrency) != 3 {
		return "base_currency must be a 3-letter ISO code"
	}
	return ""
}

// GetSettings returns the ledger settings row.
func GetSettings(ctx context.Context, q Querier) (Settings, error) {
	return collectOne[Settings](ctx, q,
		`SELECT base_currency, fx_gain_loss_account_id FROM gl_settings`)
}

// UpdateSettings replaces the ledger settings. The FX account, when set, must
// be a postable, active account — checked here so a bad pick fails now, not
// at the first foreign-currency settlement.
func UpdateSettings(ctx context.Context, q Querier, in Settings) error {
	if in.FxGainLossAccountID != nil {
		var ok bool
		err := q.QueryRow(ctx,
			`SELECT is_postable AND is_active FROM accounts WHERE id = $1`,
			*in.FxGainLossAccountID).Scan(&ok)
		if err != nil || !ok {
			return errInvalid("fx_gain_loss_account_id must be a postable, active account")
		}
	}
	return affected(q.Exec(ctx,
		`UPDATE gl_settings SET base_currency = upper($1), fx_gain_loss_account_id = $2`,
		in.BaseCurrency, in.FxGainLossAccountID))
}
