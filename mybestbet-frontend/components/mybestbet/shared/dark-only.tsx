"use client"

import { useEffect } from "react"
import { lockDark } from "@/lib/store/prefs"

export function DarkOnly() {
  useEffect(lockDark, [])
  return null
}
