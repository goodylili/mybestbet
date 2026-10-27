// Shapes of the mybestbet API (OpenAPI 3.1, /v1/*). Mirrors tools/mockapi/types.go.

export type PriceForm = "decimal" | "share" | "cents" | "naira" | "line_only" | (string & {})

export type Team = { id: string; name: string; logoUrl?: string }

export type League = {
  code: string
  name: string
  country: string
  sport: string
  fixtureCount?: number
  logoUrl?: string
}

export type SportSummary = {
  sport: string
  fixtureCount: number
  leagues: League[]
  matchMarkets: string[]
  playerProps: string[]
  markets?: MarketSpec[]
}

export type MarketSpec = { code: string; label: string; kind: string; shape: MarketShape; periods: string[]; statField?: string; regulationOnly?: boolean }

export type EventStatus = "scheduled" | "live" | "finished" | "postponed" | "cancelled" | (string & {})

export type EventInfo = {
  id: string
  sport: string
  leagueCode: string
  home: Team
  away: Team
  kickoff: string
  status: EventStatus
  score?: EventScore
  links?: Record<string, string>
}

export type EventScore = { home: number; away: number; period?: string; minute?: number; clock?: string; detail?: string }

export type CalendarDay = { date: string; count: number }

export type EventCalendar = { days: CalendarDay[] }

export type Price = {
  raw: number
  form: PriceForm
  prob: number
  bid?: number
  ask?: number
  liquidity?: number
  suspended?: boolean
  observedAt?: string
}

export type BestPrice = { source: string; price: Price; deepLink?: string }

export type BestOutcomes = { market: string; line?: number; period?: string; home?: BestPrice; draw?: BestPrice; away?: BestPrice }

export type Badges = { arbs: number; evs: number; maxMargin?: number; maxEv?: number }

export type EventSummary = EventInfo & { sources: string[]; best: BestOutcomes; badges: Badges }

export type MarketKey = {
  eventId: string
  code: string
  line?: number | null
  playerId?: string
  period?: string
  teamId?: string
}

export type BoardCell = {
  marketKey: string
  source: string
  side?: string
  label?: string
  price: Price
  fair?: number
  derived?: boolean
  deepLink?: string
}

export type SelectionRow = {
  side: string
  label: string
  cells: Record<string, BoardCell>
  consensus?: BoardCell
  bestSource?: string
}

export type MarketBoard = { key: MarketKey; keyString: string; selections: SelectionRow[] }

export type MarketPeriod = { period: string; boards: MarketBoard[] }

export type MarketShape = "three_way" | "two_way" | "over_under" | "handicap" | (string & {})

export type MarketTab = { code: string; kind: string; shape: MarketShape; periods: MarketPeriod[] }

export type EventDetail = { event: EventInfo; sources: string[]; markets: MarketTab[] }

export type HitRate = { window: number; matches: number; hits: number; rate: number }

export type PlayerMatchStat = {
  eventId: string
  teamId?: string
  opponentTeamId?: string
  isHome: boolean
  playedAt: string
  minutes: number
  started: boolean
  goals: number
  assists: number
  shots: number
  shotsOnTarget: number
  fouls: number
  tackles: number
  yellow: number
  red: number
  rating?: number
  stats?: Record<string, number>
}

export type MinutesPoint = { eventId: string; playedAt: string; minutes: number; started: boolean }

export type RatingBucket = { from: number; to: number; count: number }

export type RatingDistribution = { ratings: number[]; mean: number; min: number; max: number; buckets: RatingBucket[] }

export type Research = {
  windows: HitRate[]
  home: HitRate
  away: HitRate
  vsOpponent: HitRate
  vsOpponentMatches: PlayerMatchStat[]
  minutesTrend: MinutesPoint[]
  lineupStatus: string
  benchWarning: boolean
  ratings?: RatingDistribution
}

export type PropPlayer = { id: string; name: string; teamId?: string }

export type PropRow = {
  key: MarketKey
  keyString: string
  player: PropPlayer
  selections: SelectionRow[]
  hitRate?: HitRate
  research?: Research
}

export type PropBoard = { event: EventInfo; sources: string[]; rows: PropRow[] }

export type PropCell = {
  raw: number
  form?: PriceForm
  prob?: number
  bid?: number
  ask?: number
  liquidity?: number
  suspended?: boolean
  fair?: number
  derived?: boolean
  deepLink?: string
}

export type PropSelection = { side: string; label: string; bestSource?: string; consensus?: PropCell; cells: Record<string, PropCell> }

export type HitRateWire = { window?: number; matches?: number; hits?: number }

export type PropResearch = { windows: HitRateWire[]; home: HitRateWire; away: HitRateWire; vsOpponent: HitRateWire; vsOpponentMatches?: PlayerMatchStat[] }

export type PropPlayerInfo = PropPlayer & { minutesTrend: MinutesPoint[]; lineupStatus: string; benchWarning: boolean; ratings?: RatingDistribution }

export type PropRowWire = { keyString: string; selections: PropSelection[]; research?: PropResearch }

export type PropBoardWire = { event: EventInfo; sources: string[]; players: PropPlayerInfo[]; rows: PropRowWire[] }

export type PlayerStats = { player: PropPlayer; items: PlayerMatchStat[]; err?: string }

export type HistoryPoint = {
  source: string
  side: string
  label: string
  raw: number
  form: PriceForm
  prob: number
  suspended: boolean
  observedAt: string
}

export type MarketHistory = { marketKey: string; points: HistoryPoint[] }

export type Leg = {
  source: string
  side?: string
  label?: string
  line?: number
  price: Price
  stake?: number
  deepLink?: string
  observedAt: string
}

export type SignalType = "arb" | "ev" | "line" | "move"

export type Signal = {
  id: string
  dedupeKey: string
  type: SignalType
  status: string
  eventId: string
  leagueCode: string
  market: MarketKey
  legs: Leg[]
  margin?: number
  ev?: number
  fairProb?: number
  oldestLegAge: number
  detectedAt: string
  closedAt?: string
}

export type SignalView = Signal & { event: EventInfo; player?: PropPlayer }

export type SourceHealth = {
  name: string
  enabled: boolean
  degraded: boolean
  lastSuccess?: string
  lastError: string
  consecutiveFailures: number
  requests: number
  errors: number
  errorRate: number
  lastFetchMs: number
  freshnessMs: number
  logoUrl?: string
  type?: "sportsbook" | "prediction" | "pickem" | (string & {})
  note?: string
  countries?: string[]
  blockedCountries?: string[]
}

export type BoardUpdate = { eventId: string; cells?: BoardCell[]; signals?: Signal[]; score?: EventScore; sentAt: string }

export type EventBest = { eventId: string; best?: BestOutcomes; score?: EventScore }

export type List<T> = { items: T[] }

export type SearchItem = { kind: "team" | "player" | "league" | "event" | (string & {}); id: string; name: string; sport: string; leagueCode?: string; logoUrl?: string; awayLogoUrl?: string; kickoff?: string }

export type AuthProvider = "password" | "google" | (string & {})

export type User = {
  id: string
  email: string
  name?: string
  avatarUrl?: string
  emailVerified: boolean
  hasPassword: boolean
  providers: AuthProvider[]
  createdAt: string
}

export type UserBody = { user: User }

export type Settings = Record<string, unknown>

export type SettingsBody = { settings: Settings }

export type Meta = { auth?: { google: boolean }; country?: string }

export type OtpPurpose = "signup" | "reset"

export type CodeSent = { status: "code_sent"; resendIn: number }

export type ErrorBody = { error?: { code?: string; message?: string; resendIn?: number }; resendIn?: number }

export type BookingMode = "split" | "single"

export type BookingSide = "" | "home" | "draw" | "away" | "over" | "under" | "yes" | "no" | (string & {})

export type BookingLeg = { eventId: string; marketKey: string; source: string; side: BookingSide; label: string }

export type BookingRequest = { legs: BookingLeg[]; mode?: BookingMode; book?: string; stake?: number }

export type BookingStatus = "ok" | "partial" | "failed" | "blocked" | "unsupported" | (string & {})

export type BookedLeg = { index: number; price?: number; ok: boolean; reason?: string }

export type BookResult = {
  book: string
  status: BookingStatus
  code?: string
  webUrl?: string
  appUrl?: string
  expiresAt?: string
  odds?: number
  return?: number
  legs: BookedLeg[]
  message?: string
  cached: boolean
}

export type ManualLeg = { index: number; source: string; deepLink?: string; price?: number; reason: string }

export type QuoteLeg = { index: number; source: string; price: number }

export type SplitQuote = { odds: number; return: number; covered: number; legs: QuoteLeg[] }

export type BookQuote = { book: string; bookable: boolean; covered: number; total: number; odds: number; return: number; legs: QuoteLeg[]; missing: number[] }

export type BookingComparison = { stake: number; totalLegs: number; picked: SplitQuote; bestSplit: SplitQuote; books: BookQuote[] }

export type BookingResponse = { mode: BookingMode; book?: string; bookings: BookResult[]; manual: ManualLeg[]; comparison: BookingComparison }
