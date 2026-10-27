"use client"

import { useEffect, useId, useRef, useState, type CSSProperties, type MouseEvent } from "react"
import { InfoIcon } from "./icons"
import styles from "./info-tip.module.css"

const GAP = 8
const WIDTH = 280

function place(btn: HTMLElement): CSSProperties {
  const r = btn.getBoundingClientRect()
  const width = Math.min(WIDTH, window.innerWidth - GAP * 4)
  const left = Math.max(GAP * 2, Math.min(r.left + r.width / 2 - width / 2, window.innerWidth - width - GAP * 2))
  const below = window.innerHeight - r.bottom
  if (below >= 140 || below >= r.top) return { left, width, top: r.bottom + GAP }
  return { left, width, bottom: window.innerHeight - r.top + GAP }
}

/** A small (i) button that explains the thing next to it, on tap or hover. */
export function InfoTip({ label, text, className }: { label: string; text: string; className?: string }) {
  const [pos, setPos] = useState<CSSProperties | null>(null)
  const btn = useRef<HTMLButtonElement>(null)
  const pinned = useRef(false)
  const id = useId()

  const show = () => { if (btn.current) setPos(place(btn.current)) }
  const hide = () => { pinned.current = false; setPos(null) }

  useEffect(() => {
    if (!pos) return
    const onDown = (e: PointerEvent) => { if (!btn.current?.contains(e.target as Node)) hide() }
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") hide() }
    document.addEventListener("pointerdown", onDown)
    document.addEventListener("keydown", onKey)
    window.addEventListener("scroll", hide, { passive: true, capture: true })
    window.addEventListener("resize", hide)
    return () => {
      document.removeEventListener("pointerdown", onDown)
      document.removeEventListener("keydown", onKey)
      window.removeEventListener("scroll", hide, { capture: true })
      window.removeEventListener("resize", hide)
    }
  }, [pos])

  const toggle = (e: MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (pos && pinned.current) return hide()
    pinned.current = true
    show()
  }

  return (
    <>
      <button
        ref={btn}
        type="button"
        className={styles.ib + (className ? " " + className : "")}
        aria-label={"About " + label}
        aria-expanded={!!pos}
        aria-describedby={pos ? id : undefined}
        onClick={toggle}
        onPointerEnter={(e) => { if (e.pointerType === "mouse") show() }}
        onPointerLeave={(e) => { if (e.pointerType === "mouse" && !pinned.current) setPos(null) }}
        onFocus={show}
        onBlur={() => { if (!pinned.current) setPos(null) }}
      >
        <InfoIcon />
      </button>
      {pos && <span id={id} role="tooltip" className={styles.tip} style={pos}>{text}</span>}
    </>
  )
}
