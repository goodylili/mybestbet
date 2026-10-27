"use client"

import { useEffect, useId, useLayoutEffect, useRef, useState } from "react"
import { ChevronDownIcon, CloseIcon } from "@/components/mybestbet/shared/icons"
import { BookLogo } from "@/components/mybestbet/shared/logos"
import type { MarketBoard } from "@/lib/api/types"
import { decOf } from "@/lib/odds/format"
import { isShown, type Fixture } from "@/lib/store/data"
import type { RowPrice } from "./price-button"
import styles from "./match-row.module.css"

const SHOWN = 3
const MENU_MAX = 280
const GAP = 4

type MenuPos = { left: number; top?: number; bottom?: number; maxHeight: number }

function menuPos(button: HTMLElement): MenuPos {
  const r = button.getBoundingClientRect()
  const below = window.innerHeight - r.bottom - GAP * 2
  const above = r.top - GAP * 2
  const left = Math.max(GAP * 2, Math.min(r.left, window.innerWidth - 200))
  if (below >= Math.min(MENU_MAX, 160) || below >= above) return { left, top: r.bottom + GAP, maxHeight: Math.min(MENU_MAX, below) }
  return { left, bottom: window.innerHeight - r.top + GAP, maxHeight: Math.min(MENU_MAX, above) }
}

export function rankBooks(f: Fixture, board: MarketBoard | null, best: RowPrice[]) {
  const wins = new Map<string, number>()
  const sum = new Map<string, number>()
  best.forEach((x) => {
    if (x?.src) wins.set(x.src, (wins.get(x.src) || 0) + 1)
  })
  if (board) {
    board.selections.forEach((s) => {
      Object.keys(s.cells).forEach((n) => {
        const c = s.cells[n], v = decOf(c.price)
        if (v && !c.derived && !c.price.suspended && isShown(n)) sum.set(n, (sum.get(n) || 0) + v)
      })
    })
  } else {
    f.sources.forEach((n) => {
      if (isShown(n)) sum.set(n, 0)
    })
  }
  return [...sum.keys()].sort((a, b) => (wins.get(b) || 0) - (wins.get(a) || 0) || (sum.get(b) || 0) - (sum.get(a) || 0) || a.localeCompare(b))
}

export function BookStack({ books, market, only, onPick }: {
  books: string[]
  market: string
  only: string | null
  onPick: (book: string | null) => void
}) {
  const [open, setOpen] = useState(false)
  const [pos, setPos] = useState<MenuPos | null>(null)
  const box = useRef<HTMLDivElement>(null)
  const btn = useRef<HTMLButtonElement>(null)
  const id = useId()

  useEffect(() => {
    if (!open) return
    const away = (e: PointerEvent) => {
      if (!box.current?.contains(e.target as Node)) setOpen(false)
    }
    const esc = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false)
    }
    const close = (e: Event) => {
      if (!(e.target instanceof Node && box.current?.contains(e.target))) setOpen(false)
    }
    document.addEventListener("pointerdown", away)
    document.addEventListener("keydown", esc)
    window.addEventListener("resize", close)
    window.addEventListener("scroll", close, { passive: true, capture: true })
    return () => {
      document.removeEventListener("pointerdown", away)
      document.removeEventListener("keydown", esc)
      window.removeEventListener("resize", close)
      window.removeEventListener("scroll", close, { capture: true })
    }
  }, [open])

  useLayoutEffect(() => {
    setPos(open && btn.current ? menuPos(btn.current) : null)
  }, [open])

  if (!books.length) return null
  const rest = books.length - SHOWN
  const pick = (n: string) => {
    onPick(n === only ? null : n)
    setOpen(false)
  }

  return (
    <div className={styles.books} ref={box}>
      <button
        ref={btn}
        type="button"
        className={styles.bstack}
        aria-expanded={open}
        aria-controls={id}
        aria-label={only ? `Showing ${only} odds for ${market}, change book` : `${books.length} ${books.length === 1 ? "book" : "books"} for ${market}`}
        onClick={() => setOpen((o) => !o)}
      >
        {only ? (
          <>
            <span className={styles.logo}><BookLogo name={only} size={16} /></span>
            <span className={styles.bname}>{only}</span>
          </>
        ) : (
          <>
            <span className={styles.logos}>
              {books.slice(0, SHOWN).map((n) => (
                <span key={n} className={styles.logo}><BookLogo name={n} size={16} /></span>
              ))}
            </span>
            {rest > 0 ? <span>+{rest}</span> : null}
          </>
        )}
        <ChevronDownIcon />
      </button>
      {only ? (
        <button type="button" className={styles.bclear} aria-label={`Show best odds across all books for ${market}`} onClick={() => onPick(null)}>
          <CloseIcon />
        </button>
      ) : null}
      {open && pos ? (
        <ul id={id} className={styles.bmenu} style={pos} aria-label={`Books for ${market}`}>
          {books.map((n) => (
            <li key={n}>
              <button type="button" aria-pressed={n === only} onClick={() => pick(n)}>
                <BookLogo name={n} size={16} />
                <span>{n}</span>
              </button>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
