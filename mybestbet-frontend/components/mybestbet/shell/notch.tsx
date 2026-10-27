"use client"

import Link from "next/link"
import type { Ref } from "react"
import { CloseIcon, SearchIcon } from "@/components/mybestbet/shared/icons"
import { signalList } from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { routes } from "@/lib/routes"
import { useShell } from "./shell-context"
import { SearchResults } from "./search-results"
import { useSearchInput } from "./use-search-input"
import { useNav } from "./use-nav"
import styles from "./notch.module.css"

/** The section switcher hanging from the top bar on tablet and desktop, which turns into a search field. */
export function Notch({ ref, searching, setSearching }: {
  ref?: Ref<HTMLElement>
  searching: boolean
  setSearching: (on: boolean) => void
}) {
  useStore()
  const nav = useNav()
  const shell = useShell()
  const search = useSearchInput()
  const current = (on: boolean) => (on ? ("page" as const) : undefined)
  const matches = nav.section === "list" || nav.section === "match" || nav.section === "player"

  const open = (e: React.MouseEvent<HTMLButtonElement>) => {
    setSearching(true)
    const input = e.currentTarget.parentElement?.querySelector("input")
    setTimeout(() => {
      input?.focus()
      input?.select()
    })
  }

  return (
    <nav ref={ref} className={styles.anotch + (searching ? " " + styles.searching : "")} aria-label="Sections">
      <Link href={nav.listHref} aria-current={current(matches)}>Matches</Link>
      <Link href={routes.signals("arb")} data-tour="notch-arbs" aria-current={current(nav.section === "signals" && nav.kind === "arb")}>
        Arbitrage<span className={styles.c}>{signalList("arb").length}</span>
      </Link>
      <Link href={routes.signals("ev")} data-tour="notch-value" aria-current={current(nav.section === "signals" && nav.kind === "ev")}>Value bets</Link>
      <Link href={routes.sources()} aria-current={current(nav.section === "sources")}>Sources</Link>
      <button className={styles.anS} aria-label="Search" onClick={open}>
        <SearchIcon />
      </button>
      <label className={styles.anQ} htmlFor="nq">
        <SearchIcon />
        <input id="nq" data-q type="search" placeholder="Search teams, players, leagues or matches" autoComplete="off" value={shell.q} onChange={(e) => search(e.target.value)} />
        <button className={styles.anX} aria-label="Close search" onClick={() => setSearching(false)}>
          <CloseIcon />
        </button>
      </label>
      {searching && <SearchResults q={shell.q} onPick={() => { shell.setQ(""); setSearching(false) }} />}
    </nav>
  )
}
