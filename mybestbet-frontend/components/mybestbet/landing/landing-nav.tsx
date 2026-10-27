"use client"

import { SNAPSHOT } from "@/lib/api/client"
import { routes } from "@/lib/routes"
import { useStore } from "@/lib/store/emitter"
import { auth } from "@/lib/store/session"
import styles from "./landing.module.css"

export const SECTIONS = [
  ["odds", "Odds"],
  ["arbitrage", "Arbitrage"],
  ["value", "Value bets"],
  ["faq", "FAQ"],
] as const

const cx = (...names: (string | false | undefined)[]) => names.filter(Boolean).join(" ")

/** Fixed top bar with the notch of section links, plus the full-screen menu sheet below 761px. */
export function LandingNav({ menuOpen, onMenu, onTo, onGo }: {
  menuOpen: boolean
  onMenu: () => void
  onTo: (id: string) => void
  onGo: (href: string) => void
}) {
  useStore()
  const inApp = SNAPSHOT || auth.status === "in"
  const enter = () => onGo(inApp ? routes.sport("football") : "/login")
  const label = inApp ? "Open app" : "Sign in"
  return (
    <>
      <nav className={styles.nav} aria-label="Main">
        <div className={styles.edu} role="note">
          Educational demo only. Not affiliated with or licensed by any sportsbook, prediction market, league or team shown. Odds are illustrative.
        </div>
        <div className={styles.navIn}>
          <button className={styles.brand} aria-label="mybestbet, back to top" onClick={() => onTo("top")}>
            <span className={styles.wm} aria-hidden="true" />
          </button>
          <div className={styles.links}>
            {SECTIONS.map(([id, label]) => (
              <button key={id} data-to={id} onClick={() => onTo(id)}>{label}</button>
            ))}
          </div>
          <div className={styles.navR}>
            <button className={cx(styles.btn, styles.btnPri, styles.navCta)} onClick={enter}>
              {label} <span className={styles.arr} aria-hidden="true">↗</span>
            </button>
            <button
              className={styles.menuB}
              aria-label={menuOpen ? "Close menu" : "Open menu"}
              aria-expanded={menuOpen}
              aria-controls="lp-sheet"
              onClick={onMenu}
            >
              <span />
              <span />
            </button>
          </div>
        </div>
      </nav>

      <div className={cx(styles.sheet, menuOpen && styles.open)} id="lp-sheet" aria-hidden={!menuOpen}>
        <div className={styles.sheetIn}>
          {SECTIONS.map(([id, label]) => (
            <button key={id} onClick={() => onTo(id)}>{label}</button>
          ))}
        </div>
        <button className={cx(styles.btn, styles.btnPri, styles.sheetCta)} onClick={enter}>
          {label} <span className={styles.arr} aria-hidden="true">↗</span>
        </button>
      </div>
    </>
  )
}
