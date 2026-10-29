"use client"

import Link from "next/link"
import { PulseIcon, ScaleIcon, SportIcon, TicketIcon, TrendIcon, ZapIcon } from "@/components/mybestbet/shared/icons"
import { signalList } from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { prefs } from "@/lib/store/prefs"
import { money } from "@/lib/odds/format"
import { routes } from "@/lib/routes"
import { useSlipTotals } from "./bet-slip"
import { useShell } from "./shell-context"
import { useNav } from "./use-nav"
import styles from "./tab-bar.module.css"

/** Floating section bar on phones. */
export function TabBar() {
  useStore()
  const nav = useNav()
  const shell = useShell()
  const current = (on: boolean) => (on ? ("page" as const) : undefined)
  const count = prefs.slip.length
  return (
    <nav className={styles.tabbar} aria-label="Sections">
      <Link href={nav.sportHref} aria-current={current(nav.section === "list" || nav.section === "match" || nav.section === "player")}>
        <SportIcon sport={nav.sport} />
        <span>Matches</span>
      </Link>
      <Link href={routes.signals("arb")} data-tour="tab-arbs" aria-current={current(nav.section === "signals" && nav.kind === "arb")}>
        <ZapIcon />
        <span className={styles.cnt + " " + styles.g}>{signalList("arb").length}</span>
        <span>Arbs</span>
      </Link>
      <Link href={routes.signals("ev")} data-tour="tab-value" aria-current={current(nav.section === "signals" && !!nav.kind && nav.kind !== "arb")}>
        <TrendIcon />
        <span>Value</span>
      </Link>
      <Link href={routes.sources()} aria-current={current(nav.section === "sources")}>
        <PulseIcon />
        <span>Sources</span>
      </Link>
      <Link href={routes.baysed()} aria-current={current(nav.section === "baysed")}>
        <ScaleIcon />
        <span>Baysed</span>
      </Link>
      <button onClick={shell.openSlip}>
        <TicketIcon />
        {count > 0 && <span className={styles.cnt}>{count}</span>}
        <span>Slip</span>
      </button>
    </nav>
  )
}

/** "2 Bet slip 6,590.00": opens the slip on phones when it has picks. */
export function SlipBar() {
  const shell = useShell()
  const { returns } = useSlipTotals()
  return (
    <button className={styles.slipbar} onClick={shell.openSlip}>
      <span className={styles.c}>{prefs.slip.length}</span>Bet slip<span className={styles.r}>{money(returns)}</span>
    </button>
  )
}
