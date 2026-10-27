import type { Metadata } from "next"
import { GameLog } from "@/components/mybestbet/player/game-log"

type Props = {
  params: Promise<{ id: string }>
  searchParams: Promise<{ event?: string; key?: string }>
}

export const metadata: Metadata = {
  title: "Game log",
}

export default async function Page({ params, searchParams }: Props) {
  const [{ id }, { event, key }] = await Promise.all([params, searchParams])
  return <GameLog id={decodeURIComponent(id)} eventId={event} propKey={key} />
}
