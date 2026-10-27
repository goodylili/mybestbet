import { headers } from "next/headers"
import { serverSession, serverText } from "@/lib/api/server"
import { DetailPrime } from "./detail-prime"

async function loadDetailText(id: string) {
  if ((await headers()).get("rsc")) return null
  const cookie = await serverSession()
  if (!cookie) return null
  try {
    return await serverText("/v1/events/" + encodeURIComponent(id), cookie)
  } catch {
    return null
  }
}

export async function DetailSeed({ id }: { id: string }) {
  const text = await loadDetailText(id)
  return text ? <DetailPrime text={text} /> : null
}
