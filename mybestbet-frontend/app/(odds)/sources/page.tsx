import type { Metadata } from "next"
import { SourcesPage } from "@/components/mybestbet/sources/sources-page"

export const metadata: Metadata = {
  title: "Sources",
  description: "Health of every sportsbook and prediction market feed mybestbet reads.",
  alternates: { canonical: "/sources" },
}

export default function Page() {
  return <SourcesPage />
}
