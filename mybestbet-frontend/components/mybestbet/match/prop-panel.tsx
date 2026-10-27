"use client"

import Link from "next/link"
import { useEffect } from "react"
import type { HitRate, PlayerMatchStat, PropRow } from "@/lib/api/types"
import { PROP_STAT, statField } from "@/lib/odds/markets"
import { routes } from "@/lib/routes"
import { loadStats, S, type Fixture } from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { UserIcon } from "@/components/mybestbet/shared/icons"
import styles from "./props.module.css"

/** The stat a prop settles on for one match, matching loosely on the code when it isn't listed. */
export function statOf(code: string, s: PlayerMatchStat): number | null {
  const k = statField(code) || Object.keys(PROP_STAT).filter((x) => code.includes(x.slice(2))).map((x) => PROP_STAT[x])[0]
  if (!k) return null
  const v = s.stats?.[k] ?? (s as unknown as Record<string, number | undefined>)[k]
  return v == null ? null : v
}

const rate = (w?: HitRate) => (w ? `${w.hits}/${w.matches}` : "–")

/** Research under a prop: last 10 games against the line, hit rates and minutes. */
export function PropPanel({ f, r }: { f: Fixture; r: PropRow }) {
  useStore()
  const res = r.research
  const line = r.key.line == null ? 0.5 : r.key.line
  const rating = /RATING/.test(r.key.code)
  const ps = S.pstats[r.player.id]

  useEffect(() => {
    loadStats(r.player.id).catch(() => {})
  }, [r.player.id])

  const vals = ps ? ps.items.slice(0, 10).map((s) => statOf(r.key.code, s)).reverse() : []
  const ok = vals.length > 0 && vals.every((v) => v != null)
  const nums = vals as number[]
  const max = rating ? Math.max(8.5, ...nums) : Math.max(3, ...nums) + 0.5, min = rating ? 5.5 : 0
  const height = (v: number) => Math.max(4, ((v - min) / (max - min)) * 90)
  const opponent = r.player.teamId === f.aT.id ? f.h : f.a
  const minutes = (res?.minutesTrend || []).slice().reverse()

  return (
    <div className={styles.panel}>
      <div>
        <h3>Last 10 games</h3>
        {!ps ? (
          <div className={styles.bars + " " + styles.ld} />
        ) : ok ? (
          <div className={styles.bars} role="img" aria-label={`${r.player.name} last 10: ${nums.join(", ")}`}>
            <span className={styles.th} style={{ bottom: height(line) + "px" }}><span>{line}</span></span>
            {nums.map((v, k) => (
              <span key={k} className={styles.col}><b>{v}</b><i className={v > line ? styles.on : ""} style={{ height: height(v) + "px" }} /></span>
            ))}
          </div>
        ) : (
          <p className="faint" style={{ fontSize: 13 }}>No per-game numbers for this prop.</p>
        )}
        <Link className="btn sm" href={routes.player(r.player.id, f.id, r.keyString)} style={{ alignSelf: "flex-start" }}><UserIcon />Game log</Link>
      </div>
      <div>
        <h3>Hit rate</h3>
        {(res?.windows || []).map((w) => (
          <div key={w.window} className={styles.kv}><span>Last {w.window}</span><span className="mono">{rate(w)}</span></div>
        ))}
        <div className={styles.kv}><span>Home</span><span className="mono">{rate(res?.home)}</span></div>
        <div className={styles.kv}><span>Away</span><span className="mono">{rate(res?.away)}</span></div>
        <div className={styles.kv}><span>v {opponent}</span><span className="mono">{rate(res?.vsOpponent)}</span></div>
      </div>
      <div>
        <h3>Minutes</h3>
        <div className={styles.mins} aria-hidden="true">
          {minutes.map((m, k) => <i key={k} className={m.minutes < 60 ? styles.lo : ""} style={{ height: Math.round((Math.min(90, m.minutes) / 90) * 50) + "px" }} />)}
        </div>
        {res?.benchWarning && <span className={styles.fl + " " + styles.a}>Under 60 in last 4</span>}
        {res?.lineupStatus === "bench" ? (
          <span className={"row " + styles.lineup}><span className="dot amber" />Bench</span>
        ) : res?.lineupStatus === "starter" ? (
          <span className={"row " + styles.lineup}><span className="dot" />Starting XI</span>
        ) : res?.lineupStatus === "out" ? (
          <span className={"row " + styles.lineup}><span className="dot amber" />Out</span>
        ) : (
          <span className="faint" style={{ fontSize: 13.5 }}>Lineup not announced yet</span>
        )}
      </div>
    </div>
  )
}
