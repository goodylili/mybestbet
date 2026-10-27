"use client"

import Link from "next/link"
import { useEffect, useState } from "react"
import { banner } from "@/lib/catalog"
import { mainMarket, marketLabel, sportName } from "@/lib/odds/markets"
import { dayLong, dayOf, liveClock, minutesTo } from "@/lib/odds/time"
import { routes } from "@/lib/routes"
import { arbPct, fixture, isShown, leagueName, loadDetail, loadProps, S, signalsFor, watchEvent } from "@/lib/store/data"
import { emit, useStore } from "@/lib/store/emitter"
import { chances } from "@/lib/store/selectors"
import { Countdown, useNow } from "@/components/mybestbet/shared/countdown"
import { FitRow } from "@/components/mybestbet/shared/fit-row"
import { ChevronLeftIcon, ChevronRightIcon } from "@/components/mybestbet/shared/icons"
import { Crest, LeagueMark } from "@/components/mybestbet/shared/logos"
import { ApiError, errorCopy, Loading } from "@/components/mybestbet/shared/status"
import { MarketBlock } from "./odds-grid"
import { PropsList } from "./props-list"
import styles from "./match-page.module.css"

const pct = (p: number[] | null, i: number) => (p ? Math.round(p[i] * 100) + "%" : "")

export function MatchPage({ id, market }: { id: string; market?: string }) {
  return <MatchView key={id + "|" + (market || "")} id={id} market={market} />
}

function MatchView({ id, market }: { id: string; market?: string }) {
  useStore()
  useNow(30000)
  const [tab, setTab] = useState(market || "")
  const [err, setErr] = useState("")
  const [mode, setMode] = useState<"listed" | "fair">("listed")
  const ready = S.boot === "ok"

  useEffect(() => {
    if (market === "PROPS") loadProps(id).catch(() => {})
    loadDetail(id).catch((e: Error) => {
      S.err = e.message
      setErr(errorCopy(e, "match"))
      emit()
    })
  }, [id, market])

  useEffect(() => {
    watchEvent(ready && !err ? id : null)
    return () => watchEvent(null)
  }, [id, ready, err])

  if (S.boot === "error" && !fixture(id)) return <ApiError />
  if (S.boot === "loading" && !fixture(id)) return <Loading label="Connecting to the API" />

  const f = fixture(id)
  if (!f) {
    if (err) return <div className="empty"><p>Couldn&apos;t load this match</p><p className="faint">{err}</p></div>
    return <Loading label="Loading match" />
  }

  const det = S.det[f.id]
  const sport = S.sports.find((s) => s.sport === f.sp)
  const hasProps = !!sport?.playerProps?.length
  const markets = det ? det.d.markets : []
  let current = tab || mainMarket(f.sp)
  if (current !== "PROPS" && det && !markets.some((m) => m.code === current)) {
    current = markets.some((m) => m.code === mainMarket(f.sp)) ? mainMarket(f.sp) : markets[0]?.code || ""
  }
  const isProps = current === "PROPS"

  const choose = (code: string) => {
    setTab(code)
    try {
      window.history.replaceState(window.history.state, "", routes.event(f.id, code === mainMarket(f.sp) ? undefined : code))
    } catch {}
  }

  const p = chances(f)
  const minutes = minutesTo(f.ko)
  const arb = signalsFor(f.id, "arb").filter((a) => a.legs.every((l) => isShown(l.src)))[0]
  const bn = banner(f.lg)
  const sc = f.score && (f.status === "live" || f.status === "finished") ? f.score : null

  return (
    <>
      <nav className={styles.crumbs} aria-label="Breadcrumb">
        <Link href={routes.sport(f.sp)}><ChevronLeftIcon />{sportName(f.sp)}</Link>
        <span aria-hidden="true">/</span>
        <Link href={routes.league(f.lg)}><LeagueMark code={f.lg} size={16} />{leagueName(f.lg)}</Link>
      </nav>
      <section
        className={"card " + styles.board + (bn ? " " + styles.hb : "")}
        style={bn ? ({ "--bn": `url(${bn})` } as React.CSSProperties) : undefined}
        aria-labelledby="match-h1"
      >
        <h1 id="match-h1" className={styles.h1}>{f.h} v {f.a}</h1>
        <div className={styles.t}><Crest team={f.hT} size={56} /><b>{f.h}</b><span>{pct(p, 0)}</span></div>
        <div className={styles.mid}>
          {sc ? (
            <>
              <span className={styles.k + " mono"} aria-label={`Score ${sc.home} ${sc.away}`}>{sc.home} – {sc.away}</span>
              <span className={styles.soon}>{liveClock(sc)}</span>
            </>
          ) : (
            <>
              <span className={styles.k}>{f.t}</span>
              <span className={minutes <= 60 ? styles.soon : ""}>{minutes < 720 || f.status !== "scheduled" ? <Countdown f={f} /> : dayLong(dayOf(f.ko))}</span>
            </>
          )}
          <span>{p && p.length === 3 ? "Draw " + Math.round(p[1] * 100) + "%" : leagueName(f.lg)}</span>
        </div>
        <div className={styles.t}><Crest team={f.aT} size={56} /><b>{f.a}</b><span>{p ? pct(p, p.length - 1) : ""}</span></div>
      </section>
      {arb && (
        <Link className={styles.banner} href={routes.signals("arb", arb.id)}>
          <span className="badge arb">+{arbPct(arb).toFixed(2)}%</span>
          <span className={styles.tx}>{marketLabel(arb.key.code, f.sp)} arb across {arb.legs.map((l) => l.src).join(", ")}</span>
          <ChevronRightIcon />
        </Link>
      )}
      <FitRow className="mtabs" label="Markets" role="tablist" tabs tour="match-tabs">
        {markets.map((m) => {
          const per = m.periods[0]
          const n = per ? per.boards.reduce((a, b) => a + b.selections.length, 0) : 0
          return (
            <button key={m.code} role="tab" aria-selected={current === m.code} onClick={() => choose(m.code)}>
              {marketLabel(m.code, f.sp)}<span className="n">{n}</span>
            </button>
          )
        })}
        {hasProps && (
          <button role="tab" aria-selected={isProps} onClick={() => choose("PROPS")}>
            Player props{S.props[f.id] && <span className="n">{S.props[f.id].b.rows.length}</span>}
          </button>
        )}
      </FitRow>
      {isProps ? <PropsList f={f} /> : det ? <MarketBlock key={current} f={f} code={current} mode={mode} onMode={setMode} /> : <Loading label="Loading prices" />}
    </>
  )
}
