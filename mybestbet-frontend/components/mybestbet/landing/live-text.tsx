"use client"

import { useSyncExternalStore } from "react"
import { COMPS } from "@/lib/catalog"
import { useStore } from "@/lib/store/emitter"
import { S, signalList } from "@/lib/store/data"

export type CountKind = "books" | "comps" | "arbs"

const noop = () => () => {}

/** False while rendering on the server and hydrating, true after. */
export const useMounted = () => useSyncExternalStore(noop, () => true, () => false)

export const counts = (): Record<CountKind, number> => ({
  books: S.sources.length,
  comps: COMPS.length,
  arbs: signalList("arb").length,
})

/** A live count in running copy. Shows the written default until the page is running. */
export function Count({ kind, fallback = "16" }: { kind: CountKind; fallback?: string }) {
  useStore()
  const mounted = useMounted()
  return <>{mounted ? counts()[kind] : fallback}</>
}

/** ", including Bet365, Betano, Betway, SportyBet, Polymarket and Kalshi" from the loaded sources. */
export function SourceNames() {
  useStore()
  const mounted = useMounted()
  if (!mounted) return null
  const books = S.sources.filter((s) => s.kind === "book").slice(0, 4).map((s) => s.name)
  const markets = S.sources.filter((s) => s.kind === "pm").slice(0, 2).map((s) => s.name)
  const all = books.concat(markets)
  return all.length > 1 ? <>, including {all.slice(0, -1).join(", ")} and {all[all.length - 1]}</> : null
}
