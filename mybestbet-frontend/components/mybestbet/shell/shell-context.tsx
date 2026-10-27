"use client"

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react"

export type ListFilter = "all" | "soon" | "arb" | "ev"

type Shell = {
  /** Search text from the top bar or the mobile search field; filters the match list. */
  q: string
  setQ: (q: string) => void
  /** Day shown on the match list and counted in the sidebar: 0 is today, up to 29. */
  date: number
  setDate: (d: number) => void
  /** Match list quick filter. */
  filter: ListFilter
  setFilter: (f: ListFilter) => void
  slipOpen: boolean
  sheetOpen: boolean
  openSlip: () => void
  openSheet: () => void
  closeAll: () => void
}

const ShellContext = createContext<Shell | null>(null)

/** UI state shared between the sidebar, top bar and the screens inside the odds app. */
export function ShellProvider({ children }: { children: ReactNode }) {
  const [q, setQ] = useState("")
  const [date, setDate] = useState(0)
  const [filter, setFilter] = useState<ListFilter>("all")
  const [slipOpen, setSlipOpen] = useState(false)
  const [sheetOpen, setSheetOpen] = useState(false)
  const closeAll = useCallback(() => { setSlipOpen(false); setSheetOpen(false) }, [])
  const value = useMemo<Shell>(
    () => ({
      q, setQ, date, setDate, filter, setFilter, slipOpen, sheetOpen,
      openSlip: () => setSlipOpen(true),
      openSheet: () => setSheetOpen(true),
      closeAll,
    }),
    [q, date, filter, slipOpen, sheetOpen, closeAll],
  )
  return <ShellContext.Provider value={value}>{children}</ShellContext.Provider>
}

export function useShell() {
  const shell = useContext(ShellContext)
  if (!shell) throw new Error("useShell must be used inside ShellProvider")
  return shell
}
