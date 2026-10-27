import type { Metadata } from "next"
import { MatchList } from "@/components/mybestbet/matches/match-list"

export const metadata: Metadata = {
  title: "Favourites",
  robots: { index: false },
}

export default function Page() {
  return <MatchList favourites />
}
