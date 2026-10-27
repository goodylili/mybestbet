"use client"

import { useEffect, useRef, useState } from "react"
import { MybestbetWordmark } from "@/components/mybestbet-wordmark"
import styles from "./welcome-gate.module.css"

type Key = "a" | "b"

const BUG: Record<Key, [string, string, string]> = {
  a: ["AGE", "I'm 18 or over", "Not run by, or linked to, any book, league or team"],
  b: ["RISK", "Betting can lose money", "Nothing here is advice. Prices are for learning"],
}

const mmss = (ms: number) => {
  const t = Math.max(0, Math.floor(ms / 1000))
  return String(Math.floor(t / 60)).padStart(2, "0") + ":" + String(t % 60).padStart(2, "0")
}

function Tick() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path d="m3.5 8.5 3 3 6-7" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function Arrow() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path d="M3 8h10M9 4l4 4-4 4" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

/** A score that pops whenever it changes after the first render. */
function Score({ value }: { value: number }) {
  const [last, setLast] = useState(value)
  const [changes, setChanges] = useState(0)
  if (last !== value) {
    setLast(value)
    setChanges(changes + 1)
  }
  return <b key={changes} className={changes ? styles.pop : undefined}>{value}</b>
}

/**
 * The "Before kick-off" gate. Ticking both strips scores the viewer 1-0 then
 * 1-1, moves the clock from PRE to HT to FT and unlocks KICK OFF.
 */
export function WelcomeGate({ closing, reduced, onEnter, onLeave }: {
  closing: boolean
  reduced: boolean
  onEnter: () => void
  onLeave: () => void
}) {
  const [shown, setShown] = useState(false)
  const [ok, setOk] = useState<Record<Key, boolean>>({ a: false, b: false })
  const [at, setAt] = useState<Record<Key, number>>({ a: 0, b: 0 })
  const [going, setGoing] = useState(false)
  const [t0] = useState(() => Date.now())
  const [now, setNow] = useState(t0)
  const root = useRef<HTMLDivElement>(null)
  const both = ok.a && ok.b
  const count = (ok.a ? 1 : 0) + (ok.b ? 1 : 0)

  useEffect(() => {
    const id = requestAnimationFrame(() => {
      setShown(true)
      root.current?.focus({ preventScroll: true })
    })
    return () => cancelAnimationFrame(id)
  }, [])

  useEffect(() => {
    if (both) return
    const id = setInterval(() => setNow(Date.now()), 500)
    return () => clearInterval(id)
  }, [both])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Tab" || !root.current) return
      const focusable = [...root.current.querySelectorAll<HTMLButtonElement>("button:not([disabled])")]
      if (!focusable.length) return
      const i = focusable.indexOf(document.activeElement as HTMLButtonElement)
      if (e.shiftKey && i <= 0) {
        e.preventDefault()
        focusable[focusable.length - 1].focus()
      } else if (!e.shiftKey && i === focusable.length - 1) {
        e.preventDefault()
        focusable[0].focus()
      }
    }
    window.addEventListener("keydown", onKey, true)
    return () => window.removeEventListener("keydown", onKey, true)
  }, [])

  const toggle = (k: Key) => {
    setOk((x) => ({ ...x, [k]: !x[k] }))
    setAt((x) => ({ ...x, [k]: Date.now() }))
  }

  const enter = () => {
    if (!both) return
    setGoing(true)
    onEnter()
  }

  const bug = (k: Key, i: number) => (
    <button className={styles.bgRow} role="switch" aria-checked={ok[k]} style={{ "--d": 180 + i * 140 + "ms" } as React.CSSProperties} onClick={() => toggle(k)}>
      <span className={styles.bgStrip}>
        <span className={styles.bgAbbr}>{BUG[k][0]}</span>
        <span className={styles.bgMark}>
          <span className={styles.ogC}><Tick /></span>
        </span>
        <span className={styles.bgPlate}>
          <span className={styles.bgTxt}>
            <b>{BUG[k][1]}</b>
            <small>{BUG[k][2]}</small>
          </span>
          <span className={styles.bgHit} aria-hidden="true">CONFIRMED</span>
        </span>
      </span>
      <span className={styles.bgTab + " mono"}>{ok[k] ? "✓ " + mmss(at[k] - t0) : "TAP"}</span>
    </button>
  )

  return (
    <div className={styles.og + " " + styles.ogOBg + (reduced ? " " + styles.obRm : "") + (shown && !closing ? " " + styles.in : "")}>
      <div
        ref={root}
        className={styles.ogRoot + " " + styles.bg + (both ? " " + styles.ready : "") + (going ? " " + styles.going : "")}
        role="dialog"
        aria-modal="true"
        aria-labelledby="og-h"
        tabIndex={-1}
      >
        <div className={styles.bgTop}>
          <MybestbetWordmark className={styles.bgWm} />
          <span className={styles.bgLive}><i />LIVE</span>
        </div>
        <h2 id="og-h" className={styles.bgH}>Before kick-off</h2>
        {bug("a", 0)}
        {bug("b", 1)}
        <div className={styles.bgRow + " " + styles.bgMain} style={{ "--d": "520ms" } as React.CSSProperties}>
          <span className={styles.bgStrip}>
            <span className={styles.bgAbbr}>MBB</span>
            <span className={styles.bgScore + " mono"}>
              <Score value={ok.a ? 1 : 0} />
              <i>-</i>
              <Score value={ok.b ? 1 : 0} />
            </span>
            <span className={styles.bgAbbr + " " + styles.bgYou}>YOU</span>
            <button className={styles.bgGo} disabled={!both} onClick={enter}>
              <span>KICK OFF</span>
              <Arrow />
            </button>
          </span>
          <span className={styles.bgTab + " mono"}>
            <span>{mmss(now - t0)}</span>
            <em>{both ? "FT" : count ? "HT" : "PRE"}</em>
          </span>
        </div>
        <p className={styles.bgFoot}>
          <button className={styles.ogBack} onClick={onLeave}>Back to the homepage</button>
          <span>Educational demo · 18+ · not a bookmaker</span>
        </p>
      </div>
    </div>
  )
}
