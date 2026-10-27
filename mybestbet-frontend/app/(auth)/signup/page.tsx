import type { Metadata } from "next"
import { SignupForm } from "@/components/mybestbet/auth/forms"
import { authProps, type SearchParams } from "@/lib/auth/paths"

export const metadata: Metadata = {
  title: "Create an account",
  robots: { index: false, follow: false },
}

export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  return <SignupForm {...await authProps(searchParams)} />
}
