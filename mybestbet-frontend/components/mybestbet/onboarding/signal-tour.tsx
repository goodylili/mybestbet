"use client"

import dynamic from "next/dynamic"
import { useCallback, useEffect, useState } from "react"
import type { SignalType } from "@/lib/api/types"
import { signalList } from "@/lib/store/data"
import type { Stop } from "./spotlight-tour"

const SpotlightTour = dynamic(() => import("./spotlight-tour").then((m) => m.SpotlightTour), { ssr: false })

type TourType = Extract<SignalType, "arb" | "ev">

const NAV_TEXT = "These tabs switch between Arbitrage, Value bets, Line gaps and Steam moves, and Matches at the top takes you back to every price."

const openFirst = () => {
  document.querySelector<HTMLButtonElement>('[data-tour="signals-list"] .tap-area[aria-expanded="false"]')?.click()
}

const TOURS: Record<TourType, Stop[]> = {
  arb: [
    {
      sel: ['[data-tour="arb-stats"]'],
      title: "What an arb is",
      text: () => {
        const n = signalList("arb").length
        return "Books price the same match differently. When the best price on every outcome adds up to less than 100% implied chance, backing all of them locks in a profit whatever the result." + (n ? " " + n + " are open right now." : "")
      },
    },
    {
      sel: ['[data-tour="arb-legs"]'],
      title: "Back each outcome at its book",
      text: () => "Each row lists one price per outcome and the book paying it. Tap a book's name or logo to open that match on the book and check the price is still there.",
    },
    {
      sel: ['[data-tour="arb-calc"]'],
      title: "Split your stake",
      text: () => "Enter what you want to put in. The calculator splits it across the books so every outcome pays back the same, and shows the profit you lock in. Rounding keeps stakes looking natural.",
      before: openFirst,
    },
    {
      sel: ['[data-tour="signal-nav"]'],
      title: "Find your way around",
      text: () => "Sort by profit or kick-off, set a minimum and filter by sport below. " + NAV_TEXT,
    },
  ],
  ev: [
    {
      sel: ['[data-tour="signals-list"] > :first-child'],
      title: "What a value bet is",
      text: () => "We strip each book's margin and blend every book into a fair chance for each outcome. When one book pays more than that fair price, the bet is worth more than it costs over time. The green number is that edge.",
    },
    {
      sel: ['[data-tour="ev-detail"]'],
      title: "Price against fair chance",
      text: () => "The book's price implies one chance, and the fair chance is higher. The fair price is the lowest odds still worth taking, so anything above it is value.",
      before: openFirst,
    },
    {
      sel: ['[data-tour="ev-actions"]'],
      title: "Check it, then place it",
      text: () => "Open the book straight on this match to check the price is still live, or compare every book's price for the match first.",
      before: openFirst,
    },
    {
      sel: ['[data-tour="signal-nav"]'],
      title: "Find your way around",
      text: () => "Value pays off over many bets, not one, so keep stakes small and steady. Sort by edge or kick-off, set a minimum edge and filter by sport below. " + NAV_TEXT,
    },
  ],
}

const key = (type: TourType) => "mbb.tour." + type

const seen = (name: string) => {
  try {
    return !!localStorage.getItem(name)
  } catch {
    return true
  }
}

const markSeen = (name: string) => {
  try {
    localStorage.setItem(name, "1")
  } catch {}
}

const prefersReduced = () => typeof window !== "undefined" && !!window.matchMedia && matchMedia("(prefers-reduced-motion: reduce)").matches

export const hasSignalTour = (type: SignalType): type is TourType => type === "arb" || type === "ev"

let replayListener: (() => void) | null = null

/** Start the current signal page's tour again. */
export function replaySignalTour() {
  replayListener?.()
}

/** Four-stop walkthrough of the arbitrage or value bets page, shown once after the main tour and on demand. */
export function SignalTour({ type, ready }: { type: TourType; ready: boolean }) {
  const [phase, setPhase] = useState<"open" | "closing" | null>(null)
  const [reduced] = useState(prefersReduced)

  useEffect(() => {
    if (!ready || phase || seen(key(type)) || !seen("mbb.gate") || !seen("mbb.tour")) return
    const id = setTimeout(() => setPhase("open"), 700)
    return () => clearTimeout(id)
  }, [ready, phase, type])

  useEffect(() => {
    replayListener = () => { if (ready) setPhase("open") }
    return () => { replayListener = null }
  }, [ready])

  const end = useCallback(() => {
    markSeen(key(type))
    setPhase("closing")
    setTimeout(() => setPhase(null), reduced ? 0 : 260)
  }, [reduced, type])

  if (!phase) return null
  return <SpotlightTour stops={TOURS[type]} label={type === "arb" ? "Arbitrage tour" : "Value bets tour"} tip={false} closing={phase === "closing"} reduced={reduced} onEnd={end} />
}
