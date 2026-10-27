"use client"

import { useEffect, useRef } from "react"
import { CalendarIcon } from "@/components/mybestbet/shared/icons"
import type { ListFilter } from "@/components/mybestbet/shell/shell-context"
import { dayTab } from "@/lib/odds/time"
import { Calendar } from "./calendar"
import styles from "./day-bar.module.css"

/** Line picker, day switcher (next 3h, today, tomorrow, calendar) and the arb and value filters. */
export function DayBar({ date, filter, lines, wantLine, calOpen, calLabel, count, horizon, onLine, onDate, onFilter, onCal }: {
  date: number
  filter: ListFilter
  lines: number[]
  wantLine: number | undefined
  calOpen: boolean
  calLabel: string
  count: (d: number) => number | null
  horizon: number
  onLine: (line: number) => void
  onDate: (d: number) => void
  onFilter: (f: ListFilter) => void
  onCal: (open: boolean) => void
}) {
  const ref = useRef<HTMLDivElement>(null)
  const dayN = (d: number) => count(d) ?? ""

  useEffect(() => {
    if (!calOpen) return
    const click = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) onCal(false)
    }
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") onCal(false)
    }
    document.addEventListener("click", click)
    document.addEventListener("keydown", key)
    return () => {
      document.removeEventListener("click", click)
      document.removeEventListener("keydown", key)
    }
  }, [calOpen, onCal])

  return (
    <div ref={ref} className={styles.fbar} data-tour="filters">
      {lines.length > 1 ? (
        <>
          <div className="seg" role="group" aria-label="Line">
            {lines.map((l) => (
              <button
                key={l}
                aria-pressed={wantLine === l || (wantLine == null && l === lines[Math.floor(lines.length / 2)])}
                onClick={() => onLine(l)}
              >
                {l}
              </button>
            ))}
          </div>
          <span className={styles.fsp} />
        </>
      ) : null}
      <div className={"seg " + styles.useg} role="group" aria-label="When and what">
        <button aria-pressed={filter === "soon"} onClick={() => onFilter("soon")}>Next 3h</button>
        {[0, 1].map((d) => (
          <button key={d} className={count(d) === 0 ? styles.none : ""} aria-pressed={date === d && filter !== "soon"} onClick={() => onDate(d)}>
            {d ? "Tomorrow" : "Today"} <span className="mono faint">{dayN(d)}</span>
          </button>
        ))}
        {date > 1 ? (
          <button aria-pressed={filter !== "soon"} onClick={() => onCal(!calOpen)}>
            {dayTab(date)} <span className="mono faint">{dayN(date)}</span>
          </button>
        ) : null}
        <button className={styles.calb} aria-expanded={calOpen} aria-label="Pick a day" onClick={() => onCal(!calOpen)}>
          <CalendarIcon />
        </button>
        <i className={styles.usep} aria-hidden="true" />
        <button aria-pressed={filter === "arb"} onClick={() => onFilter("arb")}>Arbs</button>
        <button aria-pressed={filter === "ev"} onClick={() => onFilter("ev")}>Value</button>
      </div>
      {calOpen ? <Calendar date={date} label={calLabel} count={count} horizon={horizon} onPick={onDate} /> : null}
    </div>
  )
}
