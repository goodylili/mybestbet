"use client"

import { useEffect, useRef, useState } from "react"
import { ChevronLeftIcon, ChevronRightIcon } from "./icons"
import styles from "./pager.module.css"

export const PER_PAGE = 10

/**
 * Page state for a list of 10 per page. `resetKey` should change whenever the
 * list's filters change, which sends the viewer back to page 1.
 */
export function usePage<T>(items: T[], resetKey: string) {
  const [state, setState] = useState({ key: resetKey, page: 0 })
  const page = state.key === resetKey ? state.page : 0
  const pages = Math.max(1, Math.ceil(items.length / PER_PAGE))
  const current = Math.min(page, pages - 1)
  return {
    page: current,
    total: items.length,
    items: items.slice(current * PER_PAGE, current * PER_PAGE + PER_PAGE),
    setPage: (p: number) => setState({ key: resetKey, page: p }),
  }
}

/** "11–20 of 64" with page numbers, shown only when there is more than one page. */
export function Pager({ page, total, onPage, label = "List", anchor }: {
  page: number
  total: number
  onPage: (page: number) => void
  label?: string
  /** The element at the top of the list, scrolled back into view when it's above the fold. */
  anchor?: React.RefObject<HTMLElement | null>
}) {
  const pending = useRef(false)
  useEffect(() => {
    if (!pending.current) return
    pending.current = false
    const el = anchor?.current
    if (el && el.getBoundingClientRect().top < 70) el.scrollIntoView({ block: "start", behavior: "smooth" })
  }, [page, anchor])
  if (total <= PER_PAGE) return null
  const pages = Math.ceil(total / PER_PAGE)
  const numbers: number[] = []
  for (let i = 0; i < pages; i++) {
    if (i === 0 || i === pages - 1 || Math.abs(i - page) <= 1) numbers.push(i)
    else if (numbers[numbers.length - 1] !== -1) numbers.push(-1)
  }
  const go = (p: number) => {
    pending.current = true
    onPage(p)
  }
  return (
    <nav className={styles.pager} aria-label={label + " pages"}>
      <span className={styles.count}>
        {page * PER_PAGE + 1}–{Math.min(total, page * PER_PAGE + PER_PAGE)} <span>of {total}</span>
      </span>
      <span className={styles.numbers}>
        <button className={styles.button + " " + styles.arrow} disabled={page === 0} aria-label="Previous page" onClick={() => go(page - 1)}>
          <ChevronLeftIcon />
        </button>
        {numbers.map((i, k) =>
          i < 0 ? (
            <span key={"e" + k} className={styles.ellipsis} aria-hidden="true">…</span>
          ) : (
            <button key={i} className={styles.button} aria-current={i === page ? "page" : undefined} aria-label={"Page " + (i + 1)} onClick={() => go(i)}>
              {i + 1}
            </button>
          ),
        )}
        <button className={styles.button + " " + styles.arrow} disabled={page >= pages - 1} aria-label="Next page" onClick={() => go(page + 1)}>
          <ChevronRightIcon />
        </button>
      </span>
    </nav>
  )
}
