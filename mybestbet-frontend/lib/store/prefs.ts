import type { Settings } from "@/lib/api/types"
import type { OddsFormat } from "@/lib/odds/format"
import { ALPHA2, zoneCountry } from "@/lib/geo"
import { emit } from "./emitter"

export type Theme = "dark" | "light"

export type SlipPick = {
  fx: string
  key: string
  i: number
  code: string
  label: string
  stake: string
  src?: string
}

type DisplayPrefs = { prob: boolean; flash: boolean; cd: boolean }

type GeoPrefs = { country: string; only: boolean; detected: string }

/** Viewer settings kept in localStorage: odds format, books turned off, favourites, slip and display toggles. */
export const prefs = {
  fmt: "dec" as OddsFormat,
  off: {} as Record<string, boolean>,
  starred: {} as Record<string, boolean>,
  display: { prob: true, flash: true, cd: true } as DisplayPrefs,
  theme: "dark" as Theme,
  slip: [] as SlipPick[],
  /** Chosen line per sport and market on the match list, e.g. "footballOU" → 2.5. */
  lines: {} as Record<string, number>,
  /** Chosen match list market per sport. */
  markets: {} as Record<string, string>,
  geo: { country: "", only: true, detected: "" } as GeoPrefs,
  hydrated: false,
}

const read = <T,>(key: string, fallback: T): T => {
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return fallback
    const value = JSON.parse(raw)
    return value && typeof value === "object" ? value : fallback
  } catch {
    return fallback
  }
}

const write = (key: string, value: unknown) => {
  try {
    localStorage.setItem(key, typeof value === "string" ? value : JSON.stringify(value))
  } catch {}
}

let changed = () => {}

export const onPrefsChange = (fn: () => void) => {
  changed = fn
}

const save = (key: string, value: unknown) => {
  write(key, value)
  emit()
  changed()
}

const FORMATS: OddsFormat[] = ["dec", "us", "pct"]

const SLIP_KEY = "mbb.slip.v1"
const SLIP_MAX = 30

const isPick = (v: unknown): v is SlipPick => {
  if (!v || typeof v !== "object") return false
  const p = v as Record<string, unknown>
  return typeof p.fx === "string" && typeof p.key === "string" && Number.isInteger(p.i) && typeof p.code === "string" &&
    typeof p.label === "string" && typeof p.stake === "string" && (p.src == null || typeof p.src === "string")
}

function setSlip(slip: SlipPick[]) {
  prefs.slip = slip
  write(SLIP_KEY, slip)
  emit()
}

const isFlags = (v: unknown): v is Record<string, boolean> =>
  !!v && typeof v === "object" && !Array.isArray(v) && Object.values(v).every((x) => typeof x === "boolean")

export const exportSettings = (): Settings => ({
  fmt: prefs.fmt, off: prefs.off, starred: prefs.starred, display: prefs.display, theme: prefs.theme,
  country: prefs.geo.country, localOnly: prefs.geo.only,
})

const isCountry = (v: unknown): v is string => typeof v === "string" && (v === "" || ALPHA2.test(v))

const geoValue = () => ({ country: prefs.geo.country, only: prefs.geo.only })

export function importSettings(s: Settings) {
  if (FORMATS.includes(s.fmt as OddsFormat)) write("mbb.fmt", (prefs.fmt = s.fmt as OddsFormat))
  if (isFlags(s.off)) write("mbb.off", (prefs.off = s.off))
  if (isFlags(s.starred)) write("mbb.star", (prefs.starred = s.starred))
  if (isFlags(s.display)) write("mbb.prefs", (prefs.display = { ...prefs.display, ...s.display }))
  if (s.theme === "dark" || s.theme === "light") applyTheme(s.theme)
  if (isCountry(s.country)) prefs.geo.country = s.country
  if (typeof s.localOnly === "boolean") prefs.geo.only = s.localOnly
  write("mbb.geo", geoValue())
  applyDisplay()
  emit()
}

let darkLocks = 0

function savedTheme(): Theme {
  try {
    const t = localStorage.getItem("mbb-theme")
    if (t === "dark" || t === "light") return t
    return matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark"
  } catch {
    return "dark"
  }
}

const paint = (theme: Theme) => {
  document.documentElement.dataset.theme = darkLocks ? "dark" : theme
}

export function lockDark() {
  darkLocks++
  paint("dark")
  return () => {
    darkLocks--
    paint(prefs.hydrated ? prefs.theme : savedTheme())
  }
}

function applyTheme(theme: Theme) {
  write("mbb-theme", theme)
  prefs.theme = theme
  paint(theme)
}

export function hydratePrefs() {
  if (prefs.hydrated || typeof window === "undefined") return
  try {
    const fmt = localStorage.getItem("mbb.fmt")
    if (FORMATS.includes(fmt as OddsFormat)) prefs.fmt = fmt as OddsFormat
  } catch {}
  prefs.off = read("mbb.off", {})
  prefs.starred = read("mbb.star", {})
  prefs.display = { ...prefs.display, ...read("mbb.prefs", {}) }
  prefs.theme = savedTheme()
  const geo = read<{ country?: unknown; only?: unknown }>("mbb.geo", {})
  prefs.geo = { country: isCountry(geo.country) ? geo.country : "", only: geo.only !== false, detected: zoneCountry() }
  const slip = read<unknown>(SLIP_KEY, [])
  prefs.slip = Array.isArray(slip) ? slip.filter(isPick).slice(0, SLIP_MAX) : []
  prefs.hydrated = true
  applyDisplay()
  emit()
}

function applyDisplay() {
  const b = document.body.classList
  b.toggle("no-prob", !prefs.display.prob)
  b.toggle("no-flash", !prefs.display.flash)
  b.toggle("no-cd", !prefs.display.cd)
}

export function setFormat(fmt: OddsFormat) {
  prefs.fmt = fmt
  save("mbb.fmt", fmt)
}

export function toggleDisplay(key: keyof DisplayPrefs) {
  prefs.display = { ...prefs.display, [key]: !prefs.display[key] }
  applyDisplay()
  save("mbb.prefs", prefs.display)
}

export function setTheme(theme: Theme) {
  const root = document.documentElement
  root.classList.add("thx")
  setTimeout(() => root.classList.remove("thx"), 450)
  applyTheme(theme)
  emit()
  changed()
}

export const userCountry = () => prefs.geo.country || prefs.geo.detected

export function setDetectedCountry(code?: string) {
  if (!code || !ALPHA2.test(code) || code === prefs.geo.detected) return
  prefs.geo = { ...prefs.geo, detected: code }
  emit()
}

export function setCountry(code: string) {
  if (!isCountry(code)) return
  prefs.geo = { ...prefs.geo, country: code }
  save("mbb.geo", geoValue())
}

export function toggleLocalOnly() {
  prefs.geo = { ...prefs.geo, only: !prefs.geo.only }
  save("mbb.geo", geoValue())
}

export function toggleSource(name: string) {
  const off = { ...prefs.off }
  if (off[name]) delete off[name]
  else off[name] = true
  prefs.off = off
  save("mbb.off", off)
}

export function setAllSources(names: string[], on: boolean) {
  prefs.off = on ? {} : Object.fromEntries(names.map((n) => [n, true]))
  save("mbb.off", prefs.off)
}

export function toggleStar(id: string) {
  const starred = { ...prefs.starred }
  if (starred[id]) delete starred[id]
  else starred[id] = true
  prefs.starred = starred
  save("mbb.star", starred)
}

export function setLine(sport: string, code: string, line: number) {
  prefs.lines = { ...prefs.lines, [sport + code]: line }
  emit()
}

export function setListMarket(sport: string, code: string) {
  prefs.markets = { ...prefs.markets, [sport]: code }
  emit()
}

export const inSlip = (fx: string, key: string, i: number, src?: string | null) =>
  prefs.slip.some((p) => p.fx === fx && p.key === key && p.i === i && (src == null || (p.src || "") === src))

/** Add a price to the slip, switch its book, or remove it when tapped again. */
export function togglePick(pick: Omit<SlipPick, "stake">) {
  const slip = prefs.slip.slice()
  const k = slip.findIndex((p) => p.fx === pick.fx && p.key === pick.key && p.i === pick.i)
  if (k >= 0 && (!pick.src || slip[k].src === pick.src)) slip.splice(k, 1)
  else if (k >= 0) slip[k] = { ...slip[k], src: pick.src }
  else slip.push({ ...pick, stake: "1000", src: pick.src || undefined })
  setSlip(slip)
}

export function setStake(index: number, stake: string) {
  setSlip(prefs.slip.map((p, k) => (k === index ? { ...p, stake } : p)))
}

export function setPickSource(index: number, src: string) {
  setSlip(prefs.slip.map((p, k) => (k === index ? { ...p, src } : p)))
}

export function removePick(index: number) {
  setSlip(prefs.slip.filter((_, k) => k !== index))
}

export function clearSlip() {
  setSlip([])
}
