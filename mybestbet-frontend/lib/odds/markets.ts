import type { MarketSpec, SignalType } from "@/lib/api/types"
import { formatLine } from "./format"

export const SPORTS = [
  { id: "football", name: "Football" },
  { id: "basketball", name: "Basketball" },
  { id: "tennis", name: "Tennis" },
  { id: "american-football", name: "American football" },
  { id: "esports", name: "Esports" },
] as const

export const sportName = (id: string) => SPORTS.find((s) => s.id === id)?.name ?? id

const LABEL: Record<string, string> = {
  "1X2": "Result", DC: "Double chance", OU: "Goals", BTTS: "Both score", DNB: "Draw no bet", AH: "Handicap", CORN: "Corners", CARDS: "Cards",
  OE: "Odd/Even", TG: "Team goals", CS: "Correct score", HTFT: "HT/FT", SPREAD: "Spread", TOTAL: "Total points", SETS: "Set handicap",
  GAMES: "Total games", MAPS: "Map handicap", TMAPS: "Total maps",
  P_SCORER: "Anytime scorer", P_GOALS: "Goals", P_SOT: "Shots on target", P_SHOTS: "Shots", P_AST: "Assists", P_ASSISTS: "Assists",
  P_CARD: "To be booked", P_TACKLES: "Tackles", P_FOULS: "Fouls", P_RATING: "Rating",
  "1H_1X2": "1st half", CORN_OU: "Corners", QUALIFY: "To qualify", SET_HCP: "Set handicap", TOTAL_GAMES: "Total games",
  MAP_HCP: "Map handicap", TOTAL_MAPS: "Total maps", P_ANYTIME: "Anytime scorer", P_FIRST: "First scorer", P_LAST: "Last scorer",
  P_GA: "Goal or assist", P_SAVES: "Saves", P_PASSES: "Passes",
  T_GOALS: "Team goals", T_SHOTS: "Team shots", T_PASSES: "Team passes", T_CARDS: "Cards",
}

const SPECS: Record<string, MarketSpec> = {}

export function setMarketSpecs(specs: MarketSpec[]) {
  specs.forEach((m) => { SPECS[m.code] = m })
}

export const regulationOnly = (code: string) => SPECS[code]?.regulationOnly

export const statField = (code: string) => PROP_STAT[code] || SPECS[code]?.statField || null

/** Prop market code to the stat field it is settled on. Add the real API's codes here. */
export const PROP_STAT: Record<string, string> = {
  P_SCORER: "goals", P_GOALS: "goals", P_SOT: "shotsOnTarget", P_SHOTS: "shots", P_AST: "assists", P_ASSISTS: "assists",
  P_CARD: "yellow", P_FOULS: "fouls", P_TACKLES: "tackles", P_RATING: "rating",
}

const PERIOD: Record<string, string> = { "": "Full game", "1H": "1st half", "2H": "2nd half", Q1: "Q1", Q2: "Q2", Q3: "Q3", Q4: "Q4", S1: "Set 1", M1: "Map 1" }

export const SIGNALS: Record<SignalType, { title: string; slug: string }> = {
  arb: { title: "Arbitrage", slug: "arbs" },
  ev: { title: "Value bets", slug: "value" },
  line: { title: "Line gaps", slug: "lines" },
  move: { title: "Steam moves", slug: "moves" },
}
export const SIGNAL_TYPES = Object.keys(SIGNALS) as SignalType[]
export const signalBySlug = (slug: string) => SIGNAL_TYPES.find((k) => SIGNALS[k].slug === slug)

export const LINEY = /^(OU|TOTAL|CORN|CARDS|GAMES|TMAPS|TG|T_|P_|BK_|NF_|TN_|ES_)/

export const mainMarket = (sport: string) => (sport === "football" ? "1X2" : "ML")

export function marketLabel(code: string, sport: string) {
  if (code === "ML") return sport === "tennis" || sport === "esports" ? "Match winner" : "Moneyline"
  if (LABEL[code]) return LABEL[code]
  if (SPECS[code]) return SPECS[code].label
  if (code.startsWith("P_")) return code.slice(2).replace(/_/g, " ").toLowerCase().replace(/^./, (c) => c.toUpperCase())
  return code
}

export function periodLabel(period: string, sport: string) {
  if (period) return PERIOD[period] || period
  return sport === "football" ? "Full time" : sport === "tennis" || sport === "esports" ? "Match" : "Full game"
}

const SOURCE_TYPE: Record<string, "book" | "pm" | "pick"> = { sportsbook: "book", prediction: "pm", pickem: "pick" }
const SOURCE_KIND: Record<string, "book" | "pm" | "pick"> = { polymarket: "pm", kalshi: "pm", predictfun: "pm", bayse: "pm", prizepicks: "pick" }
export const sourceKind = (name: string, type?: string) => (type && SOURCE_TYPE[type]) || SOURCE_KIND[name] || "book"
export const SOURCE_NOTE: Record<string, string> = { footballcom: "SportyBet feed", "22bet": "1xBet feed", paripesa: "1xBet feed", betwinner: "1xBet feed", prizepicks: "Lines only" }

const SIDE_WORD: Record<string, string> = { draw: "Draw", over: "Over", under: "Under", yes: "Yes", no: "No" }

export function sideName(side: string | undefined, code: string, line: number | null | undefined, home: string, away: string) {
  if (side !== "home" && side !== "away") return SIDE_WORD[side || ""] || side || ""
  const team = side === "home" ? home : away
  const handicap = SPECS[code]?.shape === "handicap" || /HCP$|^AH$|^SPREAD$/.test(code)
  return handicap && line != null ? team + " " + formatLine(side === "home" ? line : -line, true) : team
}
