"use client"

import Link from "next/link"
import { Fragment, useState, type ReactNode } from "react"
import type { League } from "@/lib/api/types"
import { LeagueMark, hasLeagueMark } from "@/components/mybestbet/shared/logos"
import { PulseIcon, ScaleIcon, SIGNAL_ICONS, SportIcon, StarIcon } from "@/components/mybestbet/shared/icons"
import { compMeta, compOrder } from "@/lib/catalog"
import { S, dayFixtures, signalList, sportCount, type Fixture } from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { prefs } from "@/lib/store/prefs"
import { SIGNALS, SIGNAL_TYPES, SPORTS, sportName } from "@/lib/odds/markets"
import { routes } from "@/lib/routes"
import { useShell } from "./shell-context"
import { useNav } from "./use-nav"
import styles from "./sidebar.module.css"

const MORE = "More leagues"

/** Three letters for a league with no logo, e.g. "Saudi Pro League" → "SPL". */
const leagueAbbr = (l: League) => {
  const words = String(l.name || l.code).replace(/[^\p{L}\p{N} ]/gu, "").split(/\s+/).filter(Boolean)
  return (words.length > 1 ? words.map((w) => w[0]).join("") : words[0] || l.code).slice(0, 3).toUpperCase()
}

function NavItem({ href, current, on, icon, name, count, green, onClick }: {
  href: string
  current: boolean
  on?: boolean
  icon: ReactNode
  name: string
  count: ReactNode
  green?: boolean
  onClick?: () => void
}) {
  return (
    <Link
      className={styles.nv + (on ? " " + styles.on : "")}
      href={href}
      aria-current={current ? "page" : undefined}
      aria-label={name}
      onClick={onClick}
    >
      {icon}
      <span className={styles.nm}>{name}</span>
      <span className={styles.n + (green ? " " + styles.g : "")}>{count}</span>
    </Link>
  )
}

/** The browse rail: sports, favourites, the sport's competitions and the signal tools. Unrolls on hover or focus. */
export function Sidebar() {
  useStore()
  const nav = useNav()
  const shell = useShell()
  const [more, setMore] = useState(false)
  const [shut, setShut] = useState(false)
  const fixtures = dayFixtures(shell.date)
  const perLeague: Record<string, number> = {}
  fixtures.forEach((f) => { perLeague[f.lg] = (perLeague[f.lg] || 0) + 1 })

  const sport = nav.sport
  const summary = S.sports.find((s) => s.sport === sport)
  const leagues = summary ? summary.leagues.slice() : Object.values(S.lg).filter((l) => l.sport === sport)
  leagues.sort((a, b) => compOrder(a.code) - compOrder(b.code) || a.name.localeCompare(b.name))
  const groups: [string, League[]][] = []
  leagues.forEach((l) => {
    const g = compMeta(l.code)?.grp || MORE
    let entry = groups.find((x) => x[0] === g)
    if (!entry) groups.push((entry = [g, []]))
    entry[1].push(l)
  })

  /** Open a list on the first of the next three days that has something in it, as picking a sport or league does. */
  const landOn = (match: (f: Fixture) => boolean) => () => {
    let date = shell.date
    if (!dayFixtures(shell.date).some(match)) {
      for (let d = 0; d < 3; d++) if (dayFixtures(d).some(match)) { date = d; break }
    }
    shell.setFilter("all")
    shell.setDate(date)
  }

  const sports = S.sports.length ? S.sports.map((s) => s.sport) : SPORTS.map((s) => s.id)
  const healthy = S.sources.filter((s) => !s.down).length

  return (
    <aside
      className={styles.side + (shut ? " " + styles.shut : "")}
      aria-label="Browse"
      onClickCapture={(e) => {
        const link = (e.target as HTMLElement).closest("a")
        if (!link) return
        setShut(true)
        link.blur()
      }}
      onMouseLeave={() => setShut(false)}
      onFocus={() => setShut(false)}
    >
      <div className={styles.sports}>
        {sports.map((id) => (
          <NavItem
            key={id}
            href={routes.sport(id)}
            on={sport === id}
            current={sport === id && nav.section === "list" && !nav.league}
            icon={<SportIcon sport={id} />}
            name={sportName(id)}
            count={sportCount(id, shell.date) || ""}
            onClick={landOn((f) => f.sp === id)}
          />
        ))}
      </div>
      <NavItem
        href={routes.favourites()}
        current={nav.league === "STAR"}
        icon={<StarIcon size={18} />}
        name="Favourites"
        count={Object.keys(prefs.starred).filter((k) => prefs.starred[k]).length}
        onClick={landOn((f) => !!prefs.starred[f.id])}
      />
      {groups.map(([group, list]) => {
        const collapsed = group === MORE && !more && !list.some((c) => c.code === nav.league)
        return (
          <Fragment key={group}>
            <h3>
              <span className={styles.nm}>{group}</span>
              {group === MORE && (
                <button className={styles.tgl} onClick={() => setMore(!more)}>{more ? "Hide" : "Show"}</button>
              )}
            </h3>
            {!collapsed && list.map((c) => (
              <NavItem
                key={c.code}
                href={routes.league(c.code)}
                current={nav.league === c.code}
                icon={hasLeagueMark(c.code) ? <LeagueMark code={c.code} size={18} /> : <span className={styles.lgab} aria-hidden="true">{leagueAbbr(c)}</span>}
                name={c.name}
                count={perLeague[c.code] || ""}
                onClick={landOn((f) => f.lg === c.code)}
              />
            ))}
          </Fragment>
        )
      })}
      <h3><span className={styles.nm}>Tools</span></h3>
      {SIGNAL_TYPES.map((k) => {
        const Icon = SIGNAL_ICONS[k]
        return (
          <NavItem
            key={k}
            href={routes.signals(k)}
            current={nav.section === "signals" && nav.kind === k}
            icon={<Icon />}
            name={SIGNALS[k].title}
            count={signalList(k).length || ""}
            green
          />
        )
      })}
      <NavItem
        href={routes.sources()}
        current={nav.section === "sources"}
        icon={<PulseIcon />}
        name="Sources"
        count={S.sources.length ? healthy + "/" + S.sources.length : ""}
      />
      <NavItem
        href={routes.baysed()}
        current={nav.section === "baysed"}
        icon={<ScaleIcon />}
        name="Baysed"
        count=""
      />
      <p className={"edu " + styles.edu}>Educational demo. Not affiliated with or licensed by any book, league or team shown. Names and logos belong to their owners.</p>
    </aside>
  )
}
