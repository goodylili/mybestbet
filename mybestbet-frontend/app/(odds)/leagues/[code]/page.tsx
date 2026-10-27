import type { Metadata } from "next"
import { MatchList } from "@/components/mybestbet/matches/match-list"
import { compMeta } from "@/lib/catalog"

type Props = { params: Promise<{ code: string }> }

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { code } = await params
  const league = decodeURIComponent(code)
  return {
    title: `${compMeta(league)?.name ?? league} odds`,
    alternates: { canonical: `/leagues/${code}` },
  }
}

export default async function Page({ params }: Props) {
  const { code } = await params
  return <MatchList league={decodeURIComponent(code)} />
}
