"use client"

import { useEffect } from "react"
import { primeSeed, type BoardSeed } from "@/lib/store/data"

export function SeedPrime({ seed }: { seed: BoardSeed }) {
  useEffect(() => {
    primeSeed(seed)
  }, [seed])
  return null
}
