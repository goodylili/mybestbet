import { useCallback, useSyncExternalStore } from "react"

let version = 0
const listeners = new Set<() => void>()

/** Tell every subscribed component that the store changed. */
export function emit() {
  version++
  listeners.forEach((fn) => {
    try {
      fn()
    } catch {}
  })
}

export function subscribe(fn: () => void) {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

/** Re-render on any store change. Returns the store version, which works as a memo key. */
export function useStore() {
  return useSyncExternalStore(subscribe, () => version, () => 0)
}

const keyVersion = new Map<string, number>()
const keyListeners = new Map<string, Set<() => void>>()

export function emitKey(key: string) {
  keyVersion.set(key, (keyVersion.get(key) || 0) + 1)
  keyListeners.get(key)?.forEach((fn) => {
    try {
      fn()
    } catch {}
  })
}

function subscribeKey(key: string, fn: () => void) {
  let set = keyListeners.get(key)
  if (!set) keyListeners.set(key, (set = new Set()))
  set.add(fn)
  return () => {
    set.delete(fn)
    if (!set.size) keyListeners.delete(key)
  }
}

export function useKey(key: string) {
  return useSyncExternalStore(
    useCallback((fn: () => void) => subscribeKey(key, fn), [key]),
    () => keyVersion.get(key) || 0,
    () => 0,
  )
}
