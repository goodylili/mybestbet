"use client"

import { usePathname } from "next/navigation"
import { useEffect } from "react"
import type { SignalType } from "@/lib/api/types"
import { routes } from "@/lib/routes"
import { useStore } from "@/lib/store/emitter"
import { fixture, leagueSport } from "@/lib/store/data"
import { signalBySlug } from "@/lib/odds/markets"

export type Section = "list" | "match" | "player" | "signals" | "sources" | "baysed" | "other"

let lastSport = "football"
let lastList = ""

const decode = (s: string) => {
  try {
    return decodeURIComponent(s)
  } catch {
    return s
  }
}

/**
 * Where the viewer is in the odds app: the section, the sport the sidebar and
 * tab bar follow (it sticks while on signals or sources), and where "Matches"
 * leads back to.
 */
export function useNav() {
  useStore()
  const pathname = usePathname() || "/"
  const [, head = "", rest = ""] = pathname.split("/")
  const param = decode(rest)
  let section: Section = "other"
  let sport = lastSport
  let league: string | null = null
  let kind: SignalType | null = null
  if (head === "sports") {
    section = "list"
    sport = param
  } else if (head === "leagues") {
    section = "list"
    league = param
    sport = leagueSport(param)
  } else if (head === "favourites") {
    section = "list"
    league = "STAR"
  } else if (head === "events") {
    section = "match"
    sport = fixture(param)?.sp || lastSport
  } else if (head === "players") {
    section = "player"
  } else if (head === "signals") {
    section = "signals"
    kind = signalBySlug(param) || null
  } else if (head === "sources") {
    section = "sources"
  } else if (head === "baysed") {
    section = "baysed"
  }
  useEffect(() => {
    lastSport = sport
    if (section === "list") lastList = pathname
  }, [sport, section, pathname])
  return {
    pathname,
    section,
    sport,
    league,
    kind,
    /** The sport's full list, with no league picked. */
    sportHref: routes.sport(sport),
    /** The last list the viewer was on, so "Matches" returns to it. */
    listHref: (section === "list" ? pathname : lastList) || routes.sport(sport),
  }
}
