"use client"

import Link from "next/link"
import { useEffect, useRef, useState } from "react"
import { api } from "@/lib/api/client"
import type { BaysedFlag, BaysedLadderLine, BaysedMarket, BaysedQuote, BaysedReport, BaysedSide } from "@/lib/api/types"
import { decOf } from "@/lib/odds/format"
import { marketLabel, periodLabel, sideName } from "@/lib/odds/markets"
import { routes } from "@/lib/routes"
import { ChevronDownIcon, ChevronUpIcon } from "@/components/mybestbet/shared/icons"
import { BookLogo } from "@/components/mybestbet/shared/logos"
import { InfoTip } from "@/components/mybestbet/shared/info-tip"
import { Pager, usePage } from "@/components/mybestbet/shared/pager"
import { Loading, errorCopy } from "@/components/mybestbet/shared/status"
import { useOdds } from "@/components/mybestbet/shared/use-odds"
import props from "@/components/mybestbet/match/props.module.css"
import styles from "./baysed-page.module.css"

type View = "flagged" | "all" | "only"
type Odds = (d: number | null | undefined) => string

const BAYSE = "bayse"
const REFRESH_MS = 30_000
const GAPS = [0.03, 0.05, 0.1]
const WINDOWS: [number, string][] = [[24, "24h"], [72, "3 days"], [168, "7 days"]]
const VIEWS: [View, string][] = [["flagged", "Flagged"], ["all", "Compared"], ["only", "Only on Bayse"]]

const FLAG_TEXT: Record<BaysedFlag, string> = {
  beats_books: "Beats every book",
  beats_harder_line: "Beats a harder line",
  off_consensus: "Off fair price",
}

const FLAG_INFO = "Beats every book: Bayse pays more than any other source on the same line. Beats a harder line: Bayse pays more than a book does on a line that is harder to hit, so Bayse's price can't be right. Off fair price: Bayse's implied chance is at least the threshold away from the books' margin-free chance at Bayse's line. When no book offers that line, it is taken between their nearest lines or estimated from the line they balance at."

const FAIR_HOW: Record<string, string> = {
  exact: " from the books on this line",
  interpolated: " between the books' nearest lines",
  poisson: " estimated from the line the books balance at",
}

const kickoff = (iso: string) => new Date(iso).toLocaleString(undefined, { weekday: "short", hour: "2-digit", minute: "2-digit" })

const lineText = (line?: number) => (line == null ? "" : String(line))

function title(m: BaysedMarket) {
  const market = marketLabel(m.market, m.sport) + (m.period ? " · " + periodLabel(m.period, m.sport) : "")
  const what = market + (m.line != null ? " " + lineText(m.line) : "")
  return m.player ? { who: m.player, what } : { who: m.home + " v " + m.away, what }
}

function sideText(m: BaysedMarket, s: { side: string; label: string }, line = m.line) {
  if (s.label) return s.label
  if (s.side === "over") return "Over"
  if (s.side === "under") return "Under"
  if (s.side === "yes") return "Yes"
  if (s.side === "no") return "No"
  return sideName(s.side, m.market, line, m.home, m.away)
}

function bookLines(m: BaysedMarket) {
  const lines = m.ladder.filter((l) => l.selections.some((s) => Object.keys(s.cells).some((src) => src !== BAYSE))).map((l) => l.line)
  if (!lines.length) return null
  const known = lines.filter((l): l is number => l != null)
  if (!known.length) return "Same market"
  const lo = Math.min(...known), hi = Math.max(...known)
  return lo === hi ? "Line " + lo : "Lines " + lo + "–" + hi
}

/** Every Bayse line against the lines and prices the other books offer for the same market. */
export function BaysedPage() {
  const [gap, setGap] = useState(0.05)
  const [hours, setHours] = useState(72)
  const [view, setView] = useState<View>("flagged")
  const [open, setOpen] = useState("")
  const [report, setReport] = useState<BaysedReport | null>(null)
  const [error, setError] = useState<unknown>(null)
  const top = useRef<HTMLSpanElement>(null)

  useEffect(() => {
    let live = true
    const load = () =>
      api<BaysedReport>(`/v1/baysed?gap=${gap}&hours=${hours}&all=true`).then(
        (r) => {
          if (!live) return
          setReport(r)
          setError(null)
        },
        (e) => live && setError(e),
      )
    load()
    const t = setInterval(load, REFRESH_MS)
    return () => {
      live = false
      clearInterval(t)
    }
  }, [gap, hours])

  const items = report?.items || []
  const rows = items.filter((m) => (view === "only" ? !m.books.length : view === "flagged" ? m.flags.length > 0 : m.books.length > 0))
  const page = usePage(rows, "baysed|" + view + gap + hours)
  const flagged = items.filter((m) => m.flags.length > 0).length

  return (
    <>
      <div className={styles.intro}>
        <h1>Baysed</h1>
        <p className="muted">Every Bayse line, match and player alike, checked against the lines and prices the other books offer for the same market. A flag means Bayse is paying more than the books say it should.</p>
        {report && (
          <span className={"row muted " + styles.sum}>
            <span className={"dot" + (flagged ? " amber" : "")} />
            {flagged} flagged · {report.compared} compared · {report.unmatched} only on Bayse · {report.books.length} books
          </span>
        )}
      </div>

      <div className={styles.bar}>
        <div className="seg" role="group" aria-label="Show">
          {VIEWS.map(([v, label]) => (
            <button key={v} aria-pressed={view === v} onClick={() => setView(v)}>{label}</button>
          ))}
        </div>
        <label className={styles.pick}>
          <span>Fair gap</span>
          <select value={gap} onChange={(e) => setGap(Number(e.target.value))}>
            {GAPS.map((g) => <option key={g} value={g}>{g * 100} pts</option>)}
          </select>
        </label>
        <label className={styles.pick}>
          <span>Kick-off within</span>
          <select value={hours} onChange={(e) => setHours(Number(e.target.value))}>
            {WINDOWS.map(([h, label]) => <option key={h} value={h}>{label}</option>)}
          </select>
        </label>
        <InfoTip label="How lines are flagged" text={FLAG_INFO} />
      </div>

      {error && !report ? (
        <div className="empty"><p>{errorCopy(error, "report")}</p></div>
      ) : !report ? (
        <Loading label="Comparing Bayse with the books" />
      ) : rows.length === 0 ? (
        <div className="empty">
          <p>{view === "flagged" ? "Nothing flagged" : "No lines here right now"}</p>
          {view === "flagged" && <p className="faint">Every compared Bayse line is in step with the books.</p>}
        </div>
      ) : (
        <>
          <span className={props.pgTop} ref={top} />
          <div className={props.list}>
            {page.items.map((m) => (
              <MarketRow key={m.keyString} m={m} open={open === m.keyString} onToggle={() => setOpen(open === m.keyString ? "" : m.keyString)} />
            ))}
          </div>
          <Pager page={page.page} total={page.total} onPage={page.setPage} label="Bayse lines" anchor={top} />
        </>
      )}
    </>
  )
}

function MarketRow({ m, open, onToggle }: { m: BaysedMarket; open: boolean; onToggle: () => void }) {
  const t = title(m)
  const lines = bookLines(m)
  const books = m.books.length + " book" + (m.books.length === 1 ? "" : "s")
  return (
    <div className={props.pr + (m.flags.length ? " " + styles.hot : "")}>
      <div className={props.prRow}>
        <button className={props.prWho} aria-expanded={open} onClick={onToggle}>
          <span className={props.whoText}>
            <span className={props.whoName}>{t.who}</span>
            <span className={props.whoMeta}>
              <span>{t.what}</span>
              {m.balancedLine != null && m.balancedLine !== m.line && <span>Books balance at {m.balancedLine}</span>}
              {m.flags.map((f) => <span key={f} className={props.fl + " " + props.a}>{FLAG_TEXT[f]}</span>)}
            </span>
          </span>
        </button>
        <span className={props.hs + " " + props.books}>
          <Link className={styles.ev} href={routes.event(m.eventId)}>{m.player ? m.home + " v " + m.away : "Open match"}</Link>
          <span className={"faint " + props.small}>{kickoff(m.kickoff)} · {lines ? lines + " · " + books : "No book offers it"}</span>
        </span>
        {[0, 1].map((k) => (m.sides[k] ? <SideTile key={k} m={m} s={m.sides[k]} hidden={k > 0} /> : <span key={k} className={props.pb + " " + props.none + (k ? " " + props.hs : "")}>–</span>))}
        <button className={props.prEx} aria-label={(open ? "Hide" : "Show") + " every line"} onClick={onToggle}>
          {open ? <ChevronUpIcon /> : <ChevronDownIcon />}
        </button>
      </div>
      {open && <Ladder m={m} />}
    </div>
  )
}

function SideTile({ m, s, hidden }: { m: BaysedMarket; s: BaysedSide; hidden: boolean }) {
  const F = useOdds()
  const ref = s.best ? { q: s.best, line: m.line } : s.harder ? { q: s.harder, line: s.harderLine } : null
  const body = (
    <>
      <span className={props.l}>{sideText(m, s)}</span>
      <span className={props.v}>{F(decOf(s.bayse))}</span>
      <span className={props.s}>
        {ref ? (
          <>
            <BookLogo name={ref.q.source} size={14} />
            <em>{F(decOf(ref.q))}{ref.line != null && ref.line !== m.line ? " @ " + ref.line : ""}</em>
          </>
        ) : s.fair != null ? (
          <em>fair {F(1 / s.fair)}</em>
        ) : null}
      </span>
    </>
  )
  const cls = [props.pb, styles.tile, s.flags.length ? styles.bad : "", hidden ? props.hs : ""].filter(Boolean).join(" ")
  const label = `Bayse ${sideText(m, s)} ${F(decOf(s.bayse))}` + (ref ? ` against ${ref.q.source} ${F(decOf(ref.q))}` + (ref.line != null ? " at " + ref.line : "") : "")
  return s.bayse.deepLink ? (
    <a className={cls} href={s.bayse.deepLink} target="_blank" rel="noopener noreferrer" aria-label={"Open " + label} title={label}>{body}</a>
  ) : (
    <div className={cls} title={label}>{body}</div>
  )
}

function Ladder({ m }: { m: BaysedMarket }) {
  const F = useOdds()
  const sources = [BAYSE, ...m.books]
  const sides = m.sides
  const away = (l: BaysedLadderLine) => (l.line == null || m.line == null ? 0 : Math.abs(l.line - m.line))
  const lines = m.ladder.toSorted((a, b) => away(a) - away(b))
  return (
    <div className={styles.ladder}>
      <div className={styles.scroll}>
        <table>
          <thead>
            <tr>
              <th>{m.ladder.some((l) => l.line != null) ? "Line" : ""}</th>
              <th></th>
              {sources.map((src) => (
                <th key={src} className={src === BAYSE ? styles.me : undefined}>
                  <span className={styles.src}><BookLogo name={src} size={14} />{src}</span>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {lines.flatMap((l) =>
              sides.flatMap((side, i) => {
                const sel = l.selections.find((x) => x.side === side.side && x.label === side.label)
                if (!sel) return []
                return (
                  <tr key={lineText(l.line) + side.side + side.label} className={[l.line === m.line ? styles.mine : "", i === 0 ? styles.first : ""].join(" ")}>
                    <td className="mono">{i === 0 || !l.selections.some((x) => x.side === sides[0].side && x.label === sides[0].label) ? lineText(l.line) : ""}</td>
                    <td className="muted">{sideText(m, side, l.line)}</td>
                    {sources.map((src) => (
                      <td key={src} className={src === BAYSE ? styles.me : undefined}>
                        {sel?.cells[src] ? <QuoteCell q={sel.cells[src]} F={F} /> : <span className="faint">–</span>}
                      </td>
                    ))}
                  </tr>
                )
              }),
            )}
          </tbody>
        </table>
      </div>
      <SideNotes m={m} F={F} />
    </div>
  )
}

function QuoteCell({ q, F }: { q: BaysedQuote; F: Odds }) {
  const text = <span className="mono">{F(decOf(q))}</span>
  return q.deepLink ? <a href={q.deepLink} target="_blank" rel="noopener noreferrer">{text}</a> : text
}

function SideNotes({ m, F }: { m: BaysedMarket; F: Odds }) {
  const notes = m.sides.flatMap((s) => {
    const out: string[] = []
    const name = sideText(m, s) + (m.line != null ? " " + m.line : "")
    if (s.flags.includes("beats_books") && s.best) out.push(`${name}: Bayse pays ${F(decOf(s.bayse))}, the best book (${s.best.source}) pays ${F(decOf(s.best))}.`)
    if (s.harder && s.harderLine != null) out.push(`${name}: Bayse pays ${F(decOf(s.bayse))}, more than ${s.harder.source} pays (${F(decOf(s.harder))}) for the harder ${sideText(m, s)} ${s.harderLine}.`)
    if (s.fair != null && s.gap != null) out.push(`${name}: fair price ${F(1 / s.fair)}${FAIR_HOW[s.fairModel || "exact"]}; Bayse is ${Math.abs(s.gap * 100).toFixed(1)} pts ${s.gap > 0 ? "too generous" : "too stingy"}.`)
    return out
  })
  if (!notes.length) return null
  return <ul className={styles.notes}>{notes.map((n) => <li key={n}>{n}</li>)}</ul>
}
