"use client"

import { useLayoutEffect, useRef, useState, type ReactNode } from "react"
import { ChevronDownIcon } from "./icons"

/**
 * A wrapping row of chips or tabs that keeps to one line: whatever doesn't fit
 * folds into a "+N" button that expands the row. The selected item never folds.
 */
export function FitRow({ className, label, role, tabs = false, tour, children }: {
  className: string
  label: string
  role?: string
  tabs?: boolean
  /** Optional data-tour hook for the onboarding spotlight. */
  tour?: string
  children: ReactNode
}) {
  const ref = useRef<HTMLDivElement>(null)
  const button = useRef<HTMLButtonElement>(null)
  const [open, setOpen] = useState(false)
  const [folded, setFolded] = useState(0)

  useLayoutEffect(() => {
    const row = ref.current, more = button.current
    if (!row || !more) return
    const measure = () => {
      const items = [...row.children].filter((c) => c !== more && (c.tagName === "BUTTON" || c.classList.contains("seg"))) as HTMLElement[]
      items.forEach((c) => c.classList.remove("fx-h"))
      row.classList.remove("fit")
      more.style.display = "none"
      if (!items.length) return setFolded(0)
      const top = items[0].offsetTop
      if (!items.some((c) => c.offsetTop > top + 4)) return setFolded(0)
      row.classList.add("fit")
      more.style.display = ""
      if (open) { more.dataset.n = "Less"; return setFolded(-1) }
      const selected = items.find((c) => c.getAttribute("aria-pressed") === "true" || c.getAttribute("aria-selected") === "true")
      let hidden = 0
      for (let i = items.length - 1; i >= 0; i--) {
        more.dataset.n = "+" + hidden
        if (more.offsetTop <= top + 4 && !items.some((c) => !c.classList.contains("fx-h") && c.offsetTop > top + 4)) break
        if (items[i] === selected) continue
        items[i].classList.add("fx-h")
        hidden++
      }
      more.dataset.n = "+" + hidden
      if (!hidden) {
        row.classList.remove("fit")
        more.style.display = "none"
      }
      setFolded(hidden)
    }
    measure()
    const observer = new ResizeObserver(() => measure())
    observer.observe(row.parentElement || row)
    return () => observer.disconnect()
  })

  return (
    <div ref={ref} className={className + (open ? " open" : "")} role={role} aria-label={label} data-tour={tour}>
      {children}
      <button
        ref={button}
        type="button"
        className={(tabs ? "" : "chip sm ") + "fx-b"}
        style={{ display: "none" }}
        data-n={open || folded < 0 ? "Less" : "+" + folded}
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <ChevronDownIcon />
      </button>
    </div>
  )
}
