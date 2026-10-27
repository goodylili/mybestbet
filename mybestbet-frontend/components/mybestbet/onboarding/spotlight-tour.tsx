"use client"

import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react"
import { BookLogo } from "@/components/mybestbet/shared/logos"
import { S, signalList } from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { prefs, setFormat, setTheme, toggleSource } from "@/lib/store/prefs"
import type { OddsFormat } from "@/lib/odds/format"
import styles from "./spotlight-tour.module.css"

/**
 * Elements the tour points at, tried in order until one is on screen. The
 * screens that own them mark them with these data-tour attributes.
 */
export type Stop = { sel: string[]; title: string; text: () => string; extra?: "books" | "prefs"; before?: () => void }

const STOPS: Stop[] = [
  {
    sel: ['[data-tour="best-price"]'],
    title: "The best price is already picked",
    text: () => "Every price tile shows the top number across all your books, with the book underneath. Tap one to drop it in your slip.",
  },
  {
    sel: ['[data-tour="notch-arbs"]', '[data-tour="tab-arbs"]'],
    title: "Arbs live here",
    text: () => {
      const n = signalList("arb").length
      return (n ? n + " open right now. " : "") + "Each one backs every outcome at a different book, and the calculator splits your stake so you profit whatever happens."
    },
  },
  {
    sel: ['[data-tour="notch-value"]', '[data-tour="tab-value"]'],
    title: "Value bets, line gaps and steam",
    text: () => "When one book pays more than the fair price, or the line jumps, it lands here with the edge already worked out.",
  },
  {
    sel: ['[data-tour="books"]'],
    title: "Your books, your rules",
    text: () => "Switch off books you can't use and everything recalculates without them.",
    extra: "books",
  },
  {
    sel: ['[data-tour="settings"]'],
    title: "Set it up your way",
    text: () => "Odds format and theme are here any time. Try them now.",
    extra: "prefs",
  },
]

const PREF_FORMATS: [OddsFormat, string][] = [["dec", "1.95"], ["us", "−105"], ["pct", "51%"]]

function target(stop: Stop) {
  for (const sel of stop.sel) {
    const e = document.querySelector<HTMLElement>(sel)
    if (e && e.offsetParent !== null) {
      const r = e.getBoundingClientRect()
      if (r.width && r.height) return e
    }
  }
  return null
}

const pad2 = (n: number) => String(n).padStart(2, "0")

/** Spotlight on one part of the screen at a time, with a card explaining it. */
export function SpotlightTour({ closing, reduced, onEnd, stops = STOPS, label = "Tour", tip = true }: {
  closing: boolean
  reduced: boolean
  onEnd: () => void
  stops?: Stop[]
  label?: string
  tip?: boolean
}) {
  useStore()
  const [idx, setIdx] = useState(0)
  const [nonce, setNonce] = useState(0)
  const [shown, setShown] = useState(false)
  const spot = useRef<HTMLDivElement>(null)
  const card = useRef<HTMLDivElement>(null)
  const index = useRef(0)
  const last = idx === stops.length - 1

  const show = useCallback((i: number) => {
    stops[i]?.before?.()
    index.current = i
    setIdx(i)
    setNonce((n) => n + 1)
  }, [stops])

  const place = useCallback(() => {
    const s = spot.current, c = card.current
    if (!s || !c) return
    const e = target(stops[index.current]), pad = 6, vw = innerWidth, vh = innerHeight
    const mobile = vw < 761
    if (!e) {
      s.style.opacity = "0"
      c.classList.add(styles.tcDock)
      c.style.left = c.style.top = ""
      return
    }
    const r = e.getBoundingClientRect()
    s.style.opacity = "1"
    s.style.left = r.left - pad + "px"
    s.style.top = r.top - pad + "px"
    s.style.width = r.width + pad * 2 + "px"
    s.style.height = r.height + pad * 2 + "px"
    if (mobile) {
      c.classList.add(styles.tcDock)
      c.style.left = c.style.top = ""
      c.classList.toggle(styles.tcUp, r.top > vh * 0.55)
      return
    }
    c.classList.remove(styles.tcDock, styles.tcUp)
    const cw = c.offsetWidth, ch = c.offsetHeight, gap = 16
    const below = r.bottom + pad + gap, above = r.top - pad - gap - ch
    const top = below + ch < vh - 12 ? below : above > 12 ? above : Math.max(12, Math.min(vh - ch - 12, r.top))
    let left = Math.max(12, Math.min(vw - cw - 12, r.left + r.width / 2 - cw / 2))
    if (top !== below && top !== above) left = r.right + gap + cw < vw ? r.right + gap : Math.max(12, r.left - gap - cw)
    c.style.left = left + "px"
    c.style.top = top + "px"
    c.dataset.side = top === below ? "b" : top === above ? "a" : "s"
  }, [stops])

  useEffect(() => {
    const id = requestAnimationFrame(() => setShown(true))
    let raf = 0
    const onMove = () => {
      cancelAnimationFrame(raf)
      raf = requestAnimationFrame(place)
    }
    addEventListener("resize", onMove)
    addEventListener("scroll", onMove, true)
    return () => {
      cancelAnimationFrame(id)
      cancelAnimationFrame(raf)
      removeEventListener("resize", onMove)
      removeEventListener("scroll", onMove, true)
    }
  }, [place])

  useLayoutEffect(() => {
    const e = target(stops[idx])
    if (e) {
      const r = e.getBoundingClientRect()
      if (r.top < 80 || r.bottom > innerHeight - 80) e.scrollIntoView({ block: "center", behavior: reduced ? "auto" : "smooth" })
    }
    const c = card.current
    c?.classList.remove(styles.tcIn)
    const raf = requestAnimationFrame(() => {
      place()
      c?.classList.add(styles.tcIn)
      c?.querySelector<HTMLButtonElement>("." + styles.tcNext)?.focus({ preventScroll: true })
    })
    const late = setTimeout(place, 420)
    let tries = 0
    const wait = e ? 0 : window.setInterval(() => {
      const found = target(stops[idx])
      if (found || ++tries > 10) {
        clearInterval(wait)
        if (found) {
          const r = found.getBoundingClientRect()
          if (r.top < 80 || r.bottom > innerHeight - 80) found.scrollIntoView({ block: "center", behavior: reduced ? "auto" : "smooth" })
          place()
          setTimeout(place, 420)
        }
      }
    }, 300)
    return () => {
      cancelAnimationFrame(raf)
      clearTimeout(late)
      clearInterval(wait)
    }
  }, [idx, nonce, place, reduced, stops])

  useEffect(() => {
    if (closing) return
    const onClick = (e: MouseEvent) => {
      if (!e.isTrusted) return
      if (card.current && e.target instanceof Node && card.current.contains(e.target)) return
      e.preventDefault()
      e.stopPropagation()
    }
    const onKey = (e: KeyboardEvent) => {
      const i = index.current
      if (e.key === "Escape") {
        e.preventDefault()
        e.stopPropagation()
        onEnd()
      } else if (e.key === "ArrowRight" || (e.key === "Enter" && document.activeElement === document.body)) {
        e.preventDefault()
        if (i < stops.length - 1) show(i + 1)
        else onEnd()
      } else if (e.key === "ArrowLeft" && i > 0) {
        e.preventDefault()
        show(i - 1)
      } else if (e.key === "/") {
        e.stopPropagation()
      }
      if (e.key === "Tab" && card.current) {
        const focusable = [...card.current.querySelectorAll<HTMLButtonElement>("button:not([disabled])")]
        if (!focusable.length) return
        const k = focusable.indexOf(document.activeElement as HTMLButtonElement)
        if (e.shiftKey && k <= 0) {
          e.preventDefault()
          focusable[focusable.length - 1].focus()
        } else if (!e.shiftKey && k === focusable.length - 1) {
          e.preventDefault()
          focusable[0].focus()
        }
      }
    }
    window.addEventListener("click", onClick, true)
    window.addEventListener("keydown", onKey, true)
    return () => {
      window.removeEventListener("click", onClick, true)
      window.removeEventListener("keydown", onKey, true)
    }
  }, [closing, onEnd, show, stops])

  const stop = stops[idx]
  let extra: ReactNode = null
  if (stop.extra === "books") {
    extra = (
      <div className={styles.tcBooks}>
        {S.sources.filter((s) => s.kind !== "pick").map((s) => {
          const on = !prefs.off[s.name]
          return (
            <button key={s.name} className={styles.tcBk + (on ? " " + styles.on : "")} aria-pressed={on} title={s.name} onClick={() => { toggleSource(s.name); show(idx) }}>
              <BookLogo name={s.name} size={22} />
            </button>
          )
        })}
      </div>
    )
  } else if (stop.extra === "prefs") {
    extra = (
      <div className={styles.tcPrefs}>
        <div className="seg" role="group" aria-label="Odds format">
          {PREF_FORMATS.map(([k, label]) => (
            <button key={k} aria-pressed={prefs.fmt === k} onClick={() => { setFormat(k); show(idx) }}>{label}</button>
          ))}
        </div>
        <div className="seg" role="group" aria-label="Theme">
          <button aria-pressed={prefs.theme !== "light"} onClick={() => { setTheme("dark"); show(idx) }}>Dark</button>
          <button aria-pressed={prefs.theme === "light"} onClick={() => { setTheme("light"); show(idx) }}>Light</button>
        </div>
      </div>
    )
  }

  return (
    <div className={styles.ot + (reduced ? " " + styles.obRm : "") + (shown && !closing ? " " + styles.in : "")}>
      <div ref={spot} className={styles.otSpot} />
      <div ref={card} className={styles.otCard} role="dialog" aria-live="polite" aria-label={label}>
        <div className={styles.tcTop}>
          <span className={styles.tcN + " mono"}>
            {pad2(idx + 1)}
            <i>/{pad2(stops.length)}</i>
          </span>
          <span className={styles.tcBar}>
            <i style={{ width: ((idx + 1) / stops.length) * 100 + "%" }} />
          </span>
        </div>
        <h3>{stop.title}</h3>
        <p>{stop.text()}</p>
        {extra}
        {last && tip && (
          <p className={styles.tcTip}>
            Press <kbd>/</kbd> anywhere to search teams and leagues.
          </p>
        )}
        <div className={styles.tcFt}>
          <button className={styles.tcSkip} onClick={() => (last ? show(idx - 1) : onEnd())}>{last ? "Back" : "Skip tour"}</button>
          <button className={styles.tcNext} onClick={() => (last ? onEnd() : show(idx + 1))}>{last ? "Done" : "Next"}</button>
        </div>
      </div>
    </div>
  )
}
