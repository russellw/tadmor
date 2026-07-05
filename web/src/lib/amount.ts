// Exact-decimal helpers for the amount strings the API returns (Postgres
// numeric(19,4) selected ::text, e.g. "1234.5000"). Sums use scaled BigInt
// arithmetic so values never pass through binary floating point.

const SCALE = 4

function toUnits(value: string): bigint {
  const neg = value.startsWith("-")
  const [whole, frac = ""] = (neg ? value.slice(1) : value).split(".")
  const units = BigInt(whole + frac.padEnd(SCALE, "0").slice(0, SCALE))
  return neg ? -units : units
}

function fromUnits(units: bigint): string {
  const neg = units < 0n
  const digits = (neg ? -units : units).toString().padStart(SCALE + 1, "0")
  return `${neg ? "-" : ""}${digits.slice(0, -SCALE)}.${digits.slice(-SCALE)}`
}

/** Sum amount strings exactly, returning a string at the same 4-decimal scale. */
export function sumAmounts(values: string[]): string {
  return fromUnits(values.reduce((total, v) => total + toUnits(v), 0n))
}

/** True when the amount is exactly zero (at any scale, e.g. "0.0000"). */
export function isZeroAmount(value: string): boolean {
  return toUnits(value) === 0n
}

/** Negate an amount string exactly, normalizing to the 4-decimal scale
 *  (so "0.0000" never becomes "-0.0000"). */
export function negateAmount(value: string): string {
  return fromUnits(-toUnits(value))
}

// Round n/d to the nearest integer, half away from zero (matching Postgres
// round(numeric)). BigInt division truncates toward zero, so the remainder
// carries n's sign.
function roundDiv(n: bigint, d: bigint): bigint {
  const q = n / d
  const r = (n % d) * 2n
  if (r >= d) return q + 1n
  if (-r >= d) return q - 1n
  return q
}

const AMOUNT_RE = /^-?(\d+(\.\d*)?|\.\d+)$/

/** Preview of one invoice/bill line's money, mirroring the database's
 *  generated columns: subtotal = round(qty·price, 4) and
 *  tax = round(qty·price·rate/100, 4). Returns null while any input is not a
 *  parseable decimal (e.g. mid-typing). */
export function lineAmounts(
  quantity: string,
  unitPrice: string,
  taxRate: string,
): { subtotal: string; tax: string; total: string } | null {
  if (![quantity, unitPrice, taxRate].every((v) => AMOUNT_RE.test(v))) {
    return null
  }
  const qp = toUnits(quantity) * toUnits(unitPrice) // scale 8
  const subtotal = roundDiv(qp, 10_000n) // scale 4
  const tax = roundDiv(qp * toUnits(taxRate), 10_000_000_000n) // ·rate → scale 12, ÷100 → 4
  return {
    subtotal: fromUnits(subtotal),
    tax: fromUnits(tax),
    total: fromUnits(subtotal + tax),
  }
}

/** Format an amount string for display: thousands separators, trailing zeros
 *  trimmed but at least two decimals kept ("1234.5000" → "1,234.50"). */
export function formatAmount(value: string): string {
  const neg = value.startsWith("-")
  const [whole, frac = ""] = (neg ? value.slice(1) : value).split(".")
  const grouped = whole.replace(/\B(?=(\d{3})+$)/g, ",")
  const decimals = frac.replace(/0+$/, "").padEnd(2, "0")
  return `${neg ? "-" : ""}${grouped}.${decimals}`
}
