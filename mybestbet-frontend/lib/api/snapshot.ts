import type { EventDetail, EventSummary, List, MarketBoard, MarketHistory, PropBoard, PropRow } from "./types"
import { decOf } from "@/lib/odds/format"
import { hash } from "@/lib/odds/hash"
import { mainMarket } from "@/lib/odds/markets"

const ROOT = "/mybestbet/snapshot"
const ISO = /"(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d))"/g
const cache = new Map<string, Promise<unknown | null>>()
let shift: Promise<number> | null = null

/** Every timestamp in the snapshot moves forward by the time since it was taken, so kick-offs stay upcoming. */
function getShift() {
  if (!shift) {
    shift = fetch(ROOT + "/meta.json")
      .then((r) => r.json())
      .then((m: { takenAt: string }) => Date.now() - Date.parse(m.takenAt))
      .catch(() => 0)
  }
  return shift
}

function file<T>(key: string): Promise<T | null> {
  if (!cache.has(key)) {
    cache.set(
      key,
      (async () => {
        const response = await fetch(ROOT + key + ".json")
        if (!response.ok) return null
        const [text, offset] = await Promise.all([response.text(), getShift()])
        return JSON.parse(text.replace(ISO, (_m, t: string) => '"' + new Date(Date.parse(t) + offset).toISOString() + '"'))
      })(),
    )
  }
  return cache.get(key) as Promise<T | null>
}

function fakeHistory(key: string, source: string, board: MarketBoard | PropRow | null): MarketHistory {
  const points: MarketHistory["points"] = []
  board?.selections.forEach((selection, i) => {
    const cell = selection.cells[source]
    if (!cell || cell.price.form === "line_only") return
    let price = decOf(cell.price) || 2
    let t = Date.now()
    const out: MarketHistory["points"] = []
    const n = 14 + Math.floor(hash(key + source) * 8)
    for (let k = 0; k < n; k++) {
      out.push({ source, side: selection.side, label: selection.label, raw: Math.round(price * 100) / 100, form: "decimal", prob: 1 / price, suspended: false, observedAt: new Date(t).toISOString() })
      price = Math.max(1.01, price * (1 + (hash(key + source + i + "s" + k) - 0.5) * 0.035))
      t -= (30 + hash(key + source + k) * 220) * 60000
    }
    points.push(...out.reverse())
  })
  points.sort((a, b) => Date.parse(a.observedAt) - Date.parse(b.observedAt))
  return { marketKey: key, points }
}

export async function snapshotApi(path: string, lookup: (key: string) => MarketBoard | PropRow | null): Promise<unknown> {
  const [p, qs] = path.split("?")
  const query = new URLSearchParams(qs || "")
  let m: RegExpExecArray | null
  if (p === "/v1/sports" || p === "/v1/sources" || p === "/v1/signals") return file(p)
  if (p === "/v1/events") {
    const all = await file<List<EventSummary>>(p)
    const from = Date.parse(query.get("from") || ""), to = Date.parse(query.get("to") || "")
    return { items: (all?.items || []).filter((e) => { const k = Date.parse(e.kickoff); return k >= from && k <= to }) }
  }
  if ((m = /^\/v1\/events\/([^/]+)\/props$/.exec(p))) {
    const props = await file<PropBoard>("/p/" + m[1])
    if (props) return props
    const detail = (await snapshotApi("/v1/events/" + m[1], lookup)) as EventDetail
    return { event: detail.event, sources: [], rows: [] }
  }
  if ((m = /^\/v1\/events\/([^/]+)$/.exec(p))) {
    const detail = await file<EventDetail>("/d/" + m[1])
    if (detail) return detail
    const id = m[1]
    const all = await file<List<EventSummary>>("/v1/events")
    const e = all?.items.find((x) => x.id === id)
    if (!e) throw new Error("event not found")
    const code = mainMarket(e.sport), key = [e.id, code, "", "", ""].join("|")
    const sides: [string, string, EventSummary["best"]["home"]][] = e.sport === "football"
      ? [["home", e.home.name, e.best.home], ["draw", "Draw", e.best.draw], ["away", e.away.name, e.best.away]]
      : [["home", e.home.name, e.best.home], ["away", e.away.name, e.best.away]]
    const selections = sides.map(([side, label, best]) => ({
      side, label, bestSource: best ? best.source : "",
      cells: best ? { [best.source]: { marketKey: key, source: best.source, side, label, price: best.price, deepLink: best.deepLink } } : {},
    }))
    return { event: e, sources: e.sources, markets: [{ code, kind: "match", shape: e.sport === "football" ? "three_way" : "two_way", periods: [{ period: "", boards: [{ key: { eventId: e.id, code }, keyString: key, selections }] }] }] }
  }
  if ((m = /^\/v1\/players\/([^/]+)\/stats$/.exec(p))) {
    const stats = await file("/s/" + m[1])
    if (stats) return stats
    throw new Error("player not found")
  }
  if ((m = /^\/v1\/markets\/(.+)\/history$/.exec(p))) {
    const key = decodeURIComponent(m[1])
    return fakeHistory(key, query.get("source") || "", lookup(key))
  }
  throw new Error("not in sample data")
}
