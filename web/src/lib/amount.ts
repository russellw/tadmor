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

/** Format an amount string for display: thousands separators, trailing zeros
 *  trimmed but at least two decimals kept ("1234.5000" → "1,234.50"). */
export function formatAmount(value: string): string {
  const neg = value.startsWith("-")
  const [whole, frac = ""] = (neg ? value.slice(1) : value).split(".")
  const grouped = whole.replace(/\B(?=(\d{3})+$)/g, ",")
  const decimals = frac.replace(/0+$/, "").padEnd(2, "0")
  return `${neg ? "-" : ""}${grouped}.${decimals}`
}
