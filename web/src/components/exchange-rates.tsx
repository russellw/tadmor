import { useEffect, useState } from "react"
import { Link } from "react-router-dom"

import {
  getSettings,
  listExchangeRates,
  type ExchangeRate,
  type Settings,
} from "@/lib/api"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// Exchange rates are keyed by currency + date, so rows link by both. A rate is
// how many base-currency units one unit of the currency buys on that date;
// posting a document uses the latest rate on or before its date.
export function ExchangeRates() {
  const [rates, setRates] = useState<ExchangeRate[] | null>(null)
  const [settings, setSettings] = useState<Settings | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([listExchangeRates(), getSettings()])
      .then(([r, s]) => {
        if (!cancelled) {
          setRates(r)
          setSettings(s)
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <section className="mx-auto w-full max-w-5xl p-6">
      <header className="mb-6 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Exchange Rates
          </h1>
          <p className="text-sm text-muted-foreground">
            {settings !== null
              ? `Rates into the ${settings.base_currency} base currency. A document posts at the latest rate on or before its date.`
              : "Rates into the base currency, used when posting foreign-currency documents."}
          </p>
        </div>
        <Button asChild>
          <Link to="/exchange-rates/new">New rate</Link>
        </Button>
      </header>

      {error !== null && (
        <p className="text-sm text-destructive" role="alert">
          Failed to load exchange rates: {error}
        </p>
      )}

      {error === null && rates === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {rates !== null && rates.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No exchange rates yet. Documents in the base currency need none.
        </p>
      )}

      {rates !== null && rates.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Currency</TableHead>
              <TableHead>Date</TableHead>
              <TableHead className="text-right">
                Rate{settings !== null && ` (→ ${settings.base_currency})`}
              </TableHead>
              <TableHead className="w-0"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rates.map((r) => (
              <TableRow key={`${r.currency_code}-${r.rate_date}`}>
                <TableCell className="font-mono font-medium">
                  {r.currency_code}
                </TableCell>
                <TableCell className="font-mono">{r.rate_date}</TableCell>
                <TableCell className="text-right font-mono tabular-nums">
                  {r.rate}
                </TableCell>
                <TableCell className="text-right">
                  <Link
                    to={`/exchange-rates/${encodeURIComponent(r.currency_code)}/${encodeURIComponent(r.rate_date)}`}
                    className="text-sm font-medium text-primary hover:underline"
                  >
                    Edit
                  </Link>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </section>
  )
}
