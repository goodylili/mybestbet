import type { EventSummary, List, SportSummary } from "@/lib/api/types"
import { serverJson, serverSession } from "@/lib/api/server"
import type { BoardSeed as Seed } from "@/lib/store/data"
import { SeedPrime } from "./seed-prime"

const BACK = 6 * 3600e3
const AHEAD = 30 * 3600e3

async function loadSeed(): Promise<Seed | null> {
  const cookie = await serverSession()
  if (!cookie) return null
  const now = Date.now(), from = now - BACK, to = now + AHEAD
  const range = "from=" + encodeURIComponent(new Date(from).toISOString()) + "&to=" + encodeURIComponent(new Date(to).toISOString())
  try {
    const [sports, events] = await Promise.all([serverJson<List<SportSummary>>("/v1/sports", cookie), serverJson<List<EventSummary>>("/v1/events?" + range, cookie)])
    return { from, to, sports: sports.items, events: events.items }
  } catch {
    return null
  }
}

export async function BoardSeed() {
  const seed = await loadSeed()
  return seed ? <SeedPrime seed={seed} /> : null
}
