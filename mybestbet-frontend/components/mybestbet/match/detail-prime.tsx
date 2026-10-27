"use client"

import { useEffect } from "react"
import { primeDetail } from "@/lib/store/data"

export function DetailPrime({ text }: { text: string }) {
  useEffect(() => {
    primeDetail(text)
  }, [text])
  return null
}
