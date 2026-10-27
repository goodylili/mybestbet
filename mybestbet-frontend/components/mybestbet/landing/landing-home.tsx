"use client"

import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react"
import { useRouter } from "next/navigation"
import { routes } from "@/lib/routes"
import { startSession } from "@/lib/store/session"
import { Hero } from "./hero"
import { LandingFooter } from "./landing-footer"
import { LandingNav } from "./landing-nav"
import { Coverage, Cta, Faq, Feature, How, Stats } from "./sections"
import { useBands, useHeroMorph, useReveal, useScrollFrame, useStackCards } from "./use-landing-motion"
import { usePreviews } from "./use-previews"
import styles from "./landing.module.css"

const REDUCED = "(prefers-reduced-motion: reduce)"

const watchMotion = (fn: () => void) => {
  const mq = matchMedia(REDUCED)
  mq.addEventListener("change", fn)
  return () => mq.removeEventListener("change", fn)
}

/** The marketing page at "/": hero with live previews, features, how it works, coverage, FAQ and footer. */
export function LandingHome() {
  const router = useRouter()
  const root = useRef<HTMLDivElement>(null)
  const menuRef = useRef(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const motion = !useSyncExternalStore(watchMotion, () => matchMedia(REDUCED).matches, () => false)
  const previews = usePreviews()

  useEffect(() => {
    startSession(true)
    router.prefetch(routes.sport("football"))
  }, [router])

  useEffect(() => {
    menuRef.current = menuOpen
    if (!menuOpen) return
    const body = document.body
    body.style.overflow = "hidden"
    return () => { body.style.overflow = "" }
  }, [menuOpen])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || !menuRef.current) return
      setMenuOpen(false)
      root.current?.querySelector<HTMLElement>("." + styles.menuB)?.focus()
    }
    document.addEventListener("keydown", onKey)
    return () => document.removeEventListener("keydown", onKey)
  }, [])

  useReveal(root)
  useScrollFrame(root, menuRef)
  useHeroMorph(root)
  useStackCards(root)
  useBands(root)

  const scrollTo = useCallback((id: string) => {
    setMenuOpen(false)
    const behavior = matchMedia(REDUCED).matches ? "auto" : "smooth"
    if (id === "top") { window.scrollTo({ top: 0, behavior }); return }
    const el = document.getElementById(id)
    if (!el) return
    const y = el.getBoundingClientRect().top + window.scrollY - (window.innerWidth < 600 ? 72 : 96)
    window.scrollTo({ top: Math.max(0, y), behavior })
  }, [])

  const go = useCallback((href: string) => {
    setMenuOpen(false)
    router.push(href)
  }, [router])

  return (
    <div ref={root} className={styles.root + (motion ? " " + styles.js : "")}>
      <LandingNav menuOpen={menuOpen} onMenu={() => setMenuOpen(!menuOpen)} onTo={scrollTo} onGo={go} />

      <main>
        <Hero previews={previews} />
        <Stats />

        <Feature
          id="odds"
          labelId="lp-f1"
          code="EPL"
          photoName="odds"
          kind="odds"
          title={"Every price.\nOne glance."}
          body="We line up the price from every book we track and mark the best one for each outcome, so you never take a worse number by accident."
          checks={["Best price marked for every outcome", "Decimal, fractional or American odds", "Prices refresh every few seconds"]}
          cta="Compare today's odds"
          previews={previews}
          onGo={go}
        />

        <div className={styles.band}>
          <Feature
            id="arbitrage"
            labelId="lp-f2"
            code="NBA"
            photoName="arena"
            kind="arb"
            flip
            title={"Back every side.\nKeep the difference."}
            body="When books disagree enough, backing each outcome at a different book pays back more than you stake. We find those gaps and split the stakes for you."
            checks={["Stakes worked out for your bankroll", "Profit shown before you place anything", "Closed gaps disappear straight away"]}
            cta="See open arbs"
            previews={previews}
            onGo={go}
          />
        </div>

        <Feature
          id="value"
          labelId="lp-f3"
          code="SERIEA"
          photoName="ball"
          kind="value"
          title={"Prices that pay\nmore than they should."}
          body="We build a fair line from the sharpest books and flag any price that beats it, with the edge shown as a plain percentage."
          checks={["Fair odds next to every pick", "Edge sorted from largest to smallest", "Telegram alert when a big one lands"]}
          cta="Find value bets"
          previews={previews}
          onGo={go}
        />

        <div className={styles.band}>
          <How />
          <Coverage />
        </div>

        <Faq />
        <Cta onGo={go} />
      </main>

      <LandingFooter onTo={scrollTo} onGo={go} />
    </div>
  )
}
