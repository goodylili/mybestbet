"use client"

import { MONTHS, dateFor, dayLong } from "@/lib/odds/time"
import { ensureDays } from "@/lib/store/data"
import styles from "./calendar.module.css"

const WEEK = ["M", "T", "W", "T", "F", "S", "S"]

export function Calendar({ date, label, count, horizon, onPick }: {
  date: number
  label: string
  count: (d: number) => number | null
  horizon: number
  onPick: (d: number) => void
}) {
  if (count(horizon - 1) == null) ensureDays(0, horizon)
  const startOff = -((dateFor(0).getDay() + 6) % 7)
  const lastOff = horizon - 1
  const end = lastOff + (7 - ((dateFor(lastOff).getDay() + 6) % 7) - 1)
  const cells: number[] = []
  for (let d = startOff; d <= end; d++) cells.push(d)

  return (
    <div className={styles.cal} role="dialog" aria-label="Pick a day">
      <div className={styles.head}>
        <b>Pick a day</b>
        <span className="faint">{label}</span>
      </div>
      <div className={styles.grid}>
        {WEEK.map((x, k) => (
          <span key={"w" + k} className={styles.wd}>{x}</span>
        ))}
        {cells.map((d) => {
          const x = dateFor(d)
          const month = x.getDate() === 1 || d === startOff ? <i>{MONTHS[x.getMonth()]}</i> : null
          if (d < 0 || d >= horizon) return <span key={d} className={styles.dd + " " + styles.off}>{x.getDate()}</span>
          const n = count(d)
          return (
            <button
              key={d}
              className={styles.dd + (d === date ? " " + styles.sel : "") + (n === 0 ? " " + styles.zero : n ? " " + styles.has : "")}
              aria-label={dayLong(d) + (n != null ? ", " + n + " matches" : "")}
              onClick={() => onPick(d)}
            >
              {month}
              {x.getDate()}
              <small>{n || ""}</small>
            </button>
          )
        })}
      </div>
    </div>
  )
}
