"use client"

import Link from "next/link"
import { useEffect, useRef, useState } from "react"
import { MybestbetWordmark } from "@/components/mybestbet-wordmark"
import { SearchIcon, TicketIcon } from "@/components/mybestbet/shared/icons"
import { SNAPSHOT } from "@/lib/api/client"
import { S } from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { prefs } from "@/lib/store/prefs"
import { routes } from "@/lib/routes"
import { Notch } from "./notch"
import { useShell } from "./shell-context"
import { useSearchInput } from "./use-search-input"
import styles from "./top-bar.module.css"

const CONNECTION = { live: "Live", reconnecting: "Reconnecting", offline: "Offline", connecting: "Connecting" } as const

function SettingsIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </svg>
  )
}

/** "Sample data", "Live", "Reconnecting", "Offline" or "Connecting", with an amber dot when not live. */
function Connection() {
  useStore()
  const live = S.conn === "live"
  return (
    <span className={styles.live + (live ? "" : " " + styles.off)}>
      <span className="dot" />
      <span>{live && SNAPSHOT ? "Sample data" : CONNECTION[S.conn]}</span>
    </span>
  )
}

export function TopBar() {
  useStore()
  const shell = useShell()
  const search = useSearchInput()
  const [searching, setSearching] = useState(false)
  const notch = useRef<HTMLElement>(null)
  const query = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setSearching(false)
      if (e.key !== "/" || document.activeElement?.tagName === "INPUT" || document.body.classList.contains("ob-open")) return
      const q = query.current
      const list = [...document.querySelectorAll<HTMLInputElement>("input[data-q]")].find((x) => x !== q && !notch.current?.contains(x) && x.offsetParent)
      if (q && q.offsetParent) {
        e.preventDefault()
        q.focus()
      } else if (notch.current && notch.current.offsetParent) {
        e.preventDefault()
        setSearching(true)
        setTimeout(() => notch.current?.querySelector("input")?.focus(), 30)
      } else if (list) {
        e.preventDefault()
        list.focus()
      }
    }
    document.addEventListener("keydown", onKey)
    return () => document.removeEventListener("keydown", onKey)
  }, [])

  return (
    <header className={styles.top}>
      <div className={styles.topL}>
        <Link className={styles.brand} href={routes.home()} aria-label="mybestbet home">
          <MybestbetWordmark className={styles.wm} />
        </Link>
      </div>
      <Notch ref={notch} searching={searching} setSearching={setSearching} />
      <label className={styles.search} htmlFor="q">
        <SearchIcon />
        <span className="sr">Search</span>
        <input ref={query} id="q" data-q type="search" placeholder="Search teams, players or leagues" autoComplete="off" value={shell.q} onChange={(e) => search(e.target.value)} />
        <span className="kbd">/</span>
      </label>
      <div className={styles.topR}>
        <Connection />
        <button className={"ib " + styles.setb} data-tour="settings" aria-label="Settings" title="Settings" onClick={shell.openSheet}>
          <SettingsIcon />
        </button>
        <button className={styles.slipBtn} aria-label="Bet slip" onClick={shell.openSlip}>
          <TicketIcon />
          <span className={"mono" + (prefs.slip.length ? " " + styles.on : "")}>{prefs.slip.length}</span>
        </button>
      </div>
    </header>
  )
}
