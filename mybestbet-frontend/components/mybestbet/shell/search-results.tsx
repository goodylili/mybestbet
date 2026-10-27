"use client"

import Link from "next/link"
import { useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react"
import type { List, SearchItem } from "@/lib/api/types"
import { api, apiUrl, sampleData } from "@/lib/api/client"
import { sportName } from "@/lib/odds/markets"
import { dayShort, hhmm } from "@/lib/odds/time"
import { routes } from "@/lib/routes"
import { leagueName } from "@/lib/store/data"
import { useNav } from "./use-nav"
import styles from "./search-results.module.css"

const KIND_LABEL: Record<string, string> = { event: "Matches", team: "Teams", league: "Leagues", player: "Players" }
const KIND_ORDER = ["event", "team", "league", "player"]

type Found = { q: string; items: SearchItem[]; error?: boolean }

function hrefOf(item: SearchItem) {
  if (item.kind === "event") return routes.event(item.id)
  if (item.kind === "league") return routes.league(item.leagueCode || item.id)
  if (item.kind === "player") return routes.player(item.id)
  return routes.sport(item.sport)
}

function Crest({ url, className }: { url?: string; className?: string }) {
  return url ? <img className={className} src={apiUrl(url)} alt="" loading="lazy" /> : <span className={styles.ph + (className ? " " + className : "")} aria-hidden="true" />
}

function Mark({ item }: { item: SearchItem }) {
  if (item.kind === "event" && (item.logoUrl || item.awayLogoUrl)) {
    return (
      <span className={styles.pair} aria-hidden="true">
        <Crest url={item.awayLogoUrl} className={styles.away} />
        <Crest url={item.logoUrl} className={styles.home} />
      </span>
    )
  }
  return <Crest url={item.logoUrl} />
}

function detailOf(item: SearchItem) {
  const league = item.leagueCode ? leagueName(item.leagueCode) : ""
  if (item.kind === "event" && item.kickoff) {
    const ko = Date.parse(item.kickoff)
    return [league, dayShort(ko) + " " + hhmm(ko)].filter(Boolean).join(" · ")
  }
  if (item.kind === "league") return sportName(item.sport)
  return league || sportName(item.sport)
}

export function SearchResults({ q, onPick }: { q: string; onPick: (item: SearchItem) => void }) {
  const query = q.trim()
  const { sport } = useNav()
  const [found, setFound] = useState<Found | null>(null)
  const panel = useRef<HTMLDivElement>(null)
  const origin = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    if (query.length < 2 || sampleData()) return
    let live = true
    const id = setTimeout(() => {
      api<List<SearchItem>>(`/v1/search?q=${encodeURIComponent(query)}&limit=24${sport ? "&prefer=" + encodeURIComponent(sport) : ""}`)
        .then((j) => { if (live) setFound({ q: query, items: j.items }) })
        .catch(() => { if (live) setFound({ q: query, items: [], error: true }) })
    }, 220)
    return () => { live = false; clearTimeout(id) }
  }, [query, sport])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const input = document.activeElement
      if (e.key !== "ArrowDown" || !(input instanceof HTMLInputElement) || !input.hasAttribute("data-q")) return
      const first = panel.current?.querySelector<HTMLElement>("[role=option]")
      if (!first || !first.offsetParent) return
      e.preventDefault()
      origin.current = input
      first.focus()
    }
    document.addEventListener("keydown", onKey)
    return () => document.removeEventListener("keydown", onKey)
  }, [])

  const move = (e: ReactKeyboardEvent<HTMLDivElement>) => {
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return
    const options = [...(panel.current?.querySelectorAll<HTMLElement>("[role=option]") || [])]
    const at = options.indexOf(document.activeElement as HTMLElement)
    if (at < 0) return
    e.preventDefault()
    const next = at + (e.key === "ArrowDown" ? 1 : -1)
    if (next >= 0) options[Math.min(next, options.length - 1)].focus()
    else origin.current?.focus()
  }

  if (query.length < 2 || sampleData()) return null
  const current = found && found.q === query ? found : null
  const groups = KIND_ORDER.map((kind) => [kind, (current?.items || []).filter((i) => i.kind === kind).slice(0, 6)] as const).filter(([, items]) => items.length)

  return (
    <div ref={panel} className={styles.panel} role="listbox" aria-label="Search results" onKeyDown={move}>
      {!current ? (
        <p className={styles.note}>Searching</p>
      ) : current.error ? (
        <p className={styles.note}>Search is unavailable right now</p>
      ) : !groups.length ? (
        <p className={styles.note}>Nothing matches “{query}”</p>
      ) : (
        groups.map(([kind, items]) => (
          <div key={kind} className={styles.group}>
            <h3>{KIND_LABEL[kind]}</h3>
            {items.map((item) => (
              <Link key={kind + item.id} href={hrefOf(item)} className={styles.item} role="option" aria-selected={false} onClick={() => onPick(item)}>
                <Mark item={item} />
                <span className={styles.tx}>
                  <b>{item.name}</b>
                  <i>{detailOf(item)}</i>
                </span>
              </Link>
            ))}
          </div>
        ))
      )}
    </div>
  )
}
