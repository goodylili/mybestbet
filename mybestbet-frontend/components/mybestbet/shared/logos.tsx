"use client"

import { useState, type MouseEvent, type ReactNode } from "react"
import type { Team } from "@/lib/api/types"
import { apiUrl } from "@/lib/api/client"
import { bookArt, compArt, teamArt, teamTag } from "@/lib/catalog"
import { S, bookHref, sourceInfo, type Fixture } from "@/lib/store/data"

const nameOf = (team: Team | string) => (typeof team === "string" ? team : team ? team.name : "")

/** A coloured three-letter tag, used when a team has no crest. */
export function TeamTag({ name, size }: { name: string; size: number }) {
  const [abbr, color] = teamTag(name)
  return (
    <span
      className="abbr"
      style={{ width: size, height: size, fontSize: Math.max(7.5, size * 0.3), background: color + "26", color, boxShadow: `inset 0 0 0 1px ${color}55` }}
    >
      {abbr.slice(0, 3)}
    </span>
  )
}

/** Team crest from the bundled set, then the API's logo, then a tag. */
export function Crest({ team, size = 22 }: { team: Team | string; size?: number }) {
  const name = nameOf(team)
  const [failed, setFailed] = useState(false)
  const art = teamArt(name)
  if (art) {
    return art.flag ? (
      <img className="flag" src={art.src} alt="" width={size} height={Math.round(size * 0.75)} style={{ margin: `${Math.round(size * 0.125)}px 0` }} />
    ) : (
      <img className={"cr" + (art.light ? " lt" : "")} src={art.src} alt="" width={size} height={size} />
    )
  }
  const known = typeof team !== "string" && team?.logoUrl ? team : S.tname[name]
  if (known?.logoUrl && !failed) {
    return <img className="cr" src={apiUrl(known.logoUrl)} alt="" width={size} height={size} loading="lazy" onError={() => setFailed(true)} />
  }
  return <TeamTag name={name} size={size} />
}

/** Bookmaker or prediction market logo. Renders nothing when there is none. */
export function BookLogo({ name, size = 16 }: { name: string; size?: number }) {
  const [failed, setFailed] = useState(false)
  const art = bookArt(name)
  if (art) return <img className={"sl" + (art.light ? " lt" : "")} src={art.src} alt="" width={size} height={size} />
  const info = sourceInfo(name)
  if (info?.logoUrl && !failed) return <img className="sl" src={apiUrl(info.logoUrl)} alt="" width={size} height={size} loading="lazy" onError={() => setFailed(true)} />
  return null
}

const stopClick = (e: MouseEvent) => e.stopPropagation()

export function BookLink({ name, f, deep, size = 16, className, children }: {
  name: string
  f?: Fixture
  deep?: string
  size?: number
  className?: string
  children?: ReactNode
}) {
  const href = bookHref(f, name, deep)
  const logo = <BookLogo name={name} size={size} />
  if (!href) return children ? <>{logo}{children}</> : logo
  const label = "Open " + name + (f ? ": " + f.h + " v " + f.a : "")
  return (
    <a className={"bklink" + (children ? " named" : "") + (className ? " " + className : "")} href={href} target="_blank" rel="noopener noreferrer" aria-label={label} title={label} onClick={stopClick}>
      {logo}
      {children}
    </a>
  )
}

/** Competition mark: league logo, federation symbol or country flag. */
export function LeagueMark({ code, size = 18 }: { code: string; size?: number }) {
  const art = compArt(code)
  if (art) {
    return art.flag ? (
      <img className="flag" src={art.src} alt="" width={size} height={Math.round(size * 0.75)} />
    ) : (
      <img className={"sl" + (art.symbol ? " si" : "") + (art.light ? " lt" : "")} src={art.src} alt="" width={size} height={size} />
    )
  }
  const league = S.lg[code]
  if (league?.logoUrl) return <img className="sl" src={apiUrl(league.logoUrl)} alt="" width={size} height={size} loading="lazy" />
  return null
}

export const hasLeagueMark = (code: string) => !!compArt(code) || !!S.lg[code]?.logoUrl
