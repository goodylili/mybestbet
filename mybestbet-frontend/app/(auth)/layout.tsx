import type { ReactNode } from "react"
import { AuthFrame } from "@/components/mybestbet/auth/auth-frame"
import { DarkOnly } from "@/components/mybestbet/shared/dark-only"

export default function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <>
      <DarkOnly />
      <AuthFrame>{children}</AuthFrame>
    </>
  )
}
