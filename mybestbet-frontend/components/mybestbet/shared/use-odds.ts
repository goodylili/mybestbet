"use client"

import { useCallback } from "react"
import { formatOdds } from "@/lib/odds/format"
import { useStore } from "@/lib/store/emitter"
import { prefs } from "@/lib/store/prefs"

/** Formats decimal odds in the viewer's chosen format (decimal, American or percent). */
export function useOdds() {
  useStore()
  const fmt = prefs.fmt
  return useCallback((d: number | null | undefined) => formatOdds(d, fmt), [fmt])
}
