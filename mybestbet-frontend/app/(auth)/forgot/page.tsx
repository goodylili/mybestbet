import type { Metadata } from "next"
import { ForgotForm } from "@/components/mybestbet/auth/forms"
import { authProps, type SearchParams } from "@/lib/auth/paths"

export const metadata: Metadata = {
  title: "Reset your password",
  robots: { index: false, follow: false },
}

export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  return <ForgotForm {...await authProps(searchParams)} />
}
