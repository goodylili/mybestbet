import type { BoardCell, HitRate, HitRateWire, MarketKey, PriceForm, PropBoard, PropBoardWire, PropCell, PropPlayerInfo, PropRow, PropRowWire, PropSelection, SelectionRow } from "./types"

const RATING_CODE = "P_RATING"

export function parseMarketKey(keyString: string): MarketKey {
  const [eventId, code, line, playerId, period, teamId] = keyString.split("|")
  const key: MarketKey = { eventId, code }
  if (line) key.line = Number(line)
  if (playerId) key.playerId = playerId
  if (period) key.period = period
  if (teamId) key.teamId = teamId
  return key
}

export function impliedProb(raw: number, form: PriceForm) {
  switch (form) {
    case "decimal":
      return raw > 1 ? 1 / raw : 0
    case "share":
      return raw
    case "cents":
    case "naira":
      return raw / 100
  }
  return 0
}

function boardCell(c: PropCell, marketKey: string, source: string, s: PropSelection): BoardCell {
  const form = c.form || "decimal"
  return {
    marketKey,
    source,
    side: s.side,
    label: s.label,
    price: { raw: c.raw, form, prob: c.prob ?? impliedProb(c.raw, form), bid: c.bid, ask: c.ask, liquidity: c.liquidity, suspended: c.suspended },
    fair: c.fair,
    derived: c.derived,
    deepLink: c.deepLink,
  }
}

function selectionRow(s: PropSelection, marketKey: string): SelectionRow {
  const cells: Record<string, BoardCell> = {}
  for (const source in s.cells) cells[source] = boardCell(s.cells[source], marketKey, source, s)
  return { side: s.side, label: s.label, bestSource: s.bestSource, consensus: s.consensus && boardCell(s.consensus, marketKey, "consensus", s), cells }
}

const headline = (windows: HitRate[]): HitRate | undefined => windows.find((w) => w.window === 10) || windows[windows.length - 1]

function hitRate({ window = 0, matches = 0, hits = 0 }: HitRateWire): HitRate {
  return { window, matches, hits, rate: matches ? hits / matches : 0 }
}

function propRow(r: PropRowWire, players: Map<string, PropPlayerInfo>): PropRow {
  const key = parseMarketKey(r.keyString)
  const id = key.playerId || ""
  const { minutesTrend, lineupStatus, benchWarning, ratings, ...player } = players.get(id) || { id, name: "", minutesTrend: [], lineupStatus: "", benchWarning: false }
  const research = r.research && {
    windows: r.research.windows.map(hitRate),
    home: hitRate(r.research.home),
    away: hitRate(r.research.away),
    vsOpponent: hitRate(r.research.vsOpponent),
    vsOpponentMatches: r.research.vsOpponentMatches || [],
    minutesTrend,
    lineupStatus,
    benchWarning,
    ratings: key.code === RATING_CODE ? ratings : undefined,
  }
  return {
    key,
    keyString: r.keyString,
    player,
    selections: r.selections.map((s) => selectionRow(s, r.keyString)),
    hitRate: research ? headline(research.windows) : undefined,
    research,
  }
}

export function expandProps(b: PropBoardWire | PropBoard): PropBoard {
  if (!("players" in b)) return b
  const players = new Map(b.players.map((p) => [p.id, p]))
  return { event: b.event, sources: b.sources, rows: b.rows.map((r) => propRow(r, players)) }
}
