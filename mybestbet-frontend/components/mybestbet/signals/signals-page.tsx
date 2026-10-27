"use client"

import { useEffect, useRef, useState, type ReactNode } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import type { SignalType } from "@/lib/api/types"
import { teamTag } from "@/lib/catalog"
import { numberIn } from "@/lib/odds/format"
import { SIGNAL_INFO, TERM } from "@/lib/odds/glossary"
import { SIGNALS, SIGNAL_TYPES, SPORTS, sportName } from "@/lib/odds/markets"
import { ago, dayOf, dayTab, minutesTo } from "@/lib/odds/time"
import { routes } from "@/lib/routes"
import { S, arbPct, leagueName, onCount, onList, signalList, signalsPending, wantProps, type Fixture, type SignalItem, type SignalLeg } from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { signalMarket } from "@/lib/store/selectors"
import { Countdown } from "@/components/mybestbet/shared/countdown"
import { FitRow } from "@/components/mybestbet/shared/fit-row"
import { ExternalIcon, SIGNAL_ICONS, SportIcon, TelegramIcon } from "@/components/mybestbet/shared/icons"
import { InfoTip } from "@/components/mybestbet/shared/info-tip"
import { BookLink, BookLogo, Crest, LeagueMark } from "@/components/mybestbet/shared/logos"
import { Pager, usePage } from "@/components/mybestbet/shared/pager"
import { ApiError, Loading } from "@/components/mybestbet/shared/status"
import { useOdds } from "@/components/mybestbet/shared/use-odds"
import { useShell } from "@/components/mybestbet/shell/shell-context"
import { SignalTour, hasSignalTour, replaySignalTour } from "@/components/mybestbet/onboarding/signal-tour"
import { ArbCalc } from "./arb-calc"
import { MoveDetail } from "./move-detail"
import styles from "./signals-page.module.css"
import { webHref } from "@/lib/utils"

type Sort = "profit" | "time"

/*
  Sort order and sport filter carry over when switching between signal tabs,
  and reset once the viewer leaves the signals screens.
*/
const carry = { sort: "profit" as Sort, sport: "all" }
let carryReset: ReturnType<typeof setTimeout> | undefined

const SORT_LABEL: Record<SignalType, string> = { arb: "Top profit", ev: "Top edge", line: "Newest", move: "Biggest" }

const MINIMUMS: Partial<Record<SignalType, [number, string][]>> = {
  arb: [[0, "Any"], [1, "1%+"], [2, "2%+"]],
  ev: [[0, "Any"], [2, "2%+"], [5, "5%+"]],
}

const NONE: Record<SignalType, [string, string]> = {
  arb: ["No arbs right now", "Turning more books on finds more."],
  ev: ["Nothing open", "Value bets show up when one book is slow to move."],
  line: ["No line gaps right now", "Gaps appear when books disagree on the line itself."],
  move: ["No steam moves", "Sharp price moves across books show up here."],
}

/** Arbitrage, value bets, line gaps or steam moves, with filters and a 10-per-page list. */
export function SignalsPage({ type, open }: { type: SignalType; open?: string }) {
  useStore()
  if (S.boot === "error") return <ApiError />
  if (S.boot === "loading") return <Loading label="Connecting to the API" />
  return <SignalsView key={type} type={type} open={open} />
}

/** Hides kick-offs more than 12 hours away behind the day name. */
function When({ f }: { f: Fixture }) {
  const m = minutesTo(f.ko)
  return m >= -120 && m < 720 ? <Countdown f={f} /> : <>{dayTab(dayOf(f.ko))}</>
}

/** A leg's label, with the team name shortened to its tag and crest. */
function LegText({ l, f }: { l: SignalLeg; f: Fixture }) {
  for (const team of [f.hT, f.aT]) {
    if (l.lab === team.name || l.lab.startsWith(team.name + " ")) {
      return (
        <>
          <Crest team={team} size={14} />
          {teamTag(team.name)[0] + l.lab.slice(team.name.length)}
        </>
      )
    }
  }
  return <>{l.lab}</>
}

function filterRows(list: SignalItem[], type: SignalType, sport: string, min: number) {
  let out = list.filter((s) => sport === "all" || s.f.sp === sport)
  if (min && type === "arb") out = out.filter((s) => arbPct(s) >= min)
  if (min && type === "ev") out = out.filter((s) => s.ev * 100 >= min)
  return out
}

function sortRows(list: SignalItem[], type: SignalType, sort: Sort) {
  const ko = (s: SignalItem) => s.f.ko
  if (sort === "time") return list.sort((x, y) => ko(x) - ko(y))
  if (type === "arb") return list.sort((x, y) => arbPct(y) - arbPct(x))
  if (type === "ev") return list.sort((x, y) => y.ev - x.ev)
  if (type === "move") return list.sort((x, y) => Math.abs(y.margin) - Math.abs(x.margin))
  return list.sort((x, y) => y.at - x.at)
}

function SignalsView({ type, open }: { type: SignalType; open?: string }) {
  const router = useRouter()
  const { openSheet } = useShell()
  const F = useOdds()
  const anchor = useRef<HTMLSpanElement>(null)
  const [sort, setSortState] = useState<Sort>(carry.sort)
  const [sport, setSportState] = useState(carry.sport)
  const [min, setMin] = useState(0)
  const [openId, setOpenId] = useState<string | null>(open ?? null)
  const [bank, setBank] = useState("10000")
  const [round, setRound] = useState(10)

  useEffect(() => {
    clearTimeout(carryReset)
    return () => {
      carryReset = setTimeout(() => { carry.sort = "profit"; carry.sport = "all" }, 0)
    }
  }, [])

  const setSort = (v: Sort) => { carry.sort = v; setSortState(v) }
  const setSport = (v: string) => { carry.sport = v; setSportState(v) }

  const all = signalList(type)
  const on = onList()
  const sports = (S.sports.length ? S.sports.map((s) => s.sport) : SPORTS.map((s) => s.id as string)).filter((id) => all.some((s) => s.f.sp === id))
  const rows = sortRows(filterRows(all, type, sport, min), type, sort)
  const page = usePage(rows, ["sig", type, sport, min, sort].join("|"))

  const openNow = openId === null && type === "arb" ? page.items[0]?.id ?? null : openId
  if (openId === null && type === "arb" && page.items.length) setOpenId(page.items[0].id)

  const lineIds = [...new Set(page.items.filter((s) => s.key.playerId && !S.pname[s.key.playerId]).map((s) => s.f.id))].join(",")
  useEffect(() => {
    if (lineIds) lineIds.split(",").forEach(wantProps)
  }, [lineIds])

  const toggle = (id: string) => {
    const next = openNow === id ? "" : id
    setOpenId(next)
    try {
      window.history.replaceState(window.history.state, "", routes.signals(type, next || undefined))
    } catch {}
  }
  const Icon = SIGNAL_ICONS[type]
  const minimums = MINIMUMS[type]

  const head = (
    <div className={styles.hd}>
      <span className={styles.spi}><Icon /></span>
      <h1>{SIGNALS[type].title}</h1>
      {hasSignalTour(type) && (
        <button className={styles.bk + " " + styles.how} onClick={replaySignalTour}>How it works</button>
      )}
      <button className={styles.bk + " " + styles.books} onClick={openSheet} data-tour="books">
        <span className={styles.stack}>{on.slice(0, 3).map((s) => <BookLogo key={s.name} name={s.name} size={20} />)}</span>
        {on.length} books
      </button>
    </div>
  )

  const tabs = (
    <FitRow className="mtabs" role="tablist" label="Signal type" tabs tour="signal-nav">
      {SIGNAL_TYPES.map((t) => (
        <button key={t} role="tab" aria-selected={type === t} onClick={() => router.push(routes.signals(t))} onMouseEnter={() => router.prefetch(routes.signals(t))}>
          {SIGNALS[t].title} <span className="mono faint">{signalList(t).length}</span>
        </button>
      ))}
    </FitRow>
  )

  const filters = (
    <FitRow className={"chips " + styles.fchips} role="group" label="Filter">
      <div className="seg" role="group" aria-label="Sort">
        <button aria-pressed={sort !== "time"} onClick={() => setSort("profit")}>{SORT_LABEL[type]}</button>
        <button aria-pressed={sort === "time"} onClick={() => setSort("time")}>Kick-off</button>
      </div>
      {minimums && (
        <div className="seg" role="group" aria-label="Minimum">
          {minimums.map(([v, label]) => (
            <button key={v} aria-pressed={min === v} onClick={() => setMin(v)}>{label}</button>
          ))}
        </div>
      )}
      <InfoTip label={SIGNALS[type].title} text={SIGNAL_INFO[type]} />
      <span className={styles.fsp} />
      <button className="chip sm" aria-pressed={sport === "all"} onClick={() => setSport("all")}>All</button>
      {sports.map((id) => (
        <button key={id} className="chip sm" aria-pressed={sport === id} onClick={() => setSport(id)}>
          <SportIcon sport={id} size={14} />
          {sportName(id)}
        </button>
      ))}
    </FitRow>
  )

  const telegram = (
    <section className={"card " + styles.tg} aria-label="Telegram">
      <TelegramIcon />
      <span className={styles.tx}>Telegram alerts<span>/arbs 1.0 · /ev 5</span></span>
      <a className="btn sm" href="#" role="button" onClick={(e) => e.preventDefault()}>Open bot</a>
    </section>
  )

  const none = signalsPending() ? <Loading label="Loading signals" /> : (
    <div className="empty">
      <p>{NONE[type][0]}</p>
      <p className="faint">{NONE[type][1]}</p>
    </div>
  )

  const pager = <Pager page={page.page} total={page.total} onPage={page.setPage} label={SIGNALS[type].title} anchor={anchor} />

  const list = (body: ReactNode) =>
    page.items.length ? (
      <>
        <span className={styles.pgTop} ref={anchor} />
        {body}
        {pager}
      </>
    ) : (
      none
    )

  const whenCell = (f: Fixture) => (
    <span className={styles.arWhen}>
      <b className="mono">{f.t}</b>
      <span className={minutesTo(f.ko) <= 60 ? styles.soon : undefined}><When f={f} /></span>
    </span>
  )

  const legs = (s: SignalItem, price: (l: SignalLeg) => string) => (
    <span className={styles.arLegs} data-tour="arb-legs">
      {s.legs.map((l, i) => (
        <span key={i} className={styles.arLeg}>
          <span className={styles.o}><LegText l={l} f={s.f} /></span>
          <b className="mono">{price(l)}</b>
          <span className={styles.bk2}><BookLink name={l.src} f={s.f} deep={l.deep} size={13}>{l.src}</BookLink></span>
        </span>
      ))}
    </span>
  )

  const crests = (f: Fixture) => (
    <span className={styles.pairCr}>
      <Crest team={f.hT} size={22} />
      <Crest team={f.aT} size={22} />
    </span>
  )

  if (type === "arb") {
    const best = all.length ? Math.max(...all.map(arbPct)) : 0
    const avg = all.length ? all.reduce((s, a) => s + arbPct(a), 0) / all.length : 0
    return (
      <>
        {head}
        {tabs}
        <div className={styles.astats} data-tour="arb-stats">
          <div><span>Open now</span><b className="mono">{all.length}</b></div>
          <div><span>Best <InfoTip label="Best arb" text={TERM.arbProfit} /></span><b className={"mono " + styles.g}>+{best.toFixed(2)}%</b></div>
          <div><span>Average</span><b className="mono">+{avg.toFixed(2)}%</b></div>
          <div><span>Books scanned <InfoTip label="Books scanned" text={TERM.booksScanned} /></span><b className="mono">{onCount()}</b></div>
        </div>
        {filters}
        {list(
          <div className={styles.alist} data-tour="signals-list">
            <div className={styles.ahead}><span>Start</span><span>Match</span><span>Back each outcome at</span><span>Profit <InfoTip label="Profit" text={TERM.arbProfit} /></span></div>
            {page.items.map((a) => {
              const f = a.f, isOpen = openNow === a.id
              return (
                <div key={a.id} className={styles.ait + (isOpen ? " " + styles.open : "")}>
                  <div className={styles.arow}>
                    <button className="tap-area" aria-expanded={isOpen} aria-label={`${f.h} v ${f.a}, ${signalMarket(a)}, arb ${arbPct(a).toFixed(2)}%`} onClick={() => toggle(a.id)} />
                    {whenCell(f)}
                    <span className={styles.arM}>
                      {crests(f)}
                      <span className={styles.tx}>
                        <b>{f.h} v {f.a}</b>
                        <span><LeagueMark code={f.lg} size={13} />{leagueName(f.lg)} · {signalMarket(a)}</span>
                      </span>
                    </span>
                    {legs(a, (l) => F(l.d))}
                    <span className={styles.arP}><b className="mono">+{arbPct(a).toFixed(2)}%</b><span>{ago(a.at)} ago</span></span>
                  </div>
                  {isOpen && <ArbCalc a={a} bank={bank} round={round} onBank={setBank} onRound={setRound} />}
                </div>
              )
            })}
          </div>,
        )}
        {telegram}
        <SignalTour type="arb" ready={page.items.length > 0} />
      </>
    )
  }

  if (type === "ev") {
    return (
      <>
        {head}
        {tabs}
        {filters}
        {list(
          <div className={styles.list + " " + styles.evl} data-tour="signals-list">
            {page.items.map((e) => {
              const f = e.f, l = e.legs[0], isOpen = openNow === e.id
              return (
                <div key={e.id} className={styles.ait + (isOpen ? " " + styles.open : "")}>
                  <div className={styles.sig}>
                    <button className="tap-area" aria-expanded={isOpen} aria-label={`${l.lab} · ${signalMarket(e)}, ${f.h} v ${f.a}, ${F(l.d)} on ${l.src}, +${(e.ev * 100).toFixed(1)}% EV`} onClick={() => toggle(e.id)} />
                    {crests(f)}
                    <span className={styles.mn}>
                      <b>{l.lab} · {signalMarket(e)}</b>
                      <span><LeagueMark code={f.lg} size={13} />{f.h} v {f.a}</span>
                    </span>
                    <span className={styles.evB}><BookLink name={l.src} f={f} deep={l.deep} size={14} /><b className="mono">{F(l.d)}</b></span>
                    <span className={"mono " + styles.evE}>+{(e.ev * 100).toFixed(1)}%</span>
                  </div>
                  {isOpen && (
                    <div className={styles.evD} data-tour="ev-detail">
                      <div className={styles.evS}>
                        <div><span>{l.src} implies <InfoTip label="Implied chance" text={TERM.impliedChance} /></span><b className="mono">{l.d ? (100 / l.d).toFixed(1) + "%" : "–"}</b></div>
                        <div><span>Fair chance <InfoTip label="Fair chance" text={TERM.fairChance} /></span><b className={"mono " + styles.g}>{(e.fair * 100).toFixed(1)}%</b></div>
                        <div><span>Fair price <InfoTip label="Fair price" text={TERM.fairPrice} /></span><b className="mono">{e.fair ? F(1 / e.fair) : "–"}</b></div>
                        <div><span>Kick-off</span><b className="mono">{f.t} <i><When f={f} /></i></b></div>
                      </div>
                      <div className={styles.evA} data-tour="ev-actions">
                        <Link className="btn sm" href={routes.event(f.id)}>All prices</Link>
                        {webHref(l.deep) && (
                          <a className="btn sm primary" href={webHref(l.deep)} target="_blank" rel="noopener noreferrer">
                            <BookLogo name={l.src} size={16} />Bet on {l.src} <ExternalIcon />
                          </a>
                        )}
                      </div>
                    </div>
                  )}
                </div>
              )
            })}
          </div>,
        )}
        {telegram}
        <SignalTour type="ev" ready={page.items.length > 0} />
      </>
    )
  }

  if (type === "line") {
    return (
      <>
        {head}
        {tabs}
        {filters}
        {list(
          <div className={styles.alist} data-tour="signals-list">
            <div className={styles.ahead}><span>Start</span><span>Market</span><span>Lines</span><span>Gap <InfoTip label="Gap" text={TERM.lineGap} /></span></div>
            {page.items.map((s) => {
              const f = s.f
              const ns = s.legs.map((l) => (l.price.form === "line_only" ? l.price.raw : numberIn(l.lab)))
              const gap = ns.length > 1 && ns.every((x) => x != null) ? Math.abs(Math.abs(ns[0] as number) - Math.abs(ns[1] as number)) : null
              return (
                <div key={s.id} className={styles.ait}>
                  <div className={styles.arow + " " + styles.lrow}>
                    {whenCell(f)}
                    <Link className={styles.arM} href={routes.event(f.id, s.key.playerId ? "PROPS" : undefined)}>
                      {crests(f)}
                      <span className={styles.tx}>
                        <b>{signalMarket(s)}</b>
                        <span><LeagueMark code={f.lg} size={13} />{f.h} v {f.a}</span>
                      </span>
                    </Link>
                    {legs(s, (l) => (l.price.form === "line_only" ? "line" : F(l.d)))}
                    <span className={styles.arP}><b className="mono">{gap != null ? "Gap " + gap.toFixed(1) : "Gap"}</b><span>{ago(s.at)} ago</span></span>
                  </div>
                </div>
              )
            })}
          </div>,
        )}
        {telegram}
      </>
    )
  }

  return (
    <>
      {head}
      {tabs}
      {filters}
      {list(
        <div className={styles.list + " " + styles.evl} data-tour="signals-list">
          {page.items.map((s) => {
            const f = s.f, l = s.legs[0], isOpen = openNow === s.id, mv = s.margin
            return (
              <div key={s.id} className={styles.ait + (isOpen ? " " + styles.open : "")}>
                <div className={styles.sig}>
                  <button className="tap-area" aria-expanded={isOpen} aria-label={`${l.lab} · ${signalMarket(s)}, ${f.h} v ${f.a}, ${F(l.d)} on ${l.src}`} onClick={() => toggle(s.id)} />
                  {crests(f)}
                  <span className={styles.mn}>
                    <b>{l.lab} · {signalMarket(s)}</b>
                    <span><LeagueMark code={f.lg} size={13} />{f.h} v {f.a} · {ago(s.at)} ago</span>
                  </span>
                  <span className={styles.evB}><BookLink name={l.src} f={f} deep={l.deep} size={14} /><b className="mono">{F(l.d)}</b></span>
                  <span className={"mono " + styles.evE + " " + styles.mv + (mv < 0 ? " " + styles.sh : "")}>
                    {mv ? (mv < 0 ? "▼ " : "▲ ") + Math.abs(mv * 100).toFixed(1) + "%" : "Moved"}
                  </span>
                </div>
                {isOpen && <MoveDetail s={s} />}
              </div>
            )
          })}
        </div>,
      )}
      {telegram}
    </>
  )
}
