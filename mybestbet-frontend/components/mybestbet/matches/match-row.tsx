"use client"

import Link from "next/link"
import { useState } from "react"
import { useRouter } from "next/navigation"
import { Countdown, useNow } from "@/components/mybestbet/shared/countdown"
import { ChevronDownIcon, ChevronRightIcon, StarIcon } from "@/components/mybestbet/shared/icons"
import { InfoTip } from "@/components/mybestbet/shared/info-tip"
import { Crest } from "@/components/mybestbet/shared/logos"
import type { MarketBoard, MarketShape } from "@/lib/api/types"
import { decOf, formatLine, numberIn } from "@/lib/odds/format"
import { marketInfo, regTag } from "@/lib/odds/glossary"
import { mainMarket, marketLabel } from "@/lib/odds/markets"
import { liveClock } from "@/lib/odds/time"
import { routes } from "@/lib/routes"
import { S, boardBest, isLoading, isQueued, prefetchEvent, want, type Fixture } from "@/lib/store/data"
import { useKey } from "@/lib/store/emitter"
import { prefs, toggleStar } from "@/lib/store/prefs"
import { chances, listBest, mainKey, rowBoard } from "@/lib/store/selectors"
import { BookStack, rankBooks } from "./book-stack"
import { PriceButton, type RowPrice, type RowSel } from "./price-button"
import styles from "./match-row.module.css"

/** The three or two main market selections, before the match detail has loaded. */
export function mainSels(f: Fixture): RowSel[] {
  const s: RowSel[] = [{ label: f.h, short: "1" }]
  if (f.sp === "football") s.push({ label: "Draw", short: "X" })
  s.push({ label: f.a, short: "2" })
  return s
}

/** Short tag shown in the corner of a price: "1", "X", "O 2.5", "+1.5", "Yes". */
export function shortLabel(s: { side?: string; label: string }, shape: MarketShape) {
  const n = numberIn(s.label)
  if (s.side === "home") return shape === "handicap" && n != null ? formatLine(n, true) : "1"
  if (s.side === "away") return shape === "handicap" && n != null ? formatLine(n, true) : "2"
  if (s.side === "draw") return "X"
  if (s.side === "over") return "O " + (n ?? "")
  if (s.side === "under") return "U " + (n ?? "")
  if (s.side === "yes") return "Yes"
  if (s.side === "no") return "No"
  return s.label.length > 12 ? s.label.slice(0, 11) + "…" : s.label
}

const pct = (p: number[] | null, i: number) => (p ? Math.round(p[i] * 100) + "%" : "")

/** Kick-off time with a countdown for the next 12 hours, amber within the hour. */
function When({ f }: { f: Fixture }) {
  const now = useNow()
  const sc = f.status === "live" ? f.score : undefined
  if (sc) return <span className={styles.live}>{liveClock(sc)}</span>
  const m = (f.ko - now) / 60000
  const show = (f.status && f.status !== "scheduled" && f.status !== "live") || m < 720
  return <span className={m <= 60 ? styles.soon : ""} suppressHydrationWarning>{show ? <Countdown f={f} /> : null}</span>
}

export function MatchRow({ f, code, custom, onMarket, tour }: {
  f: Fixture
  /** The market this row shows: its own pick or the list's. */
  code: string
  /** Whether the row has its own market pick. */
  custom: boolean
  onMarket: (id: string, code: string) => void
  /** Mark the first price for the onboarding tour. */
  tour?: boolean
}) {
  useKey(f.id)
  const router = useRouter()
  const [only, setOnly] = useState<string | null>(null)
  const spm = S.sports.find((s) => s.sport === f.sp)
  const feed = spm ? spm.matchMarkets : [mainMarket(f.sp)]
  const main = code === mainMarket(f.sp)
  let sels: RowSel[], best: RowPrice[], key: string
  let board: MarketBoard | null = null
  if (main && !(prefs.lines[f.sp + code] != null && S.det[f.id])) {
    sels = mainSels(f)
    best = listBest(f)
    key = mainKey(f)
    board = S.det[f.id]?.by[key]?.b || null
  } else {
    const rb = rowBoard(f, code)
    if (rb) {
      sels = rb.b.selections.map((s) => ({ label: s.label, short: shortLabel(s, rb.m.shape) }))
      best = boardBest(rb.b)
      key = rb.b.keyString
      board = rb.b
    } else {
      want(f.id)
      sels = main ? mainSels(f) : [{ label: "", short: "" }, { label: "", short: "" }]
      best = main ? listBest(f) : []
      key = main ? mainKey(f) : ""
    }
  }
  const pending = isLoading("det:" + f.id) || isQueued(f.id)
  const p = chances(f)
  const b = f.badges || { arbs: 0, evs: 0 }
  const tags = !!(b.arbs || b.evs || f.status === "live")
  const href = routes.event(f.id, code)
  const intent = () => {
    prefetchEvent(f.id)
    router.prefetch(href)
  }
  const more = (
    <>
      +{feed.length - 1} markets
      <ChevronRightIcon />
    </>
  )
  const starred = !!prefs.starred[f.id]
  const sc = f.status === "live" || f.status === "finished" ? f.score : undefined
  const markAt = tour ? sels.findIndex((_, i) => !!best[i]?.d) : -1
  const books = rankBooks(f, board, best)
  const tag = regTag(code, f.sp)
  const about = [marketInfo(code, f.sp), tag?.text].filter(Boolean).join(" ")
  const pinned = !!only && books.includes(only)
  if (pinned) {
    if (board) {
      best = board.selections.map((s) => {
        const c = s.cells[only]
        const d = c && !c.derived && !c.price.suspended ? decOf(c.price) : null
        return d ? { d, src: only, deep: c.deepLink } : null
      })
    } else {
      want(f.id)
      best = sels.map(() => null)
    }
  }

  return (
    <article className={styles.row}>
      <Link className={styles.time} href={href} prefetch={false} tabIndex={-1} onPointerEnter={intent} onTouchStart={intent}>
        <b>{f.t}</b>
        <When f={f} />
      </Link>
      <Link className={styles.teams} href={href} prefetch={false} aria-label={`${f.h} v ${f.a}, all markets`} onPointerEnter={intent} onTouchStart={intent} onFocus={intent}>
        <span className={styles.team}>
          <Crest team={f.hT} />
          <b>{f.h}</b>
          {sc ? <i className={styles.score}>{sc.home}</i> : <i>{pct(p, 0)}</i>}
        </span>
        <span className={styles.team}>
          <Crest team={f.aT} />
          <b>{f.a}</b>
          {sc ? <i className={styles.score}>{sc.away}</i> : <i>{p ? pct(p, p.length - 1) : ""}</i>}
        </span>
      </Link>
      <div className={styles.odds} style={{ "--n": sels.length } as React.CSSProperties}>
        {sels.map((s, i) => (
          <PriceButton key={i} f={f} sel={s} i={i} x={best[i]} marketKey={key} code={code} pending={pending || (pinned && !board)} tour={i === markAt} pinned={pinned} />
        ))}
      </div>
      <div className={styles.foot + (tags || books.length ? "" : " " + styles.nt)}>
        {b.arbs ? <span className="badge arb">Arb {((b.maxMargin || 0) * 100).toFixed(2)}%</span> : null}
        {b.evs ? <span className="badge ev">+EV {((b.maxEv || 0) * 100).toFixed(1)}%</span> : null}
        {f.status === "live" ? <span className="badge soft">Live</span> : null}
        <BookStack books={books} market={marketLabel(code, f.sp)} only={pinned ? only : null} onPick={setOnly} />
        <Link className={styles.more} href={href} prefetch={false} onPointerEnter={intent} onFocus={intent}>{more}</Link>
      </div>
      <div className={styles.ctrl}>
        <label className={styles.mk + (custom ? " " + styles.on : "")}>
          <span className="sr">Market for {f.h} v {f.a}</span>
          <select value={code} onChange={(e) => onMarket(f.id, e.target.value)}>
            {feed.map((k) => (
              <option key={k} value={k}>{marketLabel(k, f.sp)}</option>
            ))}
          </select>
          <ChevronDownIcon />
        </label>
        {about && <InfoTip label={marketLabel(code, f.sp)} text={about} />}
        <Link className={styles.more + " " + styles.mob} href={href} prefetch={false} onTouchStart={intent}>{more}</Link>
        <button className={styles.star} aria-pressed={starred} aria-label="Favourite" onClick={() => toggleStar(f.id)}>
          <StarIcon fill={starred} />
        </button>
      </div>
    </article>
  )
}
