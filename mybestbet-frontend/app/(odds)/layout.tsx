import type React from "react"
import { Suspense } from "react"
import { AppShell } from "@/components/mybestbet/shell/app-shell"
import { BoardSeed } from "@/components/mybestbet/shell/board-seed"

export default function OddsLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <>
      <Suspense fallback={null}>
        <BoardSeed />
      </Suspense>
      <AppShell>{children}</AppShell>
    </>
  )
}
