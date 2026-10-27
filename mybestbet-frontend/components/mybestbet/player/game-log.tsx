"use client"

import Link from "next/link"
import { useEffect, useRef } from "react"
import type { PlayerMatchStat, PropRow } from "@/lib/api/types"
import { marketLabel, statField } from "@/lib/odds/markets"
import { dayShort } from "@/lib/odds/time"
import { routes } from "@/lib/routes"
import { S, fixture, loadProps, loadStats, type Fixture } from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { ChevronLeftIcon, UserIcon } from "@/components/mybestbet/shared/icons"
import { Crest } from "@/components/mybestbet/shared/logos"
import { Pager, usePage } from "@/components/mybestbet/shared/pager"
import { ApiError, Loading } from "@/components/mybestbet/shared/status"
import styles from "./game-log.module.css"

type Col = [string, string, string]

const FOOTBALL: Col[] = [
  ["goals", "G", styles.cGoals],
  ["assists", "A", styles.cAssists],
  ["shots", "Sh", styles.cShots],
  ["shotsOnTarget", "SoT", styles.cShotsOnTarget],
  ["fouls", "Fls", styles.cFouls],
  ["tackles", "Tkl", styles.cTackles],
  ["yellow", "YC", styles.cYellow],
  ["rating", "Rtg", styles.cRating],
]

const SPORT_COLS: Col[][] = [
  [["points", "PTS", ""], ["rebounds", "REB", ""], ["assists", "AST", ""], ["threes", "3PM", ""]],
  [["passYards", "PaYd", ""], ["passTDs", "PaTD", ""], ["rushYards", "RuYd", ""], ["rushTDs", "RuTD", ""], ["receptions", "Rec", ""], ["recYards", "ReYd", ""], ["recTDs", "ReTD", ""]],
  [["aces", "Ace", ""], ["doubleFaults", "DF", ""], ["sets", "Sets", ""]],
  [["kills", "K", ""], ["maps", "Maps", ""]],
]

function columnsFor(items: PlayerMatchStat[]): Col[] {
  const keys = new Set(items.flatMap((s) => Object.keys(s.stats || {})))
  return SPORT_COLS.find((cols) => cols.some(([k]) => k !== "assists" && keys.has(k))) || FOOTBALL
}

/** "Anytime scorer" for yes/no props, "Shots on target 1.5" for lines. */
function propLabel(r: PropRow, f: Fixture) {
  const label = marketLabel(r.key.code, f.sp)
  if (r.selections.some((s) => s.side === "yes")) return label
  const line = r.key.line
  return label + " " + (line != null ? (Number.isInteger(line) ? line + "+" : String(line).replace(/\.5$/, ".5")) : "")
}

const statValue = (s: PlayerMatchStat, k: string) => s.stats?.[k] ?? (s as unknown as Record<string, number | undefined | null>)[k]

/** A player's last 20 matches, tied to the prop they were opened from when there is one. */
export function GameLog({ id, eventId, propKey }: { id: string; eventId?: string; propKey?: string }) {
  useStore()
  const pid = id
  const glAnchor = useRef<HTMLSpanElement>(null)

  useEffect(() => {
    loadStats(pid).catch(() => {})
  }, [pid])
  useEffect(() => {
    if (eventId && !S.props[eventId]) loadProps(eventId).catch(() => {})
  }, [eventId])

  const f = eventId ? fixture(eventId) : undefined
  const ps = S.pstats[pid]
  const row = f && propKey && S.props[f.id] ? S.props[f.id].by[propKey] : undefined
  const res = row?.research
  const player = (ps && ps.player) || S.pname[pid] || { name: "Player" }
  const team = player.teamId ? S.tid[player.teamId] : undefined
  const statK = row ? statField(row.key.code) : null
  const line = row && row.key.line != null ? row.key.line : null
  const items = ps ? ps.items : []
  const cols = columnsFor(items)
  const opponentName = (teamId?: string) => (teamId && S.tid[teamId]?.name) || ""
  const hit = (s: PlayerMatchStat) => {
    const v = statK ? statValue(s, statK) : null
    return statK && line != null && v != null ? v > line : null
  }
  const ratings = res?.ratings
  const maxBucket = ratings ? Math.max(1, ...ratings.buckets.map((b) => b.count)) : 1
  const opponent = row && f ? (row.player.teamId === f.aT.id ? f.h : f.a) : ""

  const log = usePage(items, "gl|" + pid)
  const meetings = usePage(res?.vsOpponentMatches || [], "h2h|" + pid)

  if (S.boot === "error") return <ApiError />
  if (S.boot === "loading") return <Loading label="Connecting to the API" />

  return (
    <>
      <nav className={styles.crumbs} aria-label="Breadcrumb">
        {f ? (
          <Link href={routes.event(f.id, "PROPS")}><ChevronLeftIcon />{f.h} v {f.a}</Link>
        ) : (
          <Link href={routes.sport("football")}><ChevronLeftIcon />Matches</Link>
        )}
        <span aria-hidden="true">/</span>
        <span>Game log</span>
      </nav>

      <div className={styles.hd + " " + styles.plH}>
        {team ? <Crest team={team} size={40} /> : <span className={styles.spi}><UserIcon /></span>}
        <div>
          <h1>{player.name}</h1>
          <p className="faint">{team ? team.name : ""}{row && f ? " · " + propLabel(row, f) : ""}</p>
        </div>
      </div>

      {row && (
        <div className={styles.astats}>
          {(res ? res.windows : []).map((w) => (
            <div key={w.window}><span>Last {w.window}</span><b className="mono">{w.hits}/{w.matches}</b></div>
          ))}
          {res && <div><span>v {opponent}</span><b className="mono">{res.vsOpponent.hits}/{res.vsOpponent.matches}</b></div>}
        </div>
      )}

      <span className={styles.pgTop} ref={glAnchor} />
      <section className={"card " + styles.gl} aria-label="Last 20 matches">
        <h2>Last {items.length || 20} matches</h2>
        {!ps ? (
          <Loading />
        ) : ps.err ? (
          <p className="faint">{ps.err}</p>
        ) : !items.length ? (
          <p className="faint">No matches on record for this player yet.</p>
        ) : (
          <>
            <div className={styles.glT + (cols === FOOTBALL ? "" : " " + styles.gen)} role="table" style={{ "--n": cols.length } as React.CSSProperties}>
              <div className={styles.glR + " " + styles.glHd} role="row">
                <span>Date</span><span>Opponent</span><span>Min</span>
                {cols.map(([k, label, cls]) => <span key={k} className={k === statK ? styles.hl : cls}>{label}</span>)}
              </div>
              {log.items.map((s, i) => {
                const name = opponentName(s.opponentTeamId)
                return (
                  <div key={s.eventId || i} className={styles.glR + (hit(s) === true ? " " + styles.hit : "")} role="row">
                    <span className="mono">{dayShort(Date.parse(s.playedAt))}</span>
                    <span className={styles.glO}>{name ? (s.isHome ? "v " : "@ ") + name : s.isHome ? "Home" : "Away"}</span>
                    <span className={"mono" + (cols === FOOTBALL && s.minutes < 60 ? " " + styles.lo : "")}>{s.minutes}{s.started ? "" : <i>sub</i>}</span>
                    {cols.map(([k, , cls]) => {
                      const v = statValue(s, k)
                      return <span key={k} className={"mono " + (k === statK ? styles.hl : cls)}>{v == null ? "–" : k === "rating" ? Number(v).toFixed(1) : v}</span>
                    })}
                  </div>
                )
              })}
            </div>
            <Pager page={log.page} total={log.total} onPage={log.setPage} label="Matches" anchor={glAnchor} />
          </>
        )}
        {statK && line != null && <p className={"faint " + styles.note}>Highlighted numbers cleared the {line} line.</p>}
      </section>

      <div className={styles.pl2}>
        {ratings && (
          <section className="card" aria-label="Rating spread">
            <h2>Rating spread</h2>
            <div className={styles.rd} role="img" aria-label={"Average rating " + ratings.mean}>
              {ratings.buckets.map((b, i) => (
                <span key={i} className={styles.rdC}>
                  <i style={{ height: Math.round(b.count / maxBucket * 80) + 2 }} />
                  <b className="mono">{b.from.toFixed(1)}</b>
                </span>
              ))}
            </div>
            <div className={styles.kv}><span>Average</span><span className="mono">{Number(ratings.mean).toFixed(2)}</span></div>
            <div className={styles.kv}><span>Range</span><span className="mono">{Number(ratings.min).toFixed(1)} to {Number(ratings.max).toFixed(1)}</span></div>
          </section>
        )}
        {res && res.vsOpponentMatches && (
          <section className="card" aria-label="Head to head">
            <h2>v {opponent}</h2>
            {res.vsOpponentMatches.length ? (
              <>
                {meetings.items.map((s, i) => {
                  const v = statK ? statValue(s, statK) : null
                  return (
                    <div key={s.eventId || i} className={styles.kv}>
                      <span>{dayShort(Date.parse(s.playedAt))} · {s.isHome ? "home" : "away"} · {s.minutes}′</span>
                      <span className={"mono" + (hit(s) ? " " + styles.pos : "")}>{statK ? (v == null ? "–" : v) : "–"}</span>
                    </div>
                  )
                })}
                <Pager page={meetings.page} total={meetings.total} onPage={meetings.setPage} label="Meetings" />
              </>
            ) : (
              <p className="faint">No meetings on record.</p>
            )}
          </section>
        )}
      </div>
    </>
  )
}
