export const DAY = 864e5
export const NDAYS = 30

const WD = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]
const MON = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]

export const startOfDay = (t: number) => { const d = new Date(t); d.setHours(0, 0, 0, 0); return d.getTime() }
export const today0 = () => startOfDay(Date.now())
export const dayOf = (ko: number) => Math.round((startOfDay(ko) - today0()) / DAY)
export const dateFor = (d: number) => { const x = new Date(today0()); x.setDate(x.getDate() + d); return x }

export const dayLong = (d: number) => { const x = dateFor(d); return (d === 0 ? "Today, " : d === 1 ? "Tomorrow, " : WD[x.getDay()] + " ") + x.getDate() + " " + MON[x.getMonth()] }
export const dayTab = (d: number) => (d === 0 ? "Today" : WD[dateFor(d).getDay()] + " " + dateFor(d).getDate())
export const hhmm = (t: number) => { const d = new Date(t); return String(d.getHours()).padStart(2, "0") + ":" + String(d.getMinutes()).padStart(2, "0") }
export const dayShort = (t: number) => { const d = new Date(t); return d.getDate() + " " + MON[d.getMonth()] }
export const minutesTo = (ko: number) => (ko - Date.now()) / 60000
export const msSince = (t: number) => Date.now() - t
export const ago = (t: number) => { const s = Math.max(0, Math.round((Date.now() - t) / 1000)); return s < 60 ? s + "s" : s < 3600 ? Math.round(s / 60) + "m" : Math.round(s / 3600) + "h" }
export const msText = (ms: number) => (ms < 1000 ? "<1s" : ms < 60000 ? Math.round(ms / 1000) + "s" : Math.round(ms / 60000) + "m")

/** "1:04:22", "04:22" or "Live" once kick-off has passed. */
export function countdown(ms: number) {
  if (ms <= 0) return "Live"
  const t = Math.floor(ms / 1000), h = Math.floor(t / 3600), m = Math.floor(t / 60) % 60, s = t % 60
  const p = (n: number) => String(n).padStart(2, "0")
  return h ? h + ":" + p(m) + ":" + p(s) : p(m) + ":" + p(s)
}

export const WEEKDAYS = WD
export const MONTHS = MON

const tidy = (x?: string) => (x || "").replace(/[\u200e\u200f\u202a-\u202e]/g, "").replace(/[’′`]/g, "'").trim()

export function liveClock(s: { period?: string; clock?: string; minute?: number; detail?: string }) {
  const period = tidy(s.period), clock = tidy(s.clock) || (s.minute ? s.minute + "'" : "")
  if (period === "FT") return "Full time"
  if (period === "HT") return "Half time"
  return (period && period !== clock ? [period, clock] : [clock || period]).filter(Boolean).join(" ") || tidy(s.detail) || "Live"
}
