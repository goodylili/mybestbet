import type { Metadata } from "next"
import { Suspense } from "react"
import { DetailSeed } from "@/components/mybestbet/match/detail-seed"
import { MatchPage } from "@/components/mybestbet/match/match-page"

type Props = {
  params: Promise<{ id: string }>
  searchParams: Promise<{ market?: string }>
}

export const metadata: Metadata = {
  title: "Match odds",
}

export default async function Page({ params, searchParams }: Props) {
  const [{ id }, { market }] = await Promise.all([params, searchParams])
  const eventId = decodeURIComponent(id)
  return (
    <>
      <Suspense fallback={null}>
        <DetailSeed id={eventId} />
      </Suspense>
      <MatchPage id={eventId} market={market} />
    </>
  )
}
