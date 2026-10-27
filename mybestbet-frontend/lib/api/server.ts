import { cookies } from "next/headers"

const API_URL = (process.env.API_URL || "http://localhost:8080").replace(/\/+$/, "")
const SESSION = ["__Host-mbb_at", "mbb_at"]
const TIMEOUT = 1500

export async function serverSession() {
  if (process.env.NEXT_PUBLIC_MBB_SNAPSHOT === "1") return null
  const jar = await cookies()
  const cookie = SESSION.flatMap((name) => {
    const value = jar.get(name)?.value
    return value ? [name + "=" + value] : []
  }).join("; ")
  return cookie || null
}

export async function serverText(path: string, cookie: string) {
  const r = await fetch(API_URL + path, { headers: { Cookie: cookie, Accept: "application/json" }, cache: "no-store", signal: AbortSignal.timeout(TIMEOUT) })
  if (!r.ok) throw new Error("HTTP " + r.status)
  return r.text()
}

export const serverJson = async <T,>(path: string, cookie: string): Promise<T> => JSON.parse(await serverText(path, cookie)) as T
