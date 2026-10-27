import type { MarketHistory } from "@/lib/api/types"
import { decOf } from "@/lib/odds/format"
import styles from "./sparkline.module.css"

export type Point = { t: number; v: number }

/** A history's points for one selection, as time and decimal odds. */
export const historyPoints = (h: MarketHistory | undefined, side?: string, label?: string): Point[] =>
  (h ? h.points : [])
    .filter((p) => (!side || p.side === side) && (!label || p.label === label))
    .map((p) => ({ t: Date.parse(p.observedAt), v: decOf(p) as number }))
    .filter((p) => p.v)

/** Stepped price line with a dot on the latest price. */
export function Sparkline({ points, format, width = 320, height = 60, className = "" }: {
  points: Point[]
  format: (d: number) => string
  width?: number
  height?: number
  className?: string
}) {
  if (points.length < 2) return <div className="faint" style={{ fontSize: 13 }}>Not enough history yet</div>
  const t0 = points[0].t, t1 = points[points.length - 1].t, values = points.map((p) => p.v)
  let lo = Math.min(...values), hi = Math.max(...values)
  if (hi - lo < 0.02) { lo -= 0.01; hi += 0.01 }
  const x = (t: number) => 4 + (width - 8) * (t1 === t0 ? 1 : (t - t0) / (t1 - t0))
  const y = (v: number) => 6 + (height - 12) * (1 - (v - lo) / (hi - lo))
  let d = ""
  points.forEach((p, i) => {
    const X = x(p.t).toFixed(1), Y = y(p.v).toFixed(1)
    d += i ? ` H${X} V${Y}` : `M${X} ${Y}`
  })
  const end = points[points.length - 1]
  return (
    <svg className={styles.spark + " " + className} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" role="img" aria-label={`Moved from ${format(points[0].v)} to ${format(end.v)}`}>
      <path d={d} fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
      <circle cx={x(end.t).toFixed(1)} cy={y(end.v).toFixed(1)} r="3.2" className={styles.end} />
    </svg>
  )
}
