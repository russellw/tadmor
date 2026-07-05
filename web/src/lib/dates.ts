// Date-only arithmetic on the API's YYYY-MM-DD strings, used to prefill the
// fiscal-calendar forms. All math runs in UTC so the local timezone can never
// shift a date across midnight.

function parts(date: string): [number, number, number] {
  const [y, m, d] = date.split("-").map(Number)
  return [y, m, d]
}

function format(t: Date): string {
  return t.toISOString().slice(0, 10)
}

/** The day after the given date. */
export function dayAfter(date: string): string {
  const [y, m, d] = parts(date)
  return format(new Date(Date.UTC(y, m - 1, d + 1)))
}

/** The last day of the given date's month. */
export function endOfMonth(date: string): string {
  const [y, m] = parts(date)
  return format(new Date(Date.UTC(y, m, 0)))
}

/** The end of a year-long span starting on the given date (one year on, minus
 *  a day): 2027-01-01 → 2027-12-31. */
export function endOfYearFrom(start: string): string {
  const [y, m, d] = parts(start)
  return format(new Date(Date.UTC(y + 1, m - 1, d - 1)))
}

/** The date's YYYY-MM prefix — the conventional accounting-period name. */
export function yearMonth(date: string): string {
  return date.slice(0, 7)
}
