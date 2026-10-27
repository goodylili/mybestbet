import type { Metadata } from "next"
import { ResetForm } from "@/components/mybestbet/auth/forms"
import { authProps, type SearchParams } from "@/lib/auth/paths"

export const metadata: Metadata = {
  title: "Choose a new password",
  robots: { index: false, follow: false },
}

export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  return <ResetForm {...await authProps(searchParams)} />
}
