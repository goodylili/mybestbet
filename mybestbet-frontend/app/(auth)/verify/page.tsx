import type { Metadata } from "next"
import { VerifyForm } from "@/components/mybestbet/auth/forms"
import { authProps, type SearchParams } from "@/lib/auth/paths"

export const metadata: Metadata = {
  title: "Verify your email",
  robots: { index: false, follow: false },
}

export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  return <VerifyForm {...await authProps(searchParams)} />
}
