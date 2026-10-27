"use client"

import type { PropRow as PropRowData } from "@/lib/api/types"
import { marketLabel } from "@/lib/odds/markets"
import { boardBest, type Fixture } from "@/lib/store/data"
import { inSlip, togglePick } from "@/lib/store/prefs"
import { ChevronDownIcon, ChevronUpIcon } from "@/components/mybestbet/shared/icons"
import { BookLink, BookLogo, Crest } from "@/components/mybestbet/shared/logos"
import { useOdds } from "@/components/mybestbet/shared/use-odds"
import { flashDir, useFlashClear } from "./odds-grid"
import { PropPanel } from "./prop-panel"
import styles from "./props.module.css"

/** "Anytime scorer", "Shots 2.5" or "Rating 7+". */
export function propLabel(r: PropRowData, sport: string) {
  const yesNo = r.selections.some((s) => s.side === "yes")
  if (yesNo) return marketLabel(r.key.code, sport)
  const line = r.key.line
  return marketLabel(r.key.code, sport) + " " + (line != null ? String(line) : "")
}

export function PropRow({ f, r, open, onToggle }: { f: Fixture; r: PropRowData; open: boolean; onToggle: () => void }) {
  const F = useOdds()
  const yesNo = r.selections.some((s) => s.side === "yes")
  const team = r.player.teamId === f.aT.id ? f.aT : f.hT
  const hr = r.hitRate || r.research?.windows.find((w) => w.window === 10)
  const res = r.research
  const best = boardBest(r)
  const pp = r.selections[0]?.cells.PrizePicks
  const ppGap = pp && r.key.line != null && pp.price.raw !== r.key.line ? pp.price.raw : null
  const books = Object.keys(r.selections[0] ? r.selections[0].cells : {}).filter((n) => n !== "PrizePicks").length
  const pips = hr ? Array.from({ length: hr.matches }, (_, k) => k < hr.hits) : []
  const flashes: string[] = []

  const cell = (k: number) => {
    const x = best[k], s = r.selections[k]
    if (!s) return null
    const label = yesNo ? (s.side === "yes" ? "Yes" : "No") : s.side === "over" ? "Over" : "Under"
    if (!x.d) return <span className={styles.pb + " " + styles.none + (k ? " " + styles.hs : "")}>–</span>
    const fl = flashDir(r.keyString + "#" + k + "#" + x.src)
    if (fl) flashes.push(fl)
    return (
      <div className={[styles.pb, k ? styles.hs : "", fl ? styles[fl] : ""].filter(Boolean).join(" ")}>
        <button
          className="tap-area"
          data-pick={f.id}
          aria-pressed={inSlip(f.id, r.keyString, k)}
          aria-label={`${r.player.name} ${propLabel(r, f.sp)} ${label} at ${F(x.d)} on ${x.src}`}
          onClick={() => togglePick({ fx: f.id, key: r.keyString, i: k, code: r.key.code, label: s.label })}
        />
        <span className={styles.l}>{label}</span>
        <span className={styles.v} style={k ? undefined : { color: "var(--blue)" }}>{F(x.d)}</span>
        <span className={styles.s}><BookLink name={x.src} f={f} deep={s.cells[x.src]?.deepLink} size={14}><em>{x.src}</em></BookLink></span>
      </div>
    )
  }
  const cells = [cell(0), cell(1)]
  useFlashClear(flashes.length > 0)

  return (
    <div className={styles.pr}>
      <div className={styles.prRow}>
        <button className={styles.prWho} aria-expanded={open} onClick={onToggle}>
          <Crest team={team} size={28} />
          <span className={styles.whoText}>
            <span className={styles.whoName}>{r.player.name}</span>
            <span className={styles.whoMeta}>
              <span>{propLabel(r, f.sp)}</span>
              {res?.lineupStatus === "starter" && <span className={styles.fl + " " + styles.g}>Starting</span>}
              {res?.lineupStatus === "bench" && <span className={styles.fl + " " + styles.a}>Bench</span>}
              {res?.lineupStatus === "out" && <span className={styles.fl + " " + styles.a}>Out</span>}
            </span>
          </span>
        </button>
        <span className={styles.hs + " " + styles.books}>
          {ppGap != null ? (
            <span className={styles.fl + " " + styles.a + " " + styles.pp}><BookLogo name="PrizePicks" size={13} />PrizePicks {ppGap}</span>
          ) : (
            <span className={"faint " + styles.small}>{books} book{books === 1 ? "" : "s"}</span>
          )}
          {res?.benchWarning && <span className={"faint " + styles.small}>Under 60 lately</span>}
        </span>
        <span className={styles.hs}>
          <span className={"mono " + styles.hits}>{hr ? hr.hits + "/" + hr.matches : "–"}</span>
          <span className={styles.pip} aria-hidden="true">{pips.map((h, k) => <i key={k} className={h ? styles.on : ""} />)}</span>
        </span>
        {cells}
        <button className={styles.prEx} aria-label={(open ? "Hide" : "Show") + " research"} onClick={onToggle}>
          {open ? <ChevronUpIcon /> : <ChevronDownIcon />}
        </button>
      </div>
      {open && <PropPanel f={f} r={r} />}
    </div>
  )
}
