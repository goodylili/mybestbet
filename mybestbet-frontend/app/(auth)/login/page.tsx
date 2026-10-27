import type { Metadata } from "next"
import { LoginForm } from "@/components/mybestbet/auth/forms"
import { authProps, type SearchParams } from "@/lib/auth/paths"

export const metadata: Metadata = {
  title: "Sign in",
  robots: { index: false, follow: false },
}

export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  return <LoginForm {...await authProps(searchParams)} />
}
