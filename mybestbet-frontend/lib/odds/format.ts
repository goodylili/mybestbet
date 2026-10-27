import type { Price } from "@/lib/api/types"

export type OddsFormat = "dec" | "us" | "pct"

/** Decimal odds for any price form, or null when the price has no usable odds. */
export function decOf(price?: Pick<Price, "form" | "raw" | "prob"> | null) {
  if (!price || price.form === "line_only") return null
  if (price.form === "decimal") return price.raw > 1 ? price.raw : null
  return price.prob > 0 && price.prob < 1 ? Math.round(100 / price.prob) / 100 : null
}

export function formatOdds(d: number | null | undefined, format: OddsFormat = "dec") {
  if (d == null || !isFinite(d) || d <= 1) return "–"
  if (format === "us") return d >= 2 ? "+" + Math.round((d - 1) * 100) : "−" + Math.round(100 / (d - 1))
  if (format === "pct") return (100 / d).toFixed(0) + "%"
  return d.toFixed(2)
}

export const money = (v: number) => v.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })
export const signedMoney = (v: number) => (v >= 0 ? "+" : "−") + money(Math.abs(v))
export const compact = (v: number) =>
  v >= 1e6 ? (v / 1e6).toFixed(v >= 1e7 ? 0 : 1) + "m" : v >= 1e3 ? Math.round(v / 1e3) + "k" : String(Math.round(v))

/** First number in a label like "Over 2.5" or "−1.5". */
export function numberIn(s?: string | null) {
  const m = /[-+−]?\d+(\.\d+)?/.exec(String(s || "").replace("−", "-"))
  return m ? parseFloat(m[0].replace("−", "-")) : null
}

export const formatLine = (line: number | null | undefined, signed?: boolean) =>
  line == null ? "" : signed ? (line > 0 ? "+" : line < 0 ? "−" : "") + Math.abs(line).toFixed(1) : String(line)

/** Prediction market spread and liquidity, e.g. "48–51¢ · $12k". */
export function depth(price?: Price | null) {
  if (!price || (price.bid == null && price.ask == null && !price.liquidity)) return ""
  let quote = ""
  if (price.bid != null && price.ask != null) {
    if (price.form === "share") quote = Math.round(price.bid * 100) + "–" + Math.round(price.ask * 100) + "¢"
    else if (price.form === "cents") quote = Math.round(price.bid) + "–" + Math.round(price.ask) + "¢"
    else if (price.form === "naira") quote = "₦" + Math.round(price.bid) + "–" + Math.round(price.ask)
  }
  const liquidity = price.liquidity ? (price.form === "naira" ? "₦" : "$") + compact(price.liquidity) : ""
  return [quote, liquidity].filter(Boolean).join(" · ")
}
