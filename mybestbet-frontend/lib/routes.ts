import { SIGNALS } from "@/lib/odds/markets"
import type { SignalType } from "@/lib/api/types"

/** Every URL in the odds app, so links are built in one place. */
export const routes = {
  home: () => "/",
  sport: (sport: string) => `/sports/${sport}`,
  league: (code: string) => `/leagues/${encodeURIComponent(code)}`,
  favourites: () => "/favourites",
  event: (id: string, market?: string) => `/events/${encodeURIComponent(id)}` + (market ? `?market=${encodeURIComponent(market)}` : ""),
  player: (id: string, eventId?: string, key?: string) =>
    `/players/${encodeURIComponent(id)}` + (eventId ? `?event=${encodeURIComponent(eventId)}` + (key ? `&key=${encodeURIComponent(key)}` : "") : ""),
  signals: (type: SignalType, open?: string) => `/signals/${SIGNALS[type].slug}` + (open ? `?open=${encodeURIComponent(open)}` : ""),
  sources: () => "/sources",
}
