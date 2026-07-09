package master_test

import (
	"context"
	"errors"
	"testing"

	"tadmor/internal/dbtest"
	"tadmor/internal/master"
)

func TestExchangeRatesAndSettings(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	dbtest.Reset(ctx, t, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// The migration seeds settings: USD base, FX account at code 7000.
	st, err := master.GetSettings(ctx, tx)
	if err != nil || st.BaseCurrency != "USD" || st.FxGainLossAccountID == nil {
		t.Fatalf("seeded settings = %+v, %v; want USD base and an FX account", st, err)
	}

	// Exchange-rate create / update, with scale trimmed on read.
	if err := master.CreateExchangeRate(ctx, tx, master.ExchangeRateInput{
		CurrencyCode: "eur", RateDate: "2026-06-01", Rate: "1.10"}); err != nil {
		t.Fatalf("create rate: %v", err)
	}
	if err := master.UpdateExchangeRate(ctx, tx, "EUR", "2026-06-01", master.ExchangeRateInput{Rate: "1.125"}); err != nil {
		t.Fatalf("update rate: %v", err)
	}
	rates, err := master.ListExchangeRates(ctx, tx)
	if err != nil || len(rates) != 1 || rates[0].CurrencyCode != "EUR" || rates[0].Rate != "1.125" {
		t.Fatalf("list rates = %+v, %v", rates, err)
	}
	if err := master.DeleteExchangeRate(ctx, tx, "EUR", "2026-06-01"); err != nil {
		t.Fatalf("delete rate: %v", err)
	}
	if err := master.DeleteExchangeRate(ctx, tx, "EUR", "2026-06-01"); !errors.Is(err, master.ErrNotFound) {
		t.Errorf("delete missing rate err = %v, want ErrNotFound", err)
	}

	// A summary (non-postable) account is rejected as the FX account.
	summaryID, err := master.CreateAccount(ctx, tx, master.AccountInput{
		Code: "8000", Name: "Header", AccountType: "expense", IsPostable: false})
	if err != nil {
		t.Fatalf("create summary account: %v", err)
	}
	if err := master.UpdateSettings(ctx, tx, master.Settings{
		BaseCurrency: "USD", FxGainLossAccountID: &summaryID}); !errors.Is(err, master.ErrInvalid) {
		t.Errorf("set summary FX account err = %v, want ErrInvalid", err)
	}

	// Changing the base currency once journal entries exist is refused by the
	// database guard.
	if _, err := tx.Exec(ctx,
		`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY2026','2026-01-01','2026-12-31')`); err != nil {
		t.Fatalf("seed fiscal year: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
		 SELECT id,'2026-06','2026-06-01','2026-06-30' FROM fiscal_years WHERE name='FY2026'`); err != nil {
		t.Fatalf("seed period: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO journal_entries (entry_date, period_id, currency_code, status)
		 VALUES ('2026-06-15',(SELECT id FROM accounting_periods LIMIT 1),'USD','draft')`); err != nil {
		t.Fatalf("seed journal entry: %v", err)
	}
	err = master.UpdateSettings(ctx, tx, master.Settings{BaseCurrency: "EUR"})
	if err == nil {
		t.Errorf("changing base currency with entries present should fail")
	}
}
