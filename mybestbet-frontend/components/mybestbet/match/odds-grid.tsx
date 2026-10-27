"use client"

import { useEffect, useRef, useState } from "react"
import type { BoardCell, MarketBoard, MarketShape, MarketTab, SelectionRow } from "@/lib/api/types"
import { teamTag } from "@/lib/catalog"
import { decOf, depth, formatLine, numberIn } from "@/lib/odds/format"
import { TERM, marketInfo, regTag } from "@/lib/odds/glossary"
import { marketLabel, periodLabel, sideName } from "@/lib/odds/markets"
import { isDown, isShown, S, sourceInfo, type Fixture } from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { inSlip, togglePick } from "@/lib/store/prefs"
import { defaultBoard, fairOdds, lineLabel } from "@/lib/store/selectors"
import { HistoryIcon } from "@/components/mybestbet/shared/icons"
import { InfoTip } from "@/components/mybestbet/shared/info-tip"
import { BookLink, Crest } from "@/components/mybestbet/shared/logos"
import { Pager, usePage } from "@/components/mybestbet/shared/pager"
import { useOdds } from "@/components/mybestbet/shared/use-odds"
import { PriceHistory } from "./price-history"
import styles from "./odds-grid.module.css"

/** "up" or "dn" for a price that moved in the last 1.5s, otherwise "". */
export function flashDir(key: string) {
  const x = S.flash[key]
  return x && Date.now() - x.at < 1500 ? (x.dir > 0 ? "up" : "dn") : ""
}

/** Re-render 1.6s after a render that showed a flash, so the up/down colour clears. */
export function useFlashClear(active: boolean) {
  const [, tick] = useState(0)
  useEffect(() => {
    if (!active) return
    const id = setTimeout(() => tick((n) => n + 1), 1600)
    return () => clearTimeout(id)
  })
}

/** Short label for a selection: 1, X, 2, O 2.5, −1.5 and so on. */
export function shortLabel(s: SelectionRow, shape: MarketShape) {
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

export const selectionName = (s: SelectionRow, f: Pick<Fixture, "h" | "a">, code = "", line?: number | null) => s.label || sideName(s.side, code, line, f.h, f.a)

type Current = { m: MarketTab; pi: number; bi: number; b: MarketBoard }

function currentBoard(f: Fixture, code: string, per: number, line: number): Current | null {
  const det = S.det[f.id]
  if (!det) return null
  const m = det.d.markets.find((x) => x.code === code)
  if (!m || !m.periods.length) return null
  const pi = Math.min(Math.max(0, per), m.periods.length - 1), period = m.periods[pi]
  if (!period.boards.length) return null
  const bi = line >= 0 ? Math.min(line, period.boards.length - 1) : defaultBoard(period.boards, m.shape)
  return { m, pi, bi, b: period.boards[bi] }
}

type Hist = { key: string; src: string } | null

function teamsOf(boards: MarketBoard[]) {
  const ids: string[] = []
  for (const b of boards) if (b.key.teamId && !ids.includes(b.key.teamId)) ids.push(b.key.teamId)
  return ids
}

export function MarketBlock({ f, code, mode, onMode }: { f: Fixture; code: string; mode: "listed" | "fair"; onMode: (m: "listed" | "fair") => void }) {
  useStore()
  const [per, setPer] = useState(0)
  const [line, setLine] = useState(-1)
  const [hist, setHist] = useState<Hist>(null)
  const cb = currentBoard(f, code, per, line)
  if (!cb) return <div className="empty"><p>No prices for this market yet</p></div>
  const { m, pi, bi } = cb
  const period = m.periods[pi]
  const teams = teamsOf(period.boards).sort((a, z) => +(a !== f.hT.id) - +(z !== f.hT.id))
  const team = cb.b.key.teamId || ""
  const lines = period.boards.map((b, k) => ({ b, k })).filter(({ b }) => teams.length < 2 || (b.key.teamId || "") === team)
  const pickTeam = (id: string) => {
    const own = period.boards.map((b, k) => ({ b, k })).filter(({ b }) => b.key.teamId === id)
    setLine(own[defaultBoard(own.map((x) => x.b), m.shape)]?.k ?? -1)
    setHist(null)
  }
  const label = marketLabel(code, f.sp)
  const info = marketInfo(code, f.sp)
  const tag = regTag(code, f.sp)
  const about = [info, tag?.text].filter(Boolean).join(" ")
  return (
    <>
      <div className={styles.mhd}>
        <h2>{label}</h2>
        {tag && <span className={styles.rtag}>{tag.label}</span>}
        {about && <InfoTip label={label} text={about} />}
      </div>
      <div className={"row " + styles.mrowL}>
        <span className={styles.mctl}>
          {m.periods.length > 1 && (
            <div className="seg" role="group" aria-label="Period">
              {m.periods.map((p, k) => (
                <button key={k} aria-pressed={k === pi} onClick={() => { setPer(k); setLine(-1); setHist(null) }}>{periodLabel(p.period, f.sp)}</button>
              ))}
            </div>
          )}
          {teams.length > 1 && (
            <div className="seg" role="group" aria-label="Team">
              {teams.map((id) => {
                const tm = id === f.aT.id ? f.aT : f.hT
                return <button key={id} className={styles.tmB} aria-pressed={id === team} onClick={() => pickTeam(id)}><Crest team={tm} size={16} />{teamTag(tm.name)[0]}</button>
              })}
            </div>
          )}
          {lines.length > 1 && (
            <div className="seg" role="group" aria-label="Line">
              {lines.map(({ b, k }) => (
                <button key={b.keyString} aria-pressed={k === bi} onClick={() => { setLine(k); setHist(null) }}>{teams.length > 1 && b.key.line != null ? String(b.key.line) : lineLabel(b, m.shape)}</button>
              ))}
            </div>
          )}
        </span>
        <span className={styles.mode}>
          <div className="seg" role="group" aria-label="Price type">
            <button aria-pressed={mode === "listed"} onClick={() => onMode("listed")}>Listed</button>
            <button aria-pressed={mode === "fair"} onClick={() => onMode("fair")}>No margin</button>
          </div>
          <InfoTip label="No margin" text={TERM.noMargin} />
        </span>
      </div>
      <div className={styles.outs} id="outs">
        <OddsGrid f={f} cb={cb} fair={mode === "fair"} hist={hist} onHist={setHist} />
      </div>
    </>
  )
}

type BookRow = { nm: string; cells: (BoardCell | null)[]; vals: (number | null)[]; mg: number | null; down: boolean; line: boolean }

function OddsGrid({ f, cb, fair, hist, onHist }: { f: Fixture; cb: Current; fair: boolean; hist: Hist; onHist: (h: Hist) => void }) {
  const F = useOdds()
  const top = useRef<HTMLSpanElement>(null)
  const { m, b } = cb
  const sels = b.selections, n = sels.length
  const names = S.sources.map((s) => s.name).concat(Object.keys(sels[0] ? sels[0].cells : {}).filter((x) => !sourceInfo(x)))
  const rows: BookRow[] = names
    .filter((nm) => isShown(nm) && sels.some((s) => s.cells[nm]))
    .map((nm) => {
      const cells = sels.map((s) => s.cells[nm] || null)
      const line = cells.every((c) => !c || c.price.form === "line_only")
      const vals = cells.map((c) => (!c || c.price.form === "line_only" ? null : fair ? fairOdds(c, cells) : decOf(c.price)))
      const ps = cells.map((c) => (c && c.price.form !== "line_only" ? c.price.prob : null))
      const mg = ps.every((x) => x) ? ((ps as number[]).reduce((a, v) => a + v, 0) - 1) * 100 : null
      return { nm, cells, vals, mg, down: isDown(nm), line }
    })
  rows.sort((a, z) => (+a.down - +z.down) || (+a.line - +z.line) || (+(a.mg == null) - +(z.mg == null)) || ((a.mg as number) - (z.mg as number)))
  const best = sels.map((_, i) =>
    Math.max(0, ...rows.filter((r) => !r.down && !r.line).map((r) => {
      const c = r.cells[i]
      return c && !c.derived && !c.price.suspended ? r.vals[i] || 0 : 0
    })),
  )
  const page = usePage(rows, "bk|" + b.keyString)
  const anyDerived = rows.some((r) => r.cells.some((c) => c?.derived))
  const anyDepth = rows.some((r) => r.cells.some((c) => c && depth(c.price)))
  const flashes: string[] = []
  const histOpen = (nm: string) => !!hist && hist.src === nm && hist.key === b.keyString

  const head = sels.map((s, i) => {
    const tm = s.side === "home" ? f.hT : s.side === "away" ? f.aT : null
    const cp = s.consensus ? s.consensus.price.prob : null
    return (
      <span key={"h" + i} className={styles.ogC + " " + styles.ogHc}>
        {tm ? <b><Crest team={tm} size={16} />{m.shape === "handicap" ? shortLabel(s, m.shape) : teamTag(tm.name)[0]}</b> : <b>{selectionName(s, f)}</b>}
        <i>{cp ? (cp * 100).toFixed(0) + "%" : ""}</i>
      </span>
    )
  })

  const body = page.items.map((r) => {
    const on = histOpen(r.nm)
    return [
      <div key={r.nm + "b"} className={[styles.ogC, styles.ogBk, styles.ogHb2, r.down ? styles.down : "", on ? styles.on : ""].filter(Boolean).join(" ")}>
        <button
          className="tap-area"
          title={r.nm + " price history"}
          aria-label={r.nm + " price history"}
          aria-expanded={on}
          onClick={() => onHist(on ? null : { key: b.keyString, src: r.nm })}
        />
        <BookLink name={r.nm} f={f} deep={r.cells.find((c) => c?.deepLink)?.deepLink} size={22}><span>{r.nm}</span></BookLink>
        {r.down && <em className="badge warn">down</em>}
        <i className={styles.hi} aria-hidden="true"><HistoryIcon /></i>
      </div>,
      ...r.cells.map((c, i) => {
        const k = r.nm + "c" + i
        if (!c) return <span key={k} className={styles.ogC + " " + styles.ogNa}>–</span>
        if (c.price.form === "line_only") return <span key={k} className={styles.ogC + " " + styles.ogLn} title="Line only">{c.label || String(c.price.raw)}</span>
        const v = r.vals[i]
        if (c.price.suspended) return <span key={k} className={styles.ogC + " " + styles.ogP + " " + styles.sus} title="Suspended">{F(v)}<small>Suspended <InfoTip label="Suspended" text={TERM.suspended} /></small></span>
        const isBest = !r.down && !c.derived && v && Math.abs(v - best[i]) < 1e-9
        const dp = depth(c.price)
        const fl = flashDir(b.keyString + "#" + i + "#" + r.nm)
        if (fl) flashes.push(fl)
        const cls = [styles.ogC, styles.ogP, isBest && styles.best, r.down && styles.down, c.derived && styles.der, dp && styles.dp, fl && styles[fl], inSlip(f.id, b.keyString, i, r.nm) && styles.on]
          .filter(Boolean).join(" ")
        return (
          <button
            key={k}
            className={cls}
            data-pick={f.id}
            aria-pressed={inSlip(f.id, b.keyString, i, r.nm)}
            aria-label={`${selectionName(sels[i], f)} at ${F(v)} on ${r.nm}`}
            title={c.derived ? "Derived from other markets" : undefined}
            onClick={() => togglePick({ fx: f.id, key: b.keyString, i, code: m.code, label: selectionName(sels[i], f, m.code, b.key.line), src: r.nm })}
          >
            <span className={styles.v}>{F(v)}{c.derived && <i className={styles.dv}>≈</i>}</span>
            {dp && <small>{dp}</small>}
          </button>
        )
      }),
      <span key={r.nm + "m"} className={styles.ogC + " " + styles.ogMg}>{r.mg == null ? "–" : r.mg.toFixed(1) + "%"}</span>,
      on ? <PriceHistory key={r.nm + "h"} b={b} f={f} src={r.nm} onClose={() => onHist(null)} /> : null,
    ]
  })
  useFlashClear(flashes.length > 0)

  return (
    <>
      <span className={styles.pgTop} ref={top} />
      <div className={styles.og} data-tour="odds-grid">
        <div className={styles.ogS}>
          <div className={styles.ogG} style={{ "--n": n } as React.CSSProperties}>
            <span className={[styles.ogC, styles.ogHc, styles.ogHb].join(" ")}>Bookmakers</span>
            {head}
            <span className={[styles.ogC, styles.ogHc, styles.ogMg].join(" ")}>Margin <InfoTip label="Margin" text={TERM.margin} /></span>
            {sels.every((s) => s.consensus) && (
              <>
                <span className={[styles.ogC, styles.ogBk, styles.ogCn].join(" ")}><span className={styles.cnI} aria-hidden="true">≈</span><span>Consensus</span><InfoTip label="Consensus" text={TERM.consensus} className={styles.cnTip} /></span>
                {sels.map((s, i) => <span key={"c" + i} className={styles.ogC + " " + styles.ogCv} title="Fair price across books">{F(1 / s.consensus!.price.prob)}</span>)}
                <span className={styles.ogC + " " + styles.ogMg}>0.0%</span>
              </>
            )}
            {body}
          </div>
        </div>
      </div>
      <p className={styles.legend}>
        <span><i className={styles.lgBest} aria-hidden="true" />Best price<InfoTip label="Best price" text={TERM.best} /></span>
        {sels.every((s) => s.consensus) && <span className={styles.lgCn}><i className={styles.lgDer} aria-hidden="true">≈</i>Consensus<InfoTip label="Consensus" text={TERM.consensus} /></span>}
        {anyDerived && <span><i className={styles.lgDer} aria-hidden="true">≈</i>Derived<InfoTip label="Derived price" text={TERM.derived} /></span>}
        {anyDepth && <span><i className={styles.lgDer} aria-hidden="true">¢</i>Prediction market quote<InfoTip label="Prediction market quote" text={TERM.share} /></span>}
      </p>
      <Pager page={page.page} total={page.total} onPage={page.setPage} label="Bookmakers" anchor={top} />
    </>
  )
}
