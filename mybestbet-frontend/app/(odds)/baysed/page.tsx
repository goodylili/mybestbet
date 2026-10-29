import type { Metadata } from "next"
import { BaysedPage } from "@/components/mybestbet/baysed/baysed-page"

export const metadata: Metadata = {
  title: "Baysed",
  description: "Bayse lines priced out of line with every other book.",
  alternates: { canonical: "/baysed" },
  robots: { index: false },
}

export default function Page() {
  return <BaysedPage />
}
