import catalog from "@/lib/data/catalog.json"
import { hash } from "@/lib/odds/hash"

type TeamEntry = { abbr: string; color: string; logo: string | null }
type Comp = { id: string; sp: string; grp: string; name: string; mk: string | null }

const teams = catalog.teams as Record<string, TeamEntry>
const img = catalog.img as Record<string, string>
const light = new Set(catalog.light as string[])

export const COMPS = catalog.comps as Comp[]
export const compMeta = (code: string) => COMPS.find((c) => c.id === code)
export const compOrder = (code: string) => { const k = COMPS.findIndex((c) => c.id === code); return k < 0 ? 999 : k }
export const banner = (code?: string | null) => (code ? (catalog.banners as Record<string, string>)[code] || null : null)
export const photo = (name: string) => (catalog.photos as Record<string, string>)[name] || ""
export const heroVideo = catalog.heroVideo as string | null

export type Art = { src: string; light: boolean; flag: boolean; symbol: boolean }

function art(key?: string | null): Art | null {
  if (!key || !img[key]) return null
  return { src: img[key], light: light.has(key), flag: key.startsWith("f_"), symbol: key.startsWith("si_") }
}

const PALETTE = ["#3B82F6", "#EF4444", "#10B981", "#F59E0B", "#8B5CF6", "#EC4899", "#14B8A6", "#F97316", "#6366F1", "#84CC16"]

/** Three-letter abbreviation and colour for a team, from the catalog or derived from its name. */
export function teamTag(name: string): [string, string] {
  const t = teams[name]
  if (t) return [t.abbr, t.color]
  const words = name.replace(/[^\p{L}\p{N} ]/gu, "").split(/\s+/).filter(Boolean)
  return [(words.length > 1 ? words.map((w) => w[0]).join("") : name).slice(0, 3).toUpperCase(), PALETTE[Math.floor(hash(name) * PALETTE.length)]]
}

export const teamArt = (name: string) => art(teams[name]?.logo)
export const bookArt = (name: string) => art("s_" + name)
export const compArt = (code: string) => art(compMeta(code)?.mk)
