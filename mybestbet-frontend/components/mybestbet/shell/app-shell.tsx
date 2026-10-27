"use client"

import { usePathname } from "next/navigation"
import { useEffect, type ReactNode } from "react"
import { Onboarding } from "@/components/mybestbet/onboarding/onboarding"
import { useStore } from "@/lib/store/emitter"
import { prefs } from "@/lib/store/prefs"
import { startSession } from "@/lib/store/session"
import { BetSlip } from "./bet-slip"
import { SettingsSheet } from "./settings-sheet"
import { ShellProvider, useShell } from "./shell-context"
import { Sidebar } from "./sidebar"
import { SlipBar, TabBar } from "./tab-bar"
import { TopBar } from "./top-bar"
import styles from "./app-shell.module.css"

function Frame({ children }: { children: ReactNode }) {
  useStore()
  const shell = useShell()
  const pathname = usePathname()
  const { closeAll } = shell

  useEffect(() => {
    startSession()
  }, [])

  useEffect(() => {
    closeAll()
  }, [pathname, closeAll])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") closeAll()
    }
    document.addEventListener("keydown", onKey)
    return () => document.removeEventListener("keydown", onKey)
  }, [closeAll])

  const picks = prefs.slip.length > 0
  useEffect(() => {
    const b = document.body.classList
    b.toggle("slip-open", shell.slipOpen)
    b.toggle("sheet-open", shell.sheetOpen)
    b.toggle("has-picks", picks)
  }, [shell.slipOpen, shell.sheetOpen, picks])

  useEffect(() => () => document.body.classList.remove("slip-open", "sheet-open", "has-picks"), [])

  return (
    <>
      <TopBar />
      <div className={styles.shell}>
        <Sidebar />
        <main className={styles.main}>
          <div className={styles.mainIn + " main-in"}>{children}</div>
        </main>
        <BetSlip />
      </div>
      <div className={styles.scrim} onClick={closeAll} />
      <SettingsSheet />
      <SlipBar />
      <TabBar />
      <Onboarding />
    </>
  )
}

/** Everything around an odds screen: top bar, sidebar rail, slip, settings, tab bar and the first-visit onboarding. */
export function AppShell({ children }: { children: ReactNode }) {
  return (
    <ShellProvider>
      <Frame>{children}</Frame>
    </ShellProvider>
  )
}
