import type { BoardCell, MarketBoard, MarketShape, MarketTab, PropRow } from "@/lib/api/types"
import { teamTag } from "@/lib/catalog"
import { decOf, formatLine } from "@/lib/odds/format"
import { LINEY, mainMarket, marketLabel, periodLabel } from "@/lib/odds/markets"
import { S, boardBest, bookHref, isOn, isShown, want, type Fixture, type SignalItem } from "./data"
import { prefs, type SlipPick } from "./prefs"

/** The key of a match's main market board, e.g. "id|1X2|||". */
export const mainKey = (f: Fixture) => [f.id, mainMarket(f.sp), "", "", ""].join("|")

export function listBest(f: Fixture) {
  const best = f.best || []
  if (best.every((x) => !x || isShown(x.src))) return best
  const main = S.det[f.id]?.by[mainKey(f)]?.b
  if (main) return boardBest(main)
  want(f.id)
  return best.map((x) => (x && isShown(x.src) ? x : null))
}

/** Which line to show first: the one priced closest to evens, ignoring team-specific boards. */
export function defaultBoard(boards: MarketBoard[], shape: MarketShape) {
  if (boards.length < 2 || (shape !== "over_under" && shape !== "handicap")) return 0
  let index = 0, bestDistance = 9
  const most = Math.max(...boards.map(bookCount))
  boards.forEach((b, i) => {
    const p = boardProb(b), n = bookCount(b)
    const distance = (p == null ? 2 : Math.abs(p - 0.5)) + (b.key.teamId ? 1 : 0) + (n < most * 0.6 ? 0.5 : 0) - n / 1000
    if (distance < bestDistance) { bestDistance = distance; index = i }
  })
  return index
}

const bookCount = (b: MarketBoard) => Object.keys(b.selections[0]?.cells || {}).length

function boardProb(b: MarketBoard): number | null {
  const [a, z] = b.selections
  if (!a || !z) return null
  if (a.consensus) return a.consensus.price.prob
  const ps = Object.keys(a.cells).flatMap((src) => {
    const x = a.cells[src]?.price, y = z.cells[src]?.price
    return x && y && x.prob > 0 && y.prob > 0 && x.form !== "line_only" ? [x.prob / (x.prob + y.prob)] : []
  })
  return ps.length ? ps.reduce((s, v) => s + v, 0) / ps.length : null
}

export function lineLabel(b: MarketBoard, shape: MarketShape) {
  const line = b.key.line
  const team = b.key.teamId ? (S.tid[b.key.teamId] ? teamTag(S.tid[b.key.teamId].name)[0] + " " : "") : ""
  if (line == null) return team || "Main"
  return team + (shape === "handicap" ? formatLine(line, true) : String(line))
}

/** The board a match list row shows for a market, honouring the line picked in the filter bar. */
export function rowBoard(f: Fixture, code: string): { m: MarketTab; b: MarketBoard } | null {
  const det = S.det[f.id]
  if (!det) return null
  const m = det.d.markets.find((x) => x.code === code)
  if (!m || !m.periods.length) return null
  const per = m.periods.find((p) => !p.period) || m.periods[0]
  const wanted = prefs.lines[f.sp + code]
  let bi = wanted != null ? per.boards.findIndex((b) => b.key.line === wanted) : -1
  if (bi < 0) bi = defaultBoard(per.boards, m.shape)
  return { m, b: per.boards[bi] }
}

/** Distinct lines across loaded matches for a market, when more than one is offered. */
export function listLines(list: Fixture[], code: string) {
  for (const f of list) {
    const det = S.det[f.id]
    if (!det) continue
    const m = det.d.markets.find((x) => x.code === code)
    if (!m) continue
    const per = m.periods.find((p) => !p.period) || m.periods[0]
    const lines = per.boards.filter((b) => b.key.line != null && !b.key.teamId).map((b) => b.key.line as number)
    if (lines.length > 1 && m.shape === "over_under") return lines
  }
  return [] as number[]
}

/** Fair odds for a cell: the API's fair probability, or the book's price with its margin removed. */
export function fairOdds(cell: BoardCell, cells: (BoardCell | null)[]) {
  if (cell.fair) return 1 / cell.fair
  const ps = cells.map((x) => x && x.price.prob)
  if (ps.some((x) => !x)) return null
  const total = (ps as number[]).reduce((a, b) => a + b, 0)
  return total / cell.price.prob
}

/** Implied chances for a list row's main market, margin removed, preferring the consensus. */
export function chances(f: Fixture) {
  const det = S.det[f.id]
  const b = det && det.by[mainKey(f)]?.b
  if (b && b.selections.every((s) => s.consensus)) return b.selections.map((s) => s.consensus!.price.prob)
  if (!f.best || f.best.some((x) => !x || !x.d)) return null
  const q = f.best.map((x) => 1 / (x!.d as number)), sum = q.reduce((a, b) => a + b, 0)
  return q.map((x) => x / sum)
}

/** "Haaland · Shots on target 1.5 · 1st half" */
export function signalMarket(s: SignalItem) {
  const k = s.key, sport = s.f.sp
  const who = k.playerId ? (S.pname[k.playerId]?.name || "Player") + " · " : k.teamId && S.tid[k.teamId] ? S.tid[k.teamId].name + " · " : ""
  const line = k.line != null && LINEY.test(k.code) && !k.playerId ? " " + k.line : k.playerId && k.line != null ? " " + k.line : ""
  return who + marketLabel(k.code, sport) + line + (k.period ? " · " + periodLabel(k.period, sport) : "")
}

/** Stakes per leg that return the same whatever happens, optionally rounded. */
export function arbStakes(a: SignalItem, bank: number, round: number) {
  const sum = a.legs.reduce((s, l) => s + 1 / (l.d || 1), 0)
  const legs = a.legs.map((l) => {
    let stake = bank / ((l.d || 1) * sum)
    if (round > 0) stake = Math.max(round, Math.round(stake / round) * round)
    return { lab: l.lab, d: l.d || 0, src: l.src, deep: l.deep, stake, pay: stake * (l.d || 0) }
  })
  const total = legs.reduce((x, l) => x + l.stake, 0)
  const lo = Math.min(...legs.map((l) => l.pay)), hi = Math.max(...legs.map((l) => l.pay))
  return { legs, total, lo, hi, profitLo: lo - total, profitHi: hi - total }
}

/** Everything the slip needs about one pick: market text, chosen book and price, and its rank among books. */
export function pickInfo(p: SlipPick) {
  const f = S.ev[p.fx]
  const det = S.det[p.fx], hit = det && det.by[p.key]
  const prop = !hit && S.props[p.fx] && S.props[p.fx].by[p.key]
  const b: MarketBoard | PropRow | undefined = hit ? hit.b : prop || undefined
  let ladder: { n: string; v: number; c: Partial<BoardCell> }[] = []
  let sel = p.label
  if (b && b.selections[p.i]) {
    const s = b.selections[p.i]
    sel = s.label || p.label
    ladder = Object.keys(s.cells)
      .map((n) => ({ n, c: s.cells[n], v: decOf(s.cells[n].price) as number }))
      .filter((r) => r.v && !r.c.price?.suspended && isOn(r.n))
      .sort((a, z) => z.v - a.v)
  } else if (f?.best?.[p.i] && p.code === mainMarket(f.sp)) {
    const x = f.best[p.i]!
    ladder = [{ n: x.src, v: x.d as number, c: { deepLink: x.deep } }]
  }
  const chosen = (p.src && ladder.find((r) => r.n === p.src)) || ladder[0] || { v: NaN, n: p.src || "–", c: {} }
  const team = f && (sel === f.h || sel.startsWith(f.h + " ") ? f.hT : sel === f.a || sel.startsWith(f.a + " ") ? f.aT : null)
  const m = hit ? hit.m : null, period = hit ? hit.m.periods[hit.pi].period : ""
  const line = b && b.key.line != null && !b.key.playerId && !(m?.shape === "handicap" && /[+−-]\d/.test(sel)) ? " " + lineLabel(b as MarketBoard, m ? m.shape : "") : ""
  const who = b && b.key.playerId ? (S.pname[b.key.playerId]?.name || "") + " · " : ""
  return {
    f, sel, team,
    market: who + marketLabel(p.code, f ? f.sp : "football") + (b && b.key.playerId ? " " + b.key.line : line) + (period ? " · " + periodLabel(period, f?.sp || "football") : ""),
    price: chosen.v, src: chosen.n, deep: bookHref(f, chosen.n, chosen.c?.deepLink), rank: ladder.indexOf(chosen as (typeof ladder)[number]) + 1, n: ladder.length,
    books: ladder.map((r) => ({ name: r.n, price: r.v })),
  }
}

export { boardBest }
