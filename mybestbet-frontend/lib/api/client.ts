import type { ErrorBody, MarketBoard, MarketHistory, PropRow } from "./types"
import { snapshotApi } from "./snapshot"
import { loginHref } from "@/lib/auth/paths"

/** Sample data mode: serve the bundled snapshot from public/mybestbet/snapshot instead of the API. */
export const SNAPSHOT = process.env.NEXT_PUBLIC_MBB_SNAPSHOT === "1"

export const apiUrl = (path: string) => "/api" + path

let preview = false

export const setPreview = (on: boolean) => {
  preview = on
}

export const previewing = () => preview

export const sampleData = () => SNAPSHOT || preview

export class ApiError extends Error {
  constructor(readonly status: number, readonly code: string, message: string, readonly retryAfter?: number) {
    super(message)
  }
}

type BoardLookup = (key: string) => MarketBoard | PropRow | null

let lookup: BoardLookup = () => null

/** The snapshot fakes price history from the boards already loaded, so it needs a way to find them. */
export function setBoardLookup(fn: BoardLookup) {
  lookup = fn
}

type Refresh = "ok" | "denied" | "down"

let refreshing: Promise<Refresh> | null = null

export function refreshSession(): Promise<Refresh> {
  refreshing ??= fetch(apiUrl("/v1/auth/refresh"), { method: "POST", credentials: "same-origin", headers: { Accept: "application/json" } })
    .then((r): Refresh => (r.ok ? "ok" : r.status === 401 ? "denied" : "down"), (): Refresh => "down")
    .finally(() => {
      refreshing = null
    })
  return refreshing
}

let expired = false
let onExpire = () => {}

export const setExpireHandler = (fn: () => void) => {
  onExpire = fn
}

export async function expire() {
  if (expired) return
  expired = true
  onExpire()
  await send("/v1/auth/logout", { method: "POST", body: {}, guard: false }).catch(() => {})
  location.assign(loginHref(location.pathname + location.search))
}

async function toError(response: Response) {
  const body: ErrorBody | null = await response.json().catch(() => null)
  const wait = Number(response.headers.get("Retry-After")) || Number(body?.resendIn ?? body?.error?.resendIn) || undefined
  return new ApiError(response.status, body?.error?.code || "http_" + response.status, body?.error?.message || "HTTP " + response.status, wait)
}

type Options = { method?: string; body?: unknown; guard?: boolean; redirect?: boolean }

export async function send<T = unknown>(path: string, { method = "GET", body, guard = true, redirect = true }: Options = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" }
  if (body !== undefined) headers["Content-Type"] = "application/json"
  const init: RequestInit = { method, credentials: "same-origin", headers, body: body === undefined ? undefined : JSON.stringify(body) }
  const go = () => fetch(apiUrl(path), init).catch(() => {
    throw new ApiError(0, "network", "Network error")
  })
  let response = await go()
  if (response.status === 401 && guard) {
    const refreshed = await refreshSession()
    if (refreshed === "ok") response = await go()
    if (response.status === 401 && refreshed !== "down" && redirect) expire()
  }
  if (!response.ok) throw await toError(response)
  if (response.status === 204) return undefined as T
  return response.json().catch(() => undefined) as Promise<T>
}

export const api = <T = unknown>(path: string): Promise<T> => (sampleData() ? (snapshotApi(path, lookup) as Promise<T>) : send<T>(path))

export const post = <T = unknown>(path: string, body: unknown = {}) => send<T>(path, { method: "POST", body })

export const put = <T = unknown>(path: string, body: unknown) => send<T>(path, { method: "PUT", body })

export const publicPost = <T = unknown>(path: string, body: unknown = {}) => send<T>(path, { method: "POST", body, guard: false })

export type { MarketHistory }
