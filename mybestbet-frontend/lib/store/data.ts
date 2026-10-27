import { api, apiUrl, expire, refreshSession, sampleData, setBoardLookup } from "@/lib/api/client"
import { expandProps } from "@/lib/api/props"
import type {
  BestOutcomes, BestPrice, BoardCell, BoardUpdate, EventBest, EventCalendar, EventDetail, EventInfo, EventScore, EventSummary, League, List, MarketBoard,
  MarketHistory, MarketTab, PlayerStats, Price, PropBoard, PropBoardWire, PropPlayer, PropRow, Signal, SignalType, SignalView,
  SourceHealth, SportSummary, Team,
} from "@/lib/api/types"
import { compMeta } from "@/lib/catalog"
import { decOf } from "@/lib/odds/format"
import { mainMarket, setMarketSpecs, sideName, sourceKind, SOURCE_NOTE } from "@/lib/odds/markets"
import { DAY, NDAYS, hhmm, today0 } from "@/lib/odds/time"
import { webHref } from "@/lib/utils"
import { emit, emitKey } from "./emitter"
import { prefs, userCountry } from "./prefs"

export type BestCell = { d: number | null; src: string; price?: Price; deep?: string } | null

/** A match as the UI uses it: the API summary flattened, with kick-off parsed. */
export type Fixture = {
  id: string
  sp: string
  lg: string
  hT: Team
  aT: Team
  h: string
  a: string
  ko: number
  t: string
  status: EventInfo["status"]
  score?: EventScore
  sources: string[]
  links?: Record<string, string>
  best?: BestCell[]
  badges: { arbs: number; evs: number; maxMargin?: number; maxEv?: number }
}

export type Source = {
  name: string
  kind: "book" | "pm" | "pick"
  note?: string
  down: boolean
  h: SourceHealth
  logoUrl?: string
}

export type SignalLeg = { lab: string; label?: string; side?: string; d: number | null; src: string; deep?: string; price: Price }

export type SignalItem = {
  id: string
  type: SignalType
  f: Fixture
  key: Signal["market"]
  ks: string
  legs: SignalLeg[]
  margin: number
  ev: number
  fair: number
  at: number
  legAge: number
}

export type DetailEntry = { at: number; d: EventDetail; by: Record<string, { m: MarketTab; pi: number; bi: number; b: MarketBoard }> }
export type PropsEntry = { at: number; b: PropBoard; by: Record<string, PropRow>; err?: string }
export type Flash = { dir: 1 | -1; at: number }
export type Connection = "connecting" | "live" | "reconnecting" | "offline"

/** Everything loaded from the API. Mutated in place; call emit() after changes. */
export const S = {
  boot: "loading" as "loading" | "ok" | "error",
  err: "",
  conn: "connecting" as Connection,
  sports: [] as SportSummary[],
  lg: {} as Record<string, League>,
  ev: {} as Record<string, Fixture>,
  tid: {} as Record<string, Team>,
  tname: {} as Record<string, Team>,
  sources: [] as Source[],
  sig: new Map<string, SignalItem>(),
  det: {} as Record<string, DetailEntry>,
  props: {} as Record<string, PropsEntry>,
  pstats: {} as Record<string, PlayerStats>,
  pname: {} as Record<string, PropPlayer>,
  hist: {} as Record<string, MarketHistory>,
  flash: {} as Record<string, Flash>,
  loaded: [] as [number, number][],
  cal: {} as Record<string, { at: number; days: Record<string, number> }>,
  loading: {} as Record<string, Promise<unknown> | undefined>,
}

setBoardLookup((key) => {
  const det = Object.values(S.det).find((x) => x.by[key])
  if (det) return det.by[key].b
  const props = Object.values(S.props).find((x) => x.by[key])
  return props ? props.by[key] : null
})

const once = <T,>(key: string, fn: () => Promise<T>): Promise<T> => {
  if (S.loading[key]) return S.loading[key] as Promise<T>
  const p = fn().finally(() => {
    delete S.loading[key]
  })
  S.loading[key] = p
  return p
}

export const isLoading = (key: string) => !!S.loading[key]

/* sources and leagues */

export const sourceInfo = (name: string) => S.sources.find((s) => s.name === name)
export const isDown = (name: string) => !!sourceInfo(name)?.down
export function availableIn(name: string, country: string) {
  const h = sourceInfo(name)?.h
  if (!h || !country) return true
  return !h.blockedCountries?.includes(country) && (!h.countries?.length || h.countries.includes(country))
}
export const isAvailable = (name: string) => availableIn(name, userCountry())
export const isShown = (name: string) => !prefs.off[name] && (!prefs.geo.only || isAvailable(name))
export const isOn = (name: string) => !isDown(name) && isShown(name)
export const onList = () => S.sources.filter((s) => s.kind !== "pick" && isShown(s.name))
export const onCount = () => S.sources.filter((s) => isShown(s.name)).length

export const leagueName = (code: string) => S.lg[code]?.name || compMeta(code)?.name || code
export const leagueSport = (code: string) => S.lg[code]?.sport || compMeta(code)?.sp || "football"

/* events */

function registerTeam(team: Team) {
  if (!team || !team.id) return
  S.tid[team.id] = team
  if (!S.tname[team.name] || team.logoUrl) S.tname[team.name] = team
}

function bestCells(best: BestOutcomes, sport: string): BestCell[] {
  const conv = (x?: BestPrice): BestCell => (x ? { d: decOf(x.price), src: x.source, price: x.price, deep: x.deepLink } : null)
  return sport === "football" ? [conv(best.home), conv(best.draw), conv(best.away)] : [conv(best.home), conv(best.away)]
}

function flashIf(key: string, before?: number | null, after?: number | null) {
  if (before && after && Math.abs(before - after) > 1e-9) S.flash[key] = { dir: after > before ? 1 : -1, at: Date.now() }
}

function upsertEvent(e: EventInfo & Partial<EventSummary>) {
  const f = S.ev[e.id] || (S.ev[e.id] = { id: e.id, badges: { arbs: 0, evs: 0 }, sources: [] } as unknown as Fixture)
  f.sp = e.sport
  f.lg = e.leagueCode
  f.hT = e.home
  f.aT = e.away
  f.h = e.home.name
  f.a = e.away.name
  f.ko = Date.parse(e.kickoff)
  f.t = hhmm(f.ko)
  f.status = e.status
  if (e.score) f.score = e.score
  registerTeam(e.home)
  registerTeam(e.away)
  if (e.sources) f.sources = e.sources
  if (e.links) f.links = e.links
  if (e.best) {
    const next = bestCells(e.best, f.sp)
    if (f.best) next.forEach((x, i) => flashIf(f.id + "#list#" + i, f.best?.[i]?.d, x?.d))
    f.best = next
  }
  if (e.badges) f.badges = e.badges
  if (!S.lg[f.lg]) S.lg[f.lg] = { code: f.lg, name: leagueName(f.lg), sport: f.sp, country: "" }
  return f
}

export const fixture = (id: string): Fixture | undefined => S.ev[id]

export const bookHref = (f: Fixture | undefined, src: string, deep?: string) => webHref(deep) || webHref(f?.links?.[src])

const EVENTS_PAGE = 1000
const EVENTS_WINDOW = 24 * 3600e3
const EVENTS_PARALLEL = 4

function markLoaded(from: number, to: number) {
  const spans = [...S.loaded, [from, to] as [number, number]].sort((x, y) => x[0] - y[0])
  const merged: [number, number][] = []
  for (const [a, b] of spans) {
    const last = merged[merged.length - 1]
    if (last && a <= last[1]) last[1] = Math.max(last[1], b)
    else merged.push([a, b])
  }
  S.loaded = merged
}

async function loadWindow(from: number, to: number) {
  for (let a = from; a < to; ) {
    const j = await api<List<EventSummary>>(`/v1/events?from=${encodeURIComponent(new Date(a).toISOString())}&to=${encodeURIComponent(new Date(to).toISOString())}`)
    j.items.forEach(upsertEvent)
    const last = j.items.length >= EVENTS_PAGE ? Date.parse(j.items[j.items.length - 1].kickoff) : NaN
    a = last > a ? last : to
  }
  markLoaded(from, to)
  emit()
}

async function loadEvents(from: number, to: number) {
  const windows: [number, number][] = []
  for (let a = from; a < to; a += EVENTS_WINDOW) windows.push([a, Math.min(to, a + EVENTS_WINDOW)])
  let next = 0, failure: unknown = null
  const worker = async () => {
    while (next < windows.length && !failure) {
      const [a, b] = windows[next++]
      try {
        await loadWindow(a, b)
      } catch (e) {
        failure = e
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(EVENTS_PARALLEL, windows.length) }, worker))
  if (failure) throw failure
}

const LIVE_BACK = 6 * 3600e3
const NEAR_DAYS = 3
const BLOCK_DAYS = 4
const MAX_DAYS = 92

const nearRange = () => [Date.now() - LIVE_BACK, today0() + NEAR_DAYS * DAY] as const
export const loadNear = () => once("near", () => { const [a, b] = nearRange(); return loadEvents(a, b) })

/** Whether events for day `d` (0 is today) are loaded. */
export const covered = (d: number) => {
  const a = today0() + d * DAY, b = a + DAY
  return S.loaded.some(([x, y]) => x <= Math.max(a, Date.now() - LIVE_BACK) && y >= b)
}

const failedAt: Record<number, number> = {}

export function ensureDays(from: number, to: number) {
  const todo: number[] = []
  for (let d = Math.max(0, from); d < Math.min(to, NEAR_DAYS); d++) {
    if (!covered(d)) {
      todo.push(0)
      break
    }
  }
  for (let i = Math.max(0, Math.floor((from - NEAR_DAYS) / BLOCK_DAYS)); NEAR_DAYS + i * BLOCK_DAYS < to; i++) {
    const d = NEAR_DAYS + i * BLOCK_DAYS
    if (!covered(d) || !covered(d + BLOCK_DAYS - 1)) todo.push(today0() + d * DAY)
  }
  if (!todo.length) return null
  const due = todo.filter((a) => !(Date.now() - (failedAt[a] || 0) < 30000))
  if (!due.length) return Promise.resolve()
  return once("days:" + due.join(","), async () => {
    for (const a of due) {
      try {
        await (a ? once("blk:" + a, () => loadEvents(a, a + BLOCK_DAYS * DAY)) : loadNear())
        delete failedAt[a]
      } catch {
        failedAt[a] = Date.now()
      }
      emit()
    }
  })
}

export const ensureDay = (d: number) => ensureDays(d, d + 1)

const dateKey = (d: number) => {
  const x = new Date(today0() + d * DAY + DAY / 2)
  return x.getFullYear() + "-" + String(x.getMonth() + 1).padStart(2, "0") + "-" + String(x.getDate()).padStart(2, "0")
}

export function ensureCalendar(scope: string) {
  const c = S.cal[scope]
  if (c && Date.now() - c.at < 300000) return
  once("cal:" + scope, async () => {
    const tz = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC"
    const from = new Date(Math.max(today0(), Date.now() - 2 * 3600e3)).toISOString(), to = new Date(today0() + MAX_DAYS * DAY).toISOString()
    const j = await api<EventCalendar>(`/v1/events/calendar?tz=${encodeURIComponent(tz)}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}${scope ? "&" + scope : ""}`)
    S.cal[scope] = { at: Date.now(), days: Object.fromEntries(j.days.map((x) => [x.date, x.count])) }
    emit()
  }).catch(() => {
    S.cal[scope] = { at: Date.now() - 240000, days: c?.days || {} }
  })
}

export const calCount = (scope: string, d: number): number | null => (S.cal[scope] ? S.cal[scope].days[dateKey(d)] || 0 : null)

export function horizon() {
  let last = 0
  for (let d = 0; d < MAX_DAYS; d++) if (calCount("", d)) last = d
  return Math.max(NDAYS, last + 1)
}

export function dayFixtures(d: number) {
  const a = today0() + d * DAY, b = a + DAY
  const lo = d === 0 ? Math.min(a, Date.now() - 2 * 3600e3) : a, liveLo = d === 0 ? Math.min(a, Date.now() - LIVE_BACK) : a
  return Object.values(S.ev).filter((f) => f.ko >= (f.status === "live" ? liveLo : lo) && f.ko < b && f.status !== "finished").sort((x, y) => x.ko - y.ko)
}

export function fixturesFrom(from: number | "now", pred: (f: Fixture) => boolean) {
  const start = from === "now" ? Date.now() - 2 * 3600e3 : today0() + from * DAY
  return Object.values(S.ev).filter((f) => f.ko >= start && f.status !== "finished" && pred(f)).sort((x, y) => x.ko - y.ko)
}

export const sportCount = (sport: string, d: number) => dayFixtures(d).filter((f) => f.sp === sport).length

/* event detail, props, stats, history */

function indexDetail(d: EventDetail) {
  const by: DetailEntry["by"] = {}
  d.markets.forEach((m) => m.periods.forEach((p, pi) => p.boards.forEach((b, bi) => { by[b.keyString] = { m, pi, bi, b } })))
  return by
}

function diffFlash(prev: DetailEntry, d: EventDetail) {
  d.markets.forEach((m) => m.periods.forEach((p) => p.boards.forEach((b) => {
    const o = prev.by[b.keyString]
    if (!o) return
    b.selections.forEach((s, i) => {
      const os = o.b.selections[i]
      if (!os) return
      Object.keys(s.cells).forEach((n) => flashIf(b.keyString + "#" + i + "#" + n, os.cells[n] && decOf(os.cells[n].price), decOf(s.cells[n].price)))
    })
  })))
}

export function loadDetail(id: string, force?: boolean): Promise<DetailEntry> {
  const c = S.det[id]
  if (c && !force && Date.now() - c.at < 30000) return Promise.resolve(c)
  return once("det:" + id, async () => {
    const d = await api<EventDetail>("/v1/events/" + id)
    upsertEvent(d.event)
    const prev = S.det[id]
    if (prev) diffFlash(prev, d)
    S.det[id] = { at: Date.now(), d, by: indexDetail(d) }
    emit()
    return S.det[id]
  })
}

export function primeDetail(text: string) {
  let d: EventDetail
  try {
    d = JSON.parse(text)
  } catch {
    return
  }
  const id = d?.event?.id
  if (!id || sampleData() || S.det[id]?.at) return
  upsertEvent(d.event)
  S.det[id] = { at: Date.now(), d, by: indexDetail(d) }
  emit()
}

export function prefetchEvent(id: string) {
  if (S.boot === "ok") loadDetail(id).catch(() => {})
}

export function loadProps(id: string, force?: boolean): Promise<PropsEntry> {
  const c = S.props[id]
  if (c && !force && Date.now() - c.at < 60000) return Promise.resolve(c)
  return once("props:" + id, async () => {
    try {
      const b = expandProps(await api<PropBoardWire | PropBoard>("/v1/events/" + id + "/props"))
      upsertEvent(b.event)
      b.rows.forEach((r) => { S.pname[r.player.id] = r.player })
      S.props[id] = { at: Date.now(), b, by: Object.fromEntries(b.rows.map((r) => [r.keyString, r])) }
    } catch (e) {
      S.props[id] = { at: Date.now(), b: { rows: [] } as unknown as PropBoard, by: {}, err: (e as Error).message }
    }
    emit()
    return S.props[id]
  })
}

export const loadStats = (pid: string): Promise<PlayerStats> =>
  S.pstats[pid]
    ? Promise.resolve(S.pstats[pid])
    : once("ps:" + pid, async () => {
        try {
          const j = await api<PlayerStats>("/v1/players/" + pid + "/stats?last=20")
          S.pname[pid] = j.player
          S.pstats[pid] = j
        } catch (e) {
          S.pstats[pid] = { player: { id: pid, name: "Player" }, items: [], err: (e as Error).message }
        }
        emit()
        return S.pstats[pid]
      })

export const historyKey = (key: string, src: string) => key + "#" + src

export const loadHistory = (key: string, src: string): Promise<MarketHistory> => {
  const k = historyKey(key, src)
  return S.hist[k]
    ? Promise.resolve(S.hist[k])
    : once("h:" + k, async () => {
        const j = await api<MarketHistory>("/v1/markets/" + encodeURIComponent(key) + "/history?source=" + encodeURIComponent(src))
        S.hist[k] = j
        emit()
        return j
      })
}

/* detail queue: the match list loads up to four event details at a time for its market switcher */

const queue: string[] = []
let active = 0
export function want(id: string) {
  if (S.det[id]?.at || queue.includes(id) || S.loading["det:" + id]) return
  queue.push(id)
  pump()
}
/** Whether a match's detail is waiting in the list queue. */
export const isQueued = (id: string) => queue.includes(id)
function pump() {
  while (active < 4 && queue.length) {
    const id = queue.shift()!
    active++
    loadDetail(id).catch(() => {}).finally(() => { active--; pump() })
  }
}

const propQueue: string[] = []
let propActive = 0
export function wantProps(id: string) {
  if (S.props[id] || propQueue.includes(id) || S.loading["props:" + id]) return
  propQueue.push(id)
  pumpProps()
}
function pumpProps() {
  while (propActive < 3 && propQueue.length) {
    const id = propQueue.shift()!
    propActive++
    loadProps(id).catch(() => {}).finally(() => { propActive--; pumpProps() })
  }
}

/* sources, sports, signals */

function setSources(items: SourceHealth[]) {
  const order = { book: 0, pm: 1, pick: 2 }
  S.sources = items
    .map((h) => ({ name: h.name, kind: sourceKind(h.name, h.type), note: h.note || SOURCE_NOTE[h.name], down: !h.enabled || h.degraded, h, logoUrl: h.logoUrl }))
    .sort((a, b) => order[a.kind] - order[b.kind])
}

async function loadSources() {
  setSources((await api<List<SourceHealth>>("/v1/sources")).items)
}

function setSports(items: SportSummary[]) {
  S.sports = items
  items.forEach((s) => setMarketSpecs(s.markets || []))
  items.forEach((s) => s.leagues.forEach((l) => { S.lg[l.code] = l }))
}

async function loadSports() {
  setSports((await api<List<SportSummary>>("/v1/sports")).items)
}

function normalizeSignal(sv: SignalView): SignalItem {
  const f = upsertEvent(sv.event)
  const m = sv.market
  if (sv.player) S.pname[sv.player.id] = sv.player
  return {
    id: sv.id, type: sv.type, f, key: m,
    ks: [m.eventId, m.code, m.line == null ? "" : m.line, m.playerId || "", m.period || ""].join("|"),
    legs: sv.legs.map((l) => ({ lab: l.label || sideName(l.side, m.code, l.line ?? m.line, f.h, f.a), label: l.label, side: l.side, d: decOf(l.price), src: l.source, deep: l.deepLink || f.links?.[l.source], price: l.price })),
    margin: sv.margin || 0, ev: sv.ev || 0, fair: sv.fairProb || 0, at: Date.parse(sv.detectedAt), legAge: (sv.oldestLegAge || 0) / 1e6,
  }
}

async function loadSignals() {
  const j = await api<List<SignalView>>("/v1/signals?limit=500")
  S.sig = new Map(j.items.map((s) => [s.id, normalizeSignal(s)]))
}

function applySignal(s: Signal) {
  if (s.status === "closed") { S.sig.delete(s.id); return }
  const f = S.ev[s.eventId]
  const prev = S.sig.get(s.id)
  if (!f && !prev) return
  if (f) {
    const event = { id: f.id, sport: f.sp, leagueCode: f.lg, home: f.hT, away: f.aT, kickoff: new Date(f.ko).toISOString(), status: f.status }
    S.sig.set(s.id, normalizeSignal({ ...s, event }))
  } else if (prev) prev.at = Date.parse(s.detectedAt)
}

/** Open signals of one type whose books are all switched on and whose match hasn't ended. */
export const signalList = (type: SignalType) =>
  [...S.sig.values()].filter((s) => s.type === type && s.legs.every((l) => isShown(l.src)) && (!s.f.status || s.f.status === "scheduled" || s.f.status === "live"))

export const signalsFor = (fixtureId: string, type: SignalType) => [...S.sig.values()].filter((s) => s.f.id === fixtureId && s.type === type)

export const arbPct = (a: SignalItem) => (a.margin ? a.margin * 100 : (1 - a.legs.reduce((s, l) => s + 1 / (l.d || 1), 0)) * 100)

/* live updates */

function findSelection(b: MarketBoard | PropRow, side?: string, label?: string) {
  let i = b.selections.findIndex((s) => side && s.side === side && (!label || s.label === label))
  if (i < 0) i = b.selections.findIndex((s) => s.label === label)
  if (i < 0 && side) i = b.selections.findIndex((s) => s.side === side)
  return i
}

/** Best price per selection across switched-on books, skipping derived and suspended prices. */
export function boardBest(b: MarketBoard | PropRow) {
  return b.selections.map((s) => {
    let best = 0, src = ""
    Object.keys(s.cells).forEach((n) => {
      const c = s.cells[n], v = decOf(c.price)
      if (v && !c.derived && !c.price.suspended && isOn(n) && v > best) { best = v; src = n }
    })
    return { d: best || null, src }
  })
}

const CONSENSUS = "consensus"

function onUpdate(u: BoardUpdate) {
  if (u.score && S.ev[u.eventId]) S.ev[u.eventId].score = u.score
  ;(u.cells || []).forEach((c: BoardCell) => {
    const det = S.det[u.eventId]
    const hit = det && det.by[c.marketKey]
    const prop = !hit && S.props[u.eventId] && S.props[u.eventId].by[c.marketKey]
    const b = hit ? hit.b : prop
    if (!b) return
    const i = findSelection(b, c.side, c.label)
    if (i < 0) return
    if (c.source === CONSENSUS) {
      if (hit) hit.b.selections[i].consensus = { ...(hit.b.selections[i].consensus || {}), ...c }
      return
    }
    const sel = b.selections[i], old = sel.cells[c.source]
    flashIf(c.marketKey + "#" + i + "#" + c.source, old && decOf(old.price), decOf(c.price))
    sel.cells[c.source] = { ...(old || {}), ...c }
    const f = S.ev[u.eventId]
    if (hit && f && hit.m.code === mainMarket(f.sp) && !hit.m.periods[hit.pi].period && hit.bi === 0) {
      f.best = boardBest(b).map((x, k) => {
        flashIf(f.id + "#list#" + k, f.best?.[k]?.d, x.d)
        const cell = x.src ? b.selections[k].cells[x.src] : undefined
        return x.d ? { d: x.d, src: x.src, price: cell?.price, deep: cell?.deepLink } : null
      })
    }
  })
  ;(u.signals || []).forEach(applySignal)
  emitSoon()
}

let emitQueued = false
const emitSoon = () => {
  if (emitQueued) return
  emitQueued = true
  requestAnimationFrame(() => {
    emitQueued = false
    emit()
  })
}

const BEST_SYNC_MS = 1000
const touched = new Set<string>()
let touchQueued = false
let bestSync: ReturnType<typeof setTimeout> | undefined

const touchSoon = (id: string) => {
  touched.add(id)
  if (!touchQueued) {
    touchQueued = true
    requestAnimationFrame(() => {
      touchQueued = false
      const ids = [...touched]
      touched.clear()
      ids.forEach(emitKey)
    })
  }
  bestSync ??= setTimeout(() => {
    bestSync = undefined
    emit()
  }, BEST_SYNC_MS)
}

function onBest(u: EventBest) {
  const f = S.ev[u.eventId]
  if (!f) return
  if (u.score) f.score = u.score
  const b = u.best
  if (b && b.market === mainMarket(f.sp) && b.line == null && !b.period) {
    const next = bestCells(b, f.sp)
    if (f.best) next.forEach((x, i) => flashIf(f.id + "#list#" + i, f.best?.[i]?.d, x?.d))
    f.best = next
  }
  touchSoon(f.id)
}

let boardLive = false
let nearAt = 0
const NEAR_POLL_LIVE = 120000

type Stream = { path: string; close: () => void }
const streams: Record<string, Stream> = {}

function setConnection(c: Connection) {
  S.conn = c
  emit()
}

/** Sample data mode: nudge random prices every 1.3s so the page looks live. */
function simulate(name: string, path: string) {
  const ev = (/event=([^&]+)/.exec(path) || [])[1]
  const id = setInterval(() => {
    if (document.hidden) return
    if (ev) {
      const det = S.det[decodeURIComponent(ev)]
      if (!det) return
      const cells: BoardCell[] = []
      for (let k = 0, n = 1 + Math.floor(Math.random() * 3); k < n; k++) {
        const ms = det.d.markets, mm = Math.random() < 0.5 ? ms[0] : ms[Math.floor(Math.random() * ms.length)]
        const per = mm.periods[Math.floor(Math.random() * mm.periods.length)], b = per.boards[Math.floor(Math.random() * per.boards.length)]
        const sel = b.selections[Math.floor(Math.random() * b.selections.length)]
        const names = Object.keys(sel.cells).filter((x) => sel.cells[x].price.form === "decimal")
        if (!names.length) continue
        const c = sel.cells[names[Math.floor(Math.random() * names.length)]]
        const next = Math.max(1.02, Math.round((c.price.raw + (Math.random() < 0.5 ? -1 : 1) * 0.01 * (1 + Math.floor(Math.random() * 3))) * 100) / 100)
        cells.push({ ...c, price: { ...c.price, raw: next, prob: 1 / next, observedAt: new Date().toISOString() } })
      }
      if (cells.length) onUpdate({ eventId: decodeURIComponent(ev), cells, sentAt: new Date().toISOString() })
    } else if (Math.random() < 0.3 && S.sig.size) {
      const all = [...S.sig.values()]
      all[Math.floor(Math.random() * all.length)].at = Date.now()
      emit()
    }
  }, 1300)
  streams[name] = { path, close: () => clearInterval(id) }
  if (name === "main") setConnection("live")
}

function stream(name: string, path: string, attempt = 0) {
  if (streams[name]?.path === path && !attempt) return
  closeStream(name)
  if (sampleData()) return simulate(name, path)
  if (typeof EventSource === "undefined") return
  const es = new EventSource(apiUrl(path))
  const self: Stream = { path, close: () => es.close() }
  streams[name] = self
  const handle = (m: MessageEvent) => { try { onUpdate(JSON.parse(m.data)) } catch {} }
  es.addEventListener("update", handle as EventListener)
  es.onmessage = handle
  es.addEventListener("best", ((m: MessageEvent) => { try { onBest(JSON.parse(m.data)) } catch {} }) as EventListener)
  es.onopen = () => {
    if (name === "board") {
      boardLive = true
      if (attempt) nearAt = 0
    }
    attempt = 0
    if (name === "main") setConnection("live")
  }
  es.onerror = () => {
    es.close()
    if (name === "board") boardLive = false
    if (name === "main") setConnection("reconnecting")
    setTimeout(async () => {
      if (streams[name] !== self) return
      const refreshed = await refreshSession()
      if (streams[name] !== self) return
      if (refreshed === "denied") return expire()
      stream(name, path, attempt + 1)
    }, Math.min(30000, 1000 * 2 ** attempt))
  }
}

function closeStream(name: string) {
  streams[name]?.close()
  delete streams[name]
}

/** Live prices for the open match page. Pass null to stop. */
export function watchEvent(id: string | null) {
  if (id) stream("event", "/v1/stream?event=" + encodeURIComponent(id))
  else closeStream("event")
}

const watchers = new Set<string>()
/** Live prices for a set of matches, e.g. the ones on the landing page. */
export function watchExtra(ids: string[]) {
  ;[...watchers].forEach((id) => { if (!ids.includes(id)) { closeStream("w:" + id); watchers.delete(id) } })
  ids.forEach((id) => { if (!watchers.has(id)) { watchers.add(id); stream("w:" + id, "/v1/stream?event=" + encodeURIComponent(id)) } })
}

/* boot and polling */

let booted = false

export type BoardSeed = { from: number; to: number; sports: SportSummary[]; events: EventSummary[] }

type BoardCache = {
  v: 1
  at: number
  sports: SportSummary[]
  sources: SourceHealth[]
  ev: Fixture[]
  det: [string, EventDetail][]
  cal: typeof S.cal
}

const CACHE_KEY = "mbb.board.v1"
const CACHE_TTL = 6 * 3600e3
const CACHE_DETAILS = 6
const CACHE_MAX_BYTES = 2_500_000
const CACHE_EVERY = 30000

function restoreBoard() {
  let c: BoardCache | null = null
  try {
    c = JSON.parse(localStorage.getItem(CACHE_KEY) || "null")
  } catch {}
  if (!c || c.v !== 1 || Date.now() - c.at > CACHE_TTL || !c.sports?.length) return false
  setSports(c.sports)
  if (c.sources?.length) setSources(c.sources)
  const cut = Date.now() - LIVE_BACK
  c.ev.forEach((f) => {
    if (f.ko < cut || S.ev[f.id]) return
    S.ev[f.id] = f
    registerTeam(f.hT)
    registerTeam(f.aT)
  })
  c.det.forEach(([id, d]) => { if (!S.det[id]) S.det[id] = { at: 0, d, by: indexDetail(d) } })
  S.cal = { ...c.cal, ...S.cal }
  return true
}

function saveBoard() {
  if (S.boot !== "ok" || sampleData() || !S.sports.length) return
  const lo = Date.now() - LIVE_BACK, hi = today0() + NEAR_DAYS * DAY
  const ev = Object.values(S.ev).filter((f) => f.ko >= lo && f.ko < hi && f.status !== "finished")
  const body: BoardCache = { v: 1, at: Date.now(), sports: S.sports, sources: S.sources.map((s) => s.h), ev, det: [], cal: S.cal }
  let text = JSON.stringify(body), room = CACHE_MAX_BYTES - text.length
  for (const [id, x] of Object.entries(S.det).filter(([, x]) => x.at).sort((a, b) => b[1].at - a[1].at).slice(0, CACHE_DETAILS)) {
    const size = JSON.stringify(x.d).length
    if (size > room) continue
    body.det.push([id, x.d])
    room -= size
  }
  if (body.det.length) text = JSON.stringify(body)
  try {
    localStorage.setItem(CACHE_KEY, text)
  } catch {
    try {
      localStorage.removeItem(CACHE_KEY)
    } catch {}
  }
}

function applySeed(seed: BoardSeed) {
  setSports(seed.sports)
  seed.events.forEach(upsertEvent)
  if (seed.events.length < EVENTS_PAGE) markLoaded(seed.from, seed.to)
}

const BOOT_RETRIES = 6

function background(key: string, fn: () => Promise<unknown>, attempt = 0) {
  once(key, fn).then(
    () => emit(),
    () => {
      emit()
      if (attempt < BOOT_RETRIES) setTimeout(() => background(key, fn, attempt + 1), Math.min(30000, 2000 * 2 ** attempt))
    },
  )
}

export const signalsPending = () => !!S.loading.signals

const goLive = () => {
  stream("main", "/v1/stream")
  stream("board", "/v1/stream?scope=board")
}

let seedWaiting: BoardSeed | null = null

export function primeSeed(seed: BoardSeed) {
  if (!seed.sports?.length || sampleData()) return
  if (!booted) {
    seedWaiting = seed
    return
  }
  if (S.boot === "error") return
  applySeed(seed)
  if (S.boot === "loading") {
    S.boot = "ok"
    goLive()
  }
  emit()
}

export async function boot() {
  const cached = !sampleData() && restoreBoard()
  const seed = seedWaiting
  seedWaiting = null
  if (seed) applySeed(seed)
  const warm = cached || !!seed
  S.boot = warm ? "ok" : "loading"
  S.err = ""
  if (warm) goLive()
  emit()
  const [a, b] = nearRange()
  const near = seed && seed.events.length < EVENTS_PAGE ? once("near", () => loadEvents(Math.max(a, seed.to), b)) : loadNear()
  const sports = seed ? Promise.resolve() : once("sports", loadSports)
  background("signals", loadSignals)
  background("sources", loadSources)
  near.then(() => emit(), () => background("near:retry", loadNear))
  try {
    await sports
  } catch (e) {
    try {
      if (S.boot !== "ok") await near
      background("sports", loadSports)
    } catch {
      S.boot = "error"
      S.err = (e as Error).message || "Network error"
      S.conn = "offline"
      emit()
      return
    }
  }
  S.boot = "ok"
  goLive()
  emit()
}

/** Start loading once per page load, plus the polling that backs up the streams. */
export function startStore() {
  if (booted || typeof window === "undefined") return
  booted = true
  boot()
  setInterval(() => (typeof requestIdleCallback === "function" ? requestIdleCallback(saveBoard, { timeout: 5000 }) : saveBoard()), CACHE_EVERY)
  addEventListener("pagehide", saveBoard)
  document.addEventListener("visibilitychange", () => { if (document.hidden) saveBoard() })
  setInterval(async () => {
    if (S.boot !== "ok" || document.hidden || (boardLive && Date.now() - nearAt < NEAR_POLL_LIVE)) return
    try {
      await loadNear()
      nearAt = Date.now()
      emit()
    } catch {}
  }, 30000)
  setInterval(async () => {
    if (S.boot !== "ok" || document.hidden) return
    try {
      await Promise.all([loadSignals(), loadSources().catch(() => {})])
      emit()
    } catch {}
  }, 45000)
}

export const storeReady = () => S.boot === "ok"
