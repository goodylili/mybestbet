import { ApiError, SNAPSHOT, previewing, publicPost, put, send, setExpireHandler, setPreview } from "@/lib/api/client"
import type { Meta, SettingsBody, User, UserBody } from "@/lib/api/types"
import { isGuest } from "@/lib/auth/guest"
import { startStore } from "./data"
import { emit } from "./emitter"
import { exportSettings, hydratePrefs, importSettings, onPrefsChange, setDetectedCountry } from "./prefs"

export const auth = { status: "unknown" as "unknown" | "in" | "out", user: null as User | null }

function setUser(user: User | null) {
  auth.user = user
  auth.status = user ? "in" : "out"
  emit()
  return user
}

setExpireHandler(() => setUser(null))

let me: Promise<User | null> | null = null

export function loadMe(redirect = true) {
  me ??= send<UserBody>("/v1/me", { redirect }).then(
    (r) => setUser(r.user),
    (e) => {
      me = null
      if (e instanceof ApiError && e.status === 401) return setUser(null)
      throw e
    },
  )
  return me
}

let meta: Promise<Meta> | null = null

export const loadMeta = () => (meta ??= send<Meta>("/v1/meta", { guard: false }).catch((): Meta => ({})))

let pushTimer: ReturnType<typeof setTimeout> | undefined

const pushSettings = () => {
  clearTimeout(pushTimer)
  pushTimer = undefined
  return put<SettingsBody>("/v1/me/settings", { settings: exportSettings() }).catch(() => {})
}

let synced = false

async function syncSettings() {
  if (synced) return
  synced = true
  try {
    const { settings } = await send<SettingsBody>("/v1/me/settings")
    if (settings && Object.keys(settings).length) importSettings(settings)
    else pushSettings()
  } catch {}
  onPrefsChange(() => {
    clearTimeout(pushTimer)
    pushTimer = setTimeout(pushSettings, 800)
  })
}

export async function startSession(landing = false) {
  hydratePrefs()
  if (!SNAPSHOT) loadMeta().then((m) => setDetectedCountry(m.country))
  if (SNAPSHOT) return startStore()
  if (!landing) startStore()
  let user: User | null = null
  try {
    user = await loadMe(!landing && !isGuest())
  } catch {
    if (!landing) return startStore()
  }
  if (user) {
    if (previewing()) return location.reload()
    syncSettings()
    return startStore()
  }
  if (landing) {
    setPreview(true)
    startStore()
  }
}

export async function signOut() {
  if (pushTimer) await pushSettings()
  await publicPost("/v1/auth/logout").catch(() => {})
  setUser(null)
  location.assign("/login")
}
