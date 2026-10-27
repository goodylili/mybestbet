"use client"

import { routes } from "@/lib/routes"
import { Count } from "./live-text"
import styles from "./landing.module.css"

/** Always-dark footer band with the giant wordmark that slides up into view. */
export function LandingFooter({ onTo, onGo }: { onTo: (id: string) => void; onGo: (href: string) => void }) {
  return (
    <div className={styles.band + " " + styles.bandEnd}>
      <footer className={styles.foot}>
        <div className={styles.wrap}>
          <div className={styles.footTop}>
            <div className={styles.footB}>
              <span className={styles.wm} aria-hidden="true" />
              <p>Live odds from <span><Count kind="books" /></span> books and prediction markets, side by side.</p>
            </div>
            <div className={styles.footC}>
              <p className={styles.footT}>Product</p>
              <button onClick={() => onTo("odds")}>Best odds</button>
              <button onClick={() => onTo("arbitrage")}>Arbitrage</button>
              <button onClick={() => onTo("value")}>Value bets</button>
            </div>
            <div className={styles.footC}>
              <p className={styles.footT}>Help</p>
              <button onClick={() => onTo("faq")}>FAQ</button>
              <button onClick={() => onGo(routes.sources())}>Sources</button>
              <button onClick={() => onGo(routes.sport("football"))}>Open app</button>
            </div>
          </div>
          <div className={styles.footS}>
            <p>mybestbet is an educational demo. It is not affiliated with, endorsed by or licensed by any sportsbook, prediction market, league or team shown. Names and logos belong to their owners and odds are illustrative. mybestbet does not take bets. 18+ only. Photos from Unsplash, video from Pexels.</p>
            <p>© 2026 mybestbet</p>
          </div>
        </div>
        <div className={styles.big} aria-hidden="true"><span>mybestbet</span></div>
      </footer>
    </div>
  )
}
