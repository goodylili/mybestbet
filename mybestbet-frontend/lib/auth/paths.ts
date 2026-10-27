export const HOME = "/sports/football"

const AUTH_PAGES = ["/login", "/signup", "/verify", "/forgot", "/reset"]

const BASE = "https://mybestbet.invalid"

export function safeNext(next?: string | null) {
  if (!next || !next.startsWith("/") || /[\x00-\x1f\x7f\\]/.test(next)) return HOME
  let url: URL
  try {
    url = new URL(next, BASE)
  } catch {
    return HOME
  }
  const path = url.pathname + url.search + url.hash
  if (url.origin !== BASE || path.startsWith("//") || AUTH_PAGES.includes(url.pathname)) return HOME
  return path
}

export function authHref(path: string, params: Record<string, string | number | undefined> = {}) {
  const query = new URLSearchParams()
  Object.entries(params).forEach(([k, v]) => {
    if (v !== undefined && v !== "" && !(k === "next" && v === HOME)) query.set(k, String(v))
  })
  const qs = query.toString()
  return qs ? path + "?" + qs : path
}

export const loginHref = (next?: string) => authHref("/login", { next: next && safeNext(next) })

export const googleHref = (next: string) => "/api/v1/auth/google/start?" + new URLSearchParams({ next: safeNext(next) })

export type SearchParams = Promise<Record<string, string | string[] | undefined>>

export async function authProps(searchParams: SearchParams) {
  const q = await searchParams
  const one = (k: string) => {
    const v = q[k]
    return (Array.isArray(v) ? v[0] : v) || undefined
  }
  const resend = Number(one("resend"))
  return {
    next: safeNext(one("next")),
    email: one("email"),
    resend: resend > 0 && resend <= 3600 ? resend : undefined,
    googleFailed: one("error") === "google",
  }
}
