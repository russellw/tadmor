// Date-only arithmetic on the API's YYYY-MM-DD strings, used to prefill the
// fiscal-calendar forms and to bucket due dates on the dashboard. All math
// runs in UTC so the local timezone can never shift a date across midnight.

function parts(date: string): [number, number, number] {
  const [y, m, d] = date.split("-").map(Number)
  return [y, m, d]
}

function format(t: Date): string {
  return t.toISOString().slice(0, 10)
}

/** The day after the given date. */
export function dayAfter(date: string): string {
  return daysAfter(date, 1)
}

/** The date n days after the given date. */
export function daysAfter(date: string, n: number): string {
  const [y, m, d] = parts(date)
  return format(new Date(Date.UTC(y, m - 1, d + n)))
}

/** Today's date in the user's local timezone (the calendar day they see),
 *  matching how the server's aging views bucket on current_date. */
export function today(): string {
  const t = new Date()
  const m = String(t.getMonth() + 1).padStart(2, "0")
  const d = String(t.getDate()).padStart(2, "0")
  return `${t.getFullYear()}-${m}-${d}`
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
