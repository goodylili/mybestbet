"use client"

import { useEffect, useRef } from "react"
import type { MarketBoard } from "@/lib/api/types"
import { dayShort, hhmm } from "@/lib/odds/time"
import { sideName } from "@/lib/odds/markets"
import { historyKey, loadHistory, S, type Fixture } from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { CloseIcon, HistoryIcon } from "@/components/mybestbet/shared/icons"
import { historyPoints, Sparkline } from "@/components/mybestbet/shared/sparkline"
import { Loading } from "@/components/mybestbet/shared/status"
import { useOdds } from "@/components/mybestbet/shared/use-odds"
import styles from "./odds-grid.module.css"

/** One book's price history for every selection of a board, shown inline under its row. */
export function PriceHistory({ b, f, src, onClose }: { b: MarketBoard; f: Pick<Fixture, "h" | "a">; src: string; onClose: () => void }) {
  useStore()
  const F = useOdds()
  const ref = useRef<HTMLElement>(null)
  const h = S.hist[historyKey(b.keyString, src)]

  useEffect(() => {
    loadHistory(b.keyString, src).catch(() => {})
  }, [b.keyString, src])

  useEffect(() => {
    ref.current?.scrollIntoView({ block: "nearest", behavior: "smooth" })
  }, [])

  const times = h ? h.points.map((p) => Date.parse(p.observedAt)) : []
  const since = times.length ? Math.min(...times) : 0

  return (
    <section ref={ref} className={styles.hpop} aria-label={src + " price history"}>
      <div className={styles.hpH}>
        <span className="faint"><HistoryIcon />Price history</span>
        <button className="ib" aria-label="Close history" onClick={onClose}><CloseIcon /></button>
      </div>
      {h ? (
        <>
          <div className={styles.hpG}>
            {b.selections.map((s, i) => {
              const pts = historyPoints(h, s.side, s.label)
              const a = pts[0], z = pts[pts.length - 1]
              return (
                <div key={i} className={styles.hpR}>
                  <span className={styles.hpL}>{s.label || sideName(s.side, "", null, f.h, f.a)}</span>
                  <Sparkline points={pts} format={F} className={styles.spk} />
                  <span className={styles.hpV + " mono"}>{a ? F(a.v) + " → " + F(z.v) : "–"}</span>
                </div>
              )
            })}
          </div>
          <p className={"faint " + styles.hpF}>{times.length ? "Since " + dayShort(since) + " " + hhmm(since) + " · " + h.points.length + " changes" : "No changes recorded"}</p>
        </>
      ) : (
        <Loading label="Loading history" />
      )}
    </section>
  )
}
