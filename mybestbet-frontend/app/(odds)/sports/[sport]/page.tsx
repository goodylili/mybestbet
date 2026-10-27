import type { Metadata } from "next"
import { notFound } from "next/navigation"
import { MatchList } from "@/components/mybestbet/matches/match-list"
import { SPORTS, sportName } from "@/lib/odds/markets"

type Props = { params: Promise<{ sport: string }> }

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { sport } = await params
  return {
    title: `${sportName(sport)} odds`,
    alternates: { canonical: `/sports/${sport}` },
  }
}

export function generateStaticParams() {
  return SPORTS.map((s) => ({ sport: s.id }))
}

export default async function Page({ params }: Props) {
  const { sport } = await params
  if (!SPORTS.some((s) => s.id === sport)) notFound()
  return <MatchList sport={sport} />
}
