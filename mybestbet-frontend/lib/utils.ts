import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function webHref(raw?: string) {
  if (!raw) return undefined
  try {
    const url = new URL(raw)
    return url.protocol === 'https:' || url.protocol === 'http:' ? url.href : undefined
  } catch {
    return undefined
  }
}

const APP_SCHEMES = new Set(["sportybet:", "footballcom:", "bet9ja:", "betking:", "betpawa:", "betway:", "onexbet:", "1xbet:", "22bet:", "paripesa:", "betwinner:", "betano:", "bet365:"])

export function appHref(raw?: string) {
  if (!raw) return undefined
  const web = webHref(raw)
  if (web) return web
  try {
    const url = new URL(raw)
    return APP_SCHEMES.has(url.protocol) ? url.href : undefined
  } catch {
    return undefined
  }
}
