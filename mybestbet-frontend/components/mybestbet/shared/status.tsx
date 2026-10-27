"use client"

import { ApiError as HttpError } from "@/lib/api/client"
import { boot, S } from "@/lib/store/data"

export function errorCopy(e: unknown, thing = "page"): string {
  const status = e instanceof HttpError ? e.status : -1
  if (status === 404) return `This ${thing} doesn't exist or has been removed.`
  if (status === 400 || status === 422) return `That link doesn't point to a valid ${thing}.`
  if (status === 0) return "We couldn't reach the server. Check your connection and try again."
  if (status === 429) return "Too many requests right now. Wait a moment and try again."
  if (status === 401 || status === 403) return `You need to sign in to see this ${thing}.`
  return "Something went wrong on our side. Try again in a moment."
}

export function Loading({ label = "Loading" }: { label?: string }) {
  return (
    <div className="empty ld">
      <span className="spin" aria-hidden="true" />
      <p>{label}</p>
    </div>
  )
}

export function ApiError() {
  return (
    <div className="empty">
      <p>Can&apos;t reach the mybestbet API</p>
      <p className="faint">{S.err}</p>
      <button className="btn sm" style={{ marginTop: 8 }} onClick={() => boot()}>
        Try again
      </button>
    </div>
  )
}
