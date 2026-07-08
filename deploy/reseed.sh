#!/bin/sh
# Nightly demo reseed: throw away whatever demo guests changed by rebuilding
# the tadmor database from the snapshot at /var/lib/tadmor-demo/seed.sql
# (refreshed with `make demo-snapshot`). Installed as /opt/tadmor/reseed.sh,
# run as root by tadmor-reseed.timer; see docs/deployment.md.
set -eu

SEED=/var/lib/tadmor-demo/seed.sql
test -s "$SEED"

systemctl stop tadmor
# Bring the app back up whatever happens below; a failed restore then shows up
# both as this unit failing and as tadmor crash-looping on the missing DB.
trap 'systemctl start tadmor' EXIT

sudo -u postgres psql -v ON_ERROR_STOP=1 -q <<'SQL'
DROP DATABASE IF EXISTS tadmor WITH (FORCE);
CREATE DATABASE tadmor OWNER tadmor;
SQL
sudo -u postgres psql -v ON_ERROR_STOP=1 -q -d tadmor -f "$SEED"

# The snapshot ages: make sure a fiscal year and an open accounting period
# cover today, or posting documents dated now fails until someone adds them
# on the Periods screen.
sudo -u postgres psql -v ON_ERROR_STOP=1 -q -d tadmor <<'SQL'
INSERT INTO fiscal_years (name, start_date, end_date)
SELECT 'FY' || extract(year FROM current_date),
       date_trunc('year', current_date)::date,
       (date_trunc('year', current_date) + interval '1 year - 1 day')::date
WHERE NOT EXISTS (SELECT 1 FROM fiscal_years
                  WHERE current_date BETWEEN start_date AND end_date);

INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
SELECT fy.id, to_char(current_date, 'YYYY-MM'),
       date_trunc('month', current_date)::date,
       (date_trunc('month', current_date) + interval '1 month - 1 day')::date
FROM fiscal_years fy
WHERE current_date BETWEEN fy.start_date AND fy.end_date
  AND NOT EXISTS (SELECT 1 FROM accounting_periods
                  WHERE current_date BETWEEN start_date AND end_date);
SQL
