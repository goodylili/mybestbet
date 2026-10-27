"use client"

import dynamic from "next/dynamic"
import { usePathname, useRouter } from "next/navigation"
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react"
import { useNav } from "@/components/mybestbet/shell/use-nav"
import { storeReady } from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { WelcomeGate } from "./welcome-gate"

const SpotlightTour = dynamic(() => import("./spotlight-tour").then((m) => m.SpotlightTour), { ssr: false })

const KEY_GATE = "mbb.gate"
const KEY_TOUR = "mbb.tour"
const memory: Record<string, string> = {}

const get = (key: string) => {
  if (memory[key]) return memory[key]
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

const set = (key: string, value: string) => {
  memory[key] = value
  try {
    localStorage.setItem(key, value)
  } catch {}
}

let replayListener: (() => void) | null = null

/** Show the intro again: the gate if it was never passed, otherwise the tour. */
export function replayIntro() {
  replayListener?.()
}

type Phase = "open" | "closing" | null

const prefersReduced = () => typeof window !== "undefined" && !!window.matchMedia && matchMedia("(prefers-reduced-motion: reduce)").matches

/** The first-visit welcome gate, then the spotlight tour of the odds app. */
export function Onboarding() {
  const version = useStore()
  const pathname = usePathname()
  const router = useRouter()
  const nav = useNav()
  const [gate, setGateState] = useState<Phase>(null)
  const [tour, setTourState] = useState<Phase>(null)
  const [reduced] = useState(prefersReduced)
  const gateRef = useRef<Phase>(null)
  const tourRef = useRef<Phase>(null)
  const navRef = useRef(nav)
  useEffect(() => {
    navRef.current = nav
  })

  const setGate = useCallback((p: Phase) => { gateRef.current = p; setGateState(p) }, [])
  const setTour = useCallback((p: Phase) => { tourRef.current = p; setTourState(p) }, [])

  const openGate = useCallback(() => {
    if (gateRef.current) return
    document.body.classList.add("ob-open")
    setGate("open")
  }, [setGate])

  const closeGate = useCallback(() => {
    if (gateRef.current !== "open") return
    document.body.classList.remove("ob-open")
    setGate("closing")
    setTimeout(() => setGate(null), reduced ? 0 : 280)
  }, [reduced, setGate])

  /** The tour starts on a match list, so the first stop has prices to point at. */
  const startTour = useCallback((go = false) => {
    if (tourRef.current) return
    if (navRef.current.section !== "list") {
      if (!go) return
      router.push(navRef.current.sportHref)
    }
    setTour("open")
  }, [router, setTour])

  const endTour = useCallback(() => {
    if (tourRef.current !== "open") return
    setTour("closing")
    setTimeout(() => setTour(null), reduced ? 0 : 260)
    set(KEY_TOUR, "1")
  }, [reduced, setTour])

  useLayoutEffect(() => {
    if (!get(KEY_GATE)) openGate()
  }, [openGate])

  useEffect(() => {
    if (gate || tour || !storeReady() || !get(KEY_GATE)) return
    if (!get(KEY_TOUR)) {
      const id = setTimeout(() => { if (!tourRef.current && !gateRef.current) startTour() }, 600)
      return () => clearTimeout(id)
    }
  }, [version, pathname, gate, tour, openGate, startTour])

  useEffect(() => {
    replayListener = () => (get(KEY_GATE) ? startTour(true) : openGate())
    return () => { replayListener = null }
  }, [openGate, startTour])

  useEffect(() => () => document.body.classList.remove("ob-open"), [])

  useEffect(() => {
    document.body.style.overflow = gate === "open" ? "hidden" : ""
  }, [gate])

  const enter = () => {
    set(KEY_GATE, "1")
    setTimeout(closeGate, reduced ? 0 : 380)
    if (!get(KEY_TOUR)) setTimeout(startTour, reduced ? 0 : 700)
  }

  const leave = () => {
    closeGate()
    router.push("/")
  }

  return (
    <>
      {gate && <WelcomeGate closing={gate === "closing"} reduced={reduced} onEnter={enter} onLeave={leave} />}
      {tour && <SpotlightTour closing={tour === "closing"} reduced={reduced} onEnd={endTour} />}
    </>
  )
}
