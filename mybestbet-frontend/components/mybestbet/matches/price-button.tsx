"use client"

import { useEffect, useReducer } from "react"
import { BookLink } from "@/components/mybestbet/shared/logos"
import { useOdds } from "@/components/mybestbet/shared/use-odds"
import { mainMarket } from "@/lib/odds/markets"
import { msSince } from "@/lib/odds/time"
import { S, type Fixture } from "@/lib/store/data"
import { inSlip, togglePick } from "@/lib/store/prefs"
import styles from "./match-row.module.css"

const FLASH_MS = 1500

/** "up" or "dn" for a price that moved in the last 1.5s, re-rendering once the window closes. */
export function useFlash(key: string) {
  const [, tick] = useReducer((n: number) => n + 1, 0)
  const x = S.flash[key]
  const age = x ? msSince(x.at) : Infinity
  const live = age < FLASH_MS
  useEffect(() => {
    if (!live) return
    const id = setTimeout(tick, FLASH_MS - age + 20)
    return () => clearTimeout(id)
  }, [live, age, x?.at])
  return live && x ? (x.dir > 0 ? "up" : "dn") : ""
}

export type RowSel = { label: string; short: string }
export type RowPrice = { d: number | null; src: string; deep?: string } | null | undefined

/** One price on a match list row: tap to add it to the slip. */
export function PriceButton({ f, sel, i, x, marketKey, code, pending, pinned }: {
  f: Fixture
  sel: RowSel
  i: number
  x: RowPrice
  marketKey: string
  code: string
  pending: boolean
  tour?: boolean
  pinned?: boolean
}) {
  const odds = useOdds()
  const flash = useFlash(x && x.d ? (code === mainMarket(f.sp) ? f.id + "#list#" + i : marketKey + "#" + i + "#" + x.src) : "")
  if (!x || !x.d) return <span className={styles.pb + " " + styles.none}>{pending ? "…" : "–"}</span>
  return (
    <div className={styles.pb + (flash ? " " + styles[flash] : "")} data-tour="best-price">
      <button
        className="tap-area"
        aria-pressed={inSlip(f.id, marketKey, i)}
        aria-label={`${sel.label} at ${odds(x.d)} on ${x.src}`}
        onClick={() => togglePick({ fx: f.id, key: marketKey, i, code, label: sel.label, src: pinned ? x.src : undefined })}
      />
      <span className={styles.l}>{sel.short}</span>
      <span className={styles.v}>{odds(x.d)}</span>
      <span className={styles.s}>
        <BookLink name={x.src} f={f} deep={x.deep} size={14}><em>{x.src}</em></BookLink>
      </span>
    </div>
  )
}
