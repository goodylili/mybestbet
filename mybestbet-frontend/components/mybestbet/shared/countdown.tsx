"use client"

import { useEffect, useState } from "react"
import type { Fixture } from "@/lib/store/data"
import { countdown } from "@/lib/odds/time"

/** The current time, refreshed every `ms`. */
export function useNow(ms = 1000) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), ms)
    return () => clearInterval(id)
  }, [ms])
  return now
}

/** Ticking time to kick-off, or the match's status when it won't start on time. */
export function Countdown({ f }: { f: Pick<Fixture, "ko" | "status"> }) {
  const now = useNow()
  if (f.status === "postponed") return <span className="faint">Postponed</span>
  if (f.status === "cancelled") return <span className="faint">Cancelled</span>
  if (f.status === "finished") return <span className="faint">Ended</span>
  return <span className="cd mono" suppressHydrationWarning>{countdown(f.ko - now)}</span>
}

/** Whether kick-off is within the hour, for the amber "soon" colour. */
export const startsSoon = (f: Pick<Fixture, "ko">) => f.ko - Date.now() <= 3600000
