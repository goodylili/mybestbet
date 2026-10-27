"use client"

import Link from "next/link"
import { useRouter } from "next/navigation"
import { useCallback, useEffect, useRef, useState } from "react"
import { FitRow } from "@/components/mybestbet/shared/fit-row"
import { CheckIcon, ChevronDownIcon, SearchIcon, SportIcon, StarIcon } from "@/components/mybestbet/shared/icons"
import { BookLogo, LeagueMark } from "@/components/mybestbet/shared/logos"
import { Pager, usePage } from "@/components/mybestbet/shared/pager"
import { ApiError, Loading } from "@/components/mybestbet/shared/status"
import { SearchResults } from "@/components/mybestbet/shell/search-results"
import { useShell, type ListFilter } from "@/components/mybestbet/shell/shell-context"
import { banner, compOrder } from "@/lib/catalog"
import { SPORTS, mainMarket, marketLabel, sportName } from "@/lib/odds/markets"
import { dayLong, dayOf, minutesTo } from "@/lib/odds/time"
import { routes } from "@/lib/routes"
import {
  S, calCount, covered, dayFixtures, ensureCalendar, ensureDay, ensureDays, fixturesFrom, horizon, leagueName, leagueSport, onList,
  sportCount, type Fixture,
} from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { prefs, setLine, setListMarket } from "@/lib/store/prefs"
import { listLines } from "@/lib/store/selectors"
import { DayBar } from "./day-bar"
import { MatchRow } from "./match-row"
import styles from "./match-list.module.css"

/** The sport last shown, so favourites keeps the sport chip and market tabs the viewer came from. */
let lastSport = "football"

const NEXT_MIN = 20
const NEXT_DAYS = 3

function firstDays(items: Fixture[]) {
  const out: Fixture[] = []
  let days = 0, last = NaN
  for (const f of items) {
    const d = dayOf(f.ko)
    if (d !== last) {
      if (days >= NEXT_DAYS || out.length >= NEXT_MIN) break
      days++
      last = d
    }
    out.push(f)
  }
  return out
}

const byLeague = (a: string, b: string) => compOrder(a) - compOrder(b) || leagueName(a).localeCompare(leagueName(b))

/** Matches for a sport, one competition or the viewer's favourites, grouped by competition. */
export function MatchList({ sport: sportProp, league, favourites = false }: { sport?: string; league?: string; favourites?: boolean }) {
  useStore()
  const router = useRouter()
  const { q, setQ, date, setDate, filter, setFilter, openSheet } = useShell()
  const [msOpen, setMsOpen] = useState(false)
  const [spOpen, setSpOpen] = useState(false)
  const [calOpen, setCalOpen] = useState(false)
  const [cmk, setCmk] = useState<Record<string, string>>({})
  const top = useRef<HTMLSpanElement>(null)

  const lg = league || (favourites ? "STAR" : "ALL")
  const sport = league ? leagueSport(league) : sportProp || lastSport
  useEffect(() => {
    if (!favourites) lastSport = sport
  }, [favourites, sport])
  const curMk = prefs.markets[sport] || mainMarket(sport)

  const pred = (f: Fixture) => (lg === "ALL" ? f.sp === sport : lg === "STAR" ? !!prefs.starred[f.id] : f.lg === lg)
  const match = (f: Fixture) =>
    pred(f) &&
    (filter === "arb" ? !!f.badges?.arbs : filter === "ev" ? !!f.badges?.evs : true)
  const soon = (f: Fixture) => { const m = minutesTo(f.ko); return f.status === "live" || (m >= -120 && m <= 180) }

  const ready = S.boot === "ok"
  const scope = lg === "STAR" ? "" : lg === "ALL" ? "sport=" + encodeURIComponent(sport) : "league=" + encodeURIComponent(lg)
  const calScope = scope && filter !== "arb" && filter !== "ev" ? scope : ""
  if (ready) {
    ensureCalendar("")
    if (scope) ensureCalendar(scope)
  }
  const H = horizon()
  const countOn = (d: number): number | null => (calScope ? calCount(calScope, d) : covered(d) ? dayFixtures(d).filter(match).length : null)

  const datePending = ready && filter !== "soon" ? ensureDay(date) : null
  const dayList = filter === "soon" ? dayFixtures(0).filter((f) => match(f) && soon(f)) : dayFixtures(date).filter(match)
  let shown = dayList, fallback = false, waiting = !!datePending
  if (ready && !dayList.length && !datePending) {
    fallback = true
    const from = filter === "soon" ? 0 : date + 1
    let end = calScope ? -1 : H
    if (calScope) for (let d = from, days = 0; d < H && days < NEXT_DAYS; d++) {
      const n = countOn(d)
      if (n == null) { end = -2; break }
      if (n > 0) { end = d + 1; days++ }
    }
    const next = (lo: number, hi: number, start: number | "now") => {
      let edge = lo
      while (edge < hi && covered(edge)) edge++
      const got = firstDays(fixturesFrom(start, (f) => match(f) && dayOf(f.ko) < edge))
      const full = got.length >= NEXT_MIN || new Set(got.map((f) => dayOf(f.ko))).size >= NEXT_DAYS
      const more = !full && edge < hi ? ensureDays(edge, hi) : null
      shown = got
      waiting = !full && !!more && !got.length
    }
    if (end === -2) waiting = true
    else if (end >= 0) next(from, end, filter === "soon" ? "now" : from)
    if (!shown.length && !waiting && from > 0) next(0, from, "now")
  }

  const gkey = (f: Fixture) => (fallback ? dayOf(f.ko) : 0) + "|" + f.lg
  const ordered = [...shown].sort((x, y) => (fallback ? dayOf(x.ko) - dayOf(y.ko) : 0) || byLeague(x.lg, y.lg) || x.ko - y.ko)
  const totals: Record<string, number> = {}
  ordered.forEach((f) => { totals[gkey(f)] = (totals[gkey(f)] || 0) + 1 })
  const pageKey = ["fx", sport, lg, date, filter, fallback].join("|")
  const pg = usePage(ordered, pageKey)
  const groups: { key: string; code: string; day: number; items: Fixture[] }[] = []
  pg.items.forEach((f) => {
    const k = gkey(f), g = groups[groups.length - 1]
    if (g && g.key === k) g.items.push(f)
    else groups.push({ key: k, code: f.lg, day: dayOf(f.ko), items: [f] })
  })

  const C = lg !== "ALL" && lg !== "STAR" ? lg : null
  const title = C ? leagueName(C) : lg === "STAR" ? "Favourites" : sportName(sport)
  const sp = C ? leagueSport(C) : sport
  const spm = S.sports.find((s) => s.sport === sp)
  const feed = spm ? spm.matchMarkets : [mainMarket(sp)]
  const lines = curMk === mainMarket(sport) ? [] : listLines(shown, curMk)
  const wantLine = prefs.lines[sp + curMk]
  const on = onList()
  const compsToday = [...new Set((dayList.length ? dayFixtures(date) : shown).filter((f) => f.sp === sport).map((f) => f.lg))]
  const sportList = S.sports.length ? S.sports.map((s) => s.sport) : SPORTS.map((s) => s.id)
  const bn = banner(C)

  const onCal = useCallback((open: boolean) => setCalOpen(open), [])

  const landed = useRef("")
  const landKey = lg + "|" + sport
  useEffect(() => {
    if (!ready || landed.current === landKey || filter !== "all") return
    const here = countOn(date)
    if (here == null) return
    if (here > 0) {
      landed.current = landKey
      return
    }
    for (let d = 0; d < H; d++) {
      const n = countOn(d)
      if (n == null) {
        if (!calScope) ensureDays(d, H)
        return
      }
      if (n > 0) {
        landed.current = landKey
        setDate(d)
        if (!covered(d)) ensureDay(d)
        return
      }
    }
    landed.current = landKey
  })

  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (e.key !== "/" || e.defaultPrevented || document.activeElement?.tagName === "INPUT" || document.body.classList.contains("ob-open")) return
      const q2 = document.getElementById("q2")
      if (q2 && q2.offsetParent) {
        e.preventDefault()
        q2.focus()
      }
    }
    document.addEventListener("keydown", key)
    return () => document.removeEventListener("keydown", key)
  }, [])

  if (S.boot === "error") return <ApiError />
  if (S.boot === "loading") return <Loading label="Connecting to the API" />
  if (league && S.sports.length && !S.lg[league]) {
    return (
      <div className="empty">
        <p>There is no competition called {league}</p>
        <p className="faint"><Link href={routes.sport(sport)}>See all {sportName(sport).toLowerCase()} matches</Link></p>
      </div>
    )
  }

  const leave = (nextDate: number) => {
    setSpOpen(false)
    setCalOpen(false)
    setFilter("all")
    setDate(nextDate)
  }
  const goSport = (id: string) => {
    setCmk({})
    leave(date)
    router.push(routes.sport(id))
  }
  const goAll = () => {
    leave(date)
    router.push(routes.sport(sport))
  }
  const goStar = () => {
    leave(date)
    router.push(routes.favourites())
  }
  const toLeague = () => leave(date)
  const goLeague = (code: string) => {
    toLeague()
    router.push(routes.league(code))
  }

  const onDate = (d: number) => {
    setDate(d)
    setCalOpen(false)
    if (filter === "soon") setFilter("all")
    ensureDay(d)
  }
  const onFilter = (k: ListFilter) => {
    const next = filter === k ? "all" : k
    setFilter(next)
    if (next === "soon") setDate(0)
  }
  const onMarket = (id: string, code: string) => {
    setCmk((m) => {
      const next = { ...m }
      if (code === curMk) delete next[id]
      else next[id] = code
      return next
    })
  }

  let empty = null
  if (!groups.length) {
    if (!ready || waiting) empty = <Loading label={filter === "soon" || !datePending ? "Finding the next matches" : "Loading " + dayLong(date)} />
    else if (lg === "STAR") {
      empty = (
        <div className="empty">
          <p>No starred upcoming matches</p>
          <p className="faint">Star a match with the star on its row to follow it here.</p>
          <button className="btn sm" style={{ marginTop: 8 }} onClick={goAll}>Browse {sportName(sport)}</button>
        </div>
      )
    } else if (filter === "arb" || filter === "ev") {
      empty = (
        <div className="empty">
          <p>No {filter === "arb" ? "arbs" : "value bets"} in upcoming {title} matches</p>
          <button className="btn sm" style={{ marginTop: 8 }} onClick={() => setFilter("all")}>Show all matches</button>
        </div>
      )
    } else {
      empty = (
        <div className="empty">
          <p>No upcoming {title} matches listed</p>
          <p className="faint">New fixtures appear here as soon as the books list them.</p>
        </div>
      )
    }
  }
  const note = fallback && groups.length
    ? (filter === "soon" ? "Nothing in the next 3 hours" : date === 0 ? "Nothing left today" : "Nothing on " + dayLong(date)) + ". Showing the next matches."
    : ""

  const heading = (
    <div className={styles.hd}>
      {C ? <LeagueMark code={C} size={34} /> : lg === "STAR" ? null : <span className={styles.spi}><SportIcon sport={sport} size={22} /></span>}
      <h1>{title}</h1>
      <button className={styles.bk} onClick={openSheet} data-tour="books">
        <span className={styles.stack}>
          {on.slice(0, 3).map((s) => <BookLogo key={s.name} name={s.name} size={20} />)}
        </span>
        {on.length} books
      </button>
    </div>
  )

  const tourId = groups[0]?.items[0]?.id

  return (
    <div className={styles.page}>
      <FitRow className={"chips " + styles.lgs} role="group" label="Competitions">
        <span className={"seg " + styles.spc}>
          <button className="chip sm" aria-pressed={lg === "ALL"} onClick={goAll}>
            <SportIcon sport={sport} size={15} />
            {sportName(sport)}
          </button>
          <button className={"chip sm " + styles.spdd} aria-expanded={spOpen} aria-label="Switch sport" onClick={() => setSpOpen(!spOpen)}>
            <ChevronDownIcon />
          </button>
        </span>
        <button className="chip sm" aria-pressed={lg === "STAR"} aria-label="Favourites" onClick={goStar}>
          <StarIcon />
        </button>
        {compsToday.map((c) => (
          <button key={c} className="chip sm" aria-pressed={lg === c} onClick={() => goLeague(c)}>
            <LeagueMark code={c} size={15} />
            {leagueName(c)}
          </button>
        ))}
        {C && !compsToday.includes(C) ? (
          <button className="chip sm" aria-pressed="true" onClick={() => goLeague(C)}>
            <LeagueMark code={C} size={15} />
            {leagueName(C)}
          </button>
        ) : null}
      </FitRow>

      {spOpen ? (
        <div className={styles.spMenu} role="menu" aria-label="Sport">
          {sportList.map((id) => (
            <button key={id} role="menuitemradio" aria-checked={sport === id} onClick={() => goSport(id)}>
              <SportIcon sport={id} size={18} />
              <span>{sportName(id)}</span>
              <i className="mono">{sportCount(id, date)}</i>
              {sport === id ? <CheckIcon /> : null}
            </button>
          ))}
        </div>
      ) : null}

      <div className={styles.msWrap} onBlur={(e) => { if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setMsOpen(false) }}>
        <label className={styles.msearch} htmlFor="q2">
          <SearchIcon />
          <span className="sr">Search</span>
          <input id="q2" data-q type="search" placeholder="Search matches, teams, leagues or players" autoComplete="off" value={q} onFocus={() => setMsOpen(true)} onChange={(e) => { setQ(e.target.value); setMsOpen(true) }} />
        </label>
        {msOpen && <div onMouseDown={(e) => e.preventDefault()}><SearchResults q={q} onPick={() => { setQ(""); setMsOpen(false) }} /></div>}
      </div>

      {C ? (
        <div className={styles.hero + (bn ? "" : " " + styles.nb)} style={bn ? ({ "--bn": `url(${bn})` } as React.CSSProperties) : undefined}>
          {heading}
        </div>
      ) : (
        heading
      )}

      <DayBar
        date={date}
        filter={filter}
        lines={lines}
        wantLine={wantLine}
        calOpen={calOpen}
        calLabel={lg === "ALL" ? sportName(sport) : lg === "STAR" ? "Favourites" : leagueName(lg)}
        count={countOn}
        horizon={H}
        onLine={(l) => setLine(sport, curMk, l)}
        onDate={onDate}
        onFilter={onFilter}
        onCal={onCal}
      />

      {lg === "STAR" ? null : (
        <FitRow className={"mtabs " + styles.mtabs} role="tablist" label="Market" tabs tour="market-tabs">
          {feed.map((k) => (
            <button
              key={k}
              role="tab"
              aria-selected={curMk === k}
              onClick={() => {
                setListMarket(sport, k)
                setCmk({})
              }}
            >
              {marketLabel(k, sp)}
            </button>
          ))}
        </FitRow>
      )}

      {groups.length ? (
        <>
          <span ref={top} className={styles.pgTop} />
          {note ? <p className={styles.note}>{note}</p> : null}
          {groups.map((g, k) => (
            <section key={g.key} className={styles.lgrp} aria-label={(fallback ? dayLong(g.day) + ", " : "") + leagueName(g.code)}>
              {fallback && (k === 0 || groups[k - 1].day !== g.day) ? <h3 className={styles.dayh}>{dayLong(g.day)}</h3> : null}
              {C ? null : (
                <h2>
                  <Link href={routes.league(g.code)} onClick={toLeague}>
                    <LeagueMark code={g.code} size={20} />
                    {leagueName(g.code)}
                  </Link>
                  <span className={styles.n}>{totals[g.key]}</span>
                </h2>
              )}
              <div className={styles.lst}>
                {g.items.map((f) => (
                  <MatchRow key={f.id} f={f} code={cmk[f.id] || curMk} custom={!!cmk[f.id]} onMarket={onMarket} tour={f.id === tourId} />
                ))}
              </div>
            </section>
          ))}
          <Pager page={pg.page} total={pg.total} onPage={pg.setPage} label="Matches" anchor={top} />
        </>
      ) : (
        empty
      )}
    </div>
  )
}
