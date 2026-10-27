import type { SignalType } from "@/lib/api/types"
import { regulationOnly } from "./markets"

const FOOTBALL_REG = "Settles on the 90 minutes plus stoppage time. Extra time and penalties don't count."

const MARKET: Record<string, string> = {
  "1X2": "Pick the home win (1), the draw (X) or the away win (2) at the end of normal time.",
  DC: "Covers two of the three results in one bet: home or draw (1X), home or away (12), or draw or away (X2).",
  OU: "Bet on whether the total goals scored by both teams goes over or under the line.",
  BTTS: "Yes wins if both teams score at least once. No wins if either team keeps a clean sheet.",
  DNB: "Back the home or away win. If the match is drawn your stake is returned.",
  AH: "One team starts with a goal head start or deficit. Half lines can't push; whole lines refund on a tie; quarter lines split your stake across the two nearest lines.",
  CS: "Pick the exact final score.",
  HTFT: "Pick the result at half time and at full time together, e.g. draw / home.",
  "1H_1X2": "Home, draw or away based only on the score at half time.",
  CORN_OU: "Over or under the total number of corners taken by both teams.",
  CORN: "Over or under the total number of corners taken by both teams.",
  CARDS: "Over or under the total number of cards shown in the match.",
  T_CARDS: "Over or under the total number of cards shown in the match.",
  OE: "Whether the total number of goals ends up odd or even.",
  TG: "Over or under the number of goals scored by one team.",
  T_GOALS: "Over or under the number of goals scored by one team.",
  T_SHOTS: "Total shots taken by one team, grouped into ranges.",
  T_PASSES: "Total passes made by one team, grouped into ranges.",
  QUALIFY: "Which team goes through to the next round, however it's decided.",
  SPREAD: "The favourite has to win by more than the points spread; the underdog can lose by less and still cover.",
  TOTAL: "Over or under the combined points scored by both teams.",
  SETS: "One player starts with a set head start or deficit.",
  SET_HCP: "One player starts with a set head start or deficit.",
  GAMES: "Over or under the total games played in the match.",
  TOTAL_GAMES: "Over or under the total games played in the match.",
  MAPS: "One team starts with a map head start or deficit.",
  MAP_HCP: "One team starts with a map head start or deficit.",
  TMAPS: "Over or under the total maps played in the series.",
  TOTAL_MAPS: "Over or under the total maps played in the series.",
  P_SCORER: "The player scores at any time in the match. Own goals don't count.",
  P_ANYTIME: "The player scores at any time in the match. Own goals don't count.",
  P_FIRST: "The player scores the first goal of the match.",
  P_LAST: "The player scores the last goal of the match.",
  P_GA: "The player scores or assists at least once.",
  P_GOALS: "Over or under the number of goals the player scores.",
  P_SOT: "Over or under the player's shots on target.",
  P_SHOTS: "Over or under the player's total shots, on or off target.",
  P_AST: "Over or under the number of assists the player makes.",
  P_ASSISTS: "Over or under the number of assists the player makes.",
  P_CARD: "The player is shown a yellow or red card.",
  P_TACKLES: "Over or under the player's tackles.",
  P_FOULS: "Over or under the fouls the player commits.",
  P_SAVES: "Over or under the saves the goalkeeper makes.",
  P_PASSES: "Over or under the passes the player completes.",
  P_RATING: "Over or under the player's match rating from the rating provider. Usually void if the player doesn't start.",
}

const PROP_NOTE = " Most books void the bet if the player doesn't play."

/** A short explanation of a market, or null when there is nothing worth saying. */
export function marketInfo(code: string, sport: string): string | null {
  if (code === "ML") {
    if (sport === "tennis") return "Pick the player who wins the match. If a player retires, books settle it differently, so check the book's rules."
    if (sport === "esports") return "Pick the team that wins the series."
    return "Pick the team that wins the game, overtime included."
  }
  const text = MARKET[code] || (code.startsWith("BK_") || code.startsWith("NF_") ? "Over or under the player's total for this stat." : null)
  if (!text) return null
  return code.startsWith("P_") && code !== "P_RATING" ? text + PROP_NOTE : text
}

export type RegTag = { label: string; text: string }

/** Whether a market settles on regulation time, for sports where extra time can happen. */
export function regTag(code: string, sport: string): RegTag | null {
  if (sport === "tennis" || sport === "esports") return null
  const reg = regulationOnly(code)
  if (reg == null) return null
  if (sport === "football") {
    return reg
      ? { label: "Reg time", text: FOOTBALL_REG }
      : { label: "Incl. ET", text: "Extra time and penalties count." }
  }
  return reg
    ? { label: "Reg time", text: "Settles on regulation time. Overtime doesn't count." }
    : { label: "Incl. OT", text: "Overtime counts." }
}

export const TERM = {
  consensus: "The fair price for this outcome, built by stripping each book's margin and blending every book's view. It's a guide, not a bet you can place.",
  noMargin: "Shows each book's price with its built-in margin removed, so you can compare what every book really thinks the chance is.",
  best: "The highest price any book is paying for this outcome right now.",
  margin: "The book's built-in edge: how far its prices on every outcome add up above 100% implied chance. Lower is better for you.",
  derived: "Not listed directly by the book. Worked out from its other markets, so you can't bet it as shown.",
  suspended: "The book has paused betting on this market, usually after a goal, a red card or breaking news.",
  share: "Prediction markets trade shares that pay out if the outcome happens. The small line shows the sell and buy price in cents (40¢ is a 40% chance, 2.50 in decimal odds) and how much money is in the market.",
  booksScanned: "How many books we're checking for arbs right now. Switch books on or off in Books.",
  arbProfit: "Your locked-in return on the total stake, whatever the result, if you get every leg at these prices.",
  arbStake: "How to split your stake so every outcome pays back the same amount.",
  rounding: "Rounds each stake to a natural-looking amount. Returns move by a few cents.",
  evEdge: "How much more the book pays than the fair price. A 5% edge means you expect to make 5% of your stake per bet over time.",
  fairChance: "The chance of this outcome once every book's margin is removed and their views are blended.",
  impliedChance: "The chance this book's price implies: 1 divided by the decimal odds.",
  fairPrice: "The lowest odds still worth taking. Anything above it is value.",
  lineGap: "Books disagree on the line itself, e.g. one has 2.5 goals and another 3. Taking the softer line can be value.",
  move: "A fast price move across several books at once, usually from sharp money or team news. The books that haven't moved yet may still offer the old price.",
} as const

export const SIGNAL_INFO: Record<SignalType, string> = {
  arb: "An arb is when backing every outcome at different books guarantees a profit, because the best prices add up to less than 100% implied chance. The percentage is your return on the total stake.",
  ev: TERM.evEdge,
  line: TERM.lineGap,
  move: TERM.move,
}
