"use client"

import { useEffect, useState } from "react"
import { useRouter } from "next/navigation"
import type { Team } from "@/lib/api/types"
import { COMPS, teamArt } from "@/lib/catalog"
import { subscribe } from "@/lib/store/emitter"
import { S, arbPct, dayFixtures, loadDetail, signalList, storeReady, watchExtra, type Fixture } from "@/lib/store/data"
import { inSlip, togglePick, type SlipPick } from "@/lib/store/prefs"

export type PreviewKind = "hero-books" | "hero-odds" | "hero-arbs" | "odds" | "arb" | "value"

const ALL: PreviewKind[] = ["hero-books", "hero-odds", "hero-arbs", "odds", "arb", "value"]
const ZERO = Object.fromEntries(ALL.map((k) => [k, 0])) as Record<PreviewKind, number>

/** Whether a team has a real crest (bundled or from the API), not a lettered tag. */
export function hasCrest(team: Team) {
  if (teamArt(team.name)) return true
  const known = team.logoUrl ? team : S.tname[team.name]
  return !!known?.logoUrl
}

export const arbsSorted = () => signalList("arb").slice().sort((x, y) => arbPct(y) - arbPct(x))
export const wrap = (i: number, n: number) => ((i % n) + n) % n
export const evsSorted = () => signalList("ev").slice().sort((x, y) => y.ev - x.ev)

function football() {
  const now = Date.now()
  return [...dayFixtures(0), ...dayFixtures(1)].filter((f) => f.sp === "football" && f.ko - now > 10 * 60000 && !!f.best && f.best.every((x) => x && x.d))
}

export type PreviewController = ReturnType<typeof createController>

/**
 * Each preview is redrawn from the store on a schedule, like the old innerHTML
 * renderer: on mount, 350ms after a store change, every 2.4s for the live
 * odds cards, every 4.8s for the rotating arb, and on its own controls. A draw
 * bumps the preview's counter, which remounts its content so flash and fade
 * animations replay exactly as before.
 */
function createController(bump: (kinds: PreviewKind[]) => void, bumpRev: () => void) {
  const ctl = {
    running: false,
    pend: null as ReturnType<typeof setTimeout> | null,
    step: 0,
    ui: { tab: 0, arb: 0, bank: "10000", ev: null as string | null, heroArb: 0 },
    note: {} as Partial<Record<PreviewKind, { at: number; add: boolean }>>,
    pick: { hero: null as string | null, odds: null as string[] | null },
    go: (() => {}) as (href: string) => void,

    heroFix(): Fixture | null {
      let f = ctl.pick.hero ? S.ev[ctl.pick.hero] : undefined
      if (!f) {
        const all = football(), withCrest = all.filter((x) => hasCrest(x.hT) && hasCrest(x.aT))
        f = withCrest[0] || all[0]
        ctl.pick.hero = f ? f.id : null
        if (f) loadDetail(f.id).catch(() => {})
      }
      return f || null
    },

    oddsFixes(): Fixture[] {
      if (!ctl.pick.odds || !ctl.pick.odds.length) {
        const hero = ctl.heroFix(), all = football().filter((f) => !hero || f.id !== hero.id)
        const good = all.filter((f) => hasCrest(f.hT) && hasCrest(f.aT))
        const seen: Record<string, boolean> = {}, out: string[] = [], pool = good.concat(all)
        COMPS.forEach(({ id }) => {
          if (out.length >= 3 || seen[id]) return
          const f = pool.find((x) => x.lg === id)
          if (f) { seen[id] = true; out.push(f.id) }
        })
        pool.forEach((f) => { if (out.length < 3 && !out.includes(f.id)) out.push(f.id) })
        ctl.pick.odds = out
        out.forEach((id) => loadDetail(id).catch(() => {}))
      }
      return ctl.pick.odds.map((id) => S.ev[id]).filter(Boolean)
    },

    /** The fixtures as last picked, without picking again. Used while rendering. */
    heroFixture: (): Fixture | null => (ctl.pick.hero && S.ev[ctl.pick.hero]) || null,
    oddsList: (): Fixture[] => (ctl.pick.odds || []).map((id) => S.ev[id]).filter(Boolean),

    draw(kinds: PreviewKind[]) {
      if (kinds.includes("hero-odds")) ctl.heroFix()
      if (kinds.includes("odds") && ctl.ui.tab >= ctl.oddsFixes().length) ctl.ui.tab = 0
      const now = Date.now()
      kinds.forEach((k) => { const n = ctl.note[k]; if (n && now - n.at >= 9000) delete ctl.note[k] })
      bump(kinds)
    },

    redrawSoon() {
      if (!ctl.running || ctl.pend) return
      ctl.pend = setTimeout(() => {
        ctl.pend = null
        const active = document.activeElement
        if (active && active.hasAttribute && active.hasAttribute("data-bank")) {
          ctl.draw(["hero-books", "hero-odds", "hero-arbs", "odds", "value"])
          return
        }
        ctl.draw(ALL)
        ctl.watchNow()
      }, 350)
    },

    watchNow() {
      if (!ctl.running || !storeReady()) return
      const ids: string[] = [], h = ctl.heroFix()
      if (h) ids.push(h.id)
      const fx = ctl.oddsFixes(), o = fx[ctl.ui.tab] || fx[0]
      if (o && !ids.includes(o.id)) ids.push(o.id)
      watchExtra(ids)
    },

    tick() {
      if (document.hidden) return
      ctl.step++
      if (ctl.step % 2 === 0) { ctl.ui.heroArb++; ctl.draw(["hero-arbs"]) }
      ctl.draw(["hero-odds", "odds"])
    },

    pickPrice(kind: PreviewKind, pick: Omit<SlipPick, "stake">) {
      togglePick(pick)
      ctl.note[kind] = { at: Date.now(), add: inSlip(pick.fx, pick.key, pick.i) }
      ctl.draw(["hero-odds", "odds"])
    },

    setTab(k: number) { ctl.ui.tab = k; ctl.draw(["odds"]); ctl.watchNow() },
    stepArb(d: number) { ctl.ui.arb = wrap(ctl.ui.arb + d, Math.min(3, arbsSorted().length) + 1); ctl.draw(["arb"]) },
    toggleEv(id: string) { ctl.ui.ev = ctl.ui.ev === id ? null : id; ctl.draw(["value"]) },
    setBank(v: string) { ctl.ui.bank = v; bumpRev() },

    route(go: (href: string) => void) { ctl.go = go },

    mount() {
      ctl.draw(ALL)
      const off = subscribe(() => {
        if (!ctl.pick.hero || !S.ev[ctl.pick.hero]) { ctl.pick.hero = null; ctl.pick.odds = null }
        ctl.redrawSoon()
      })
      ctl.running = true
      ctl.watchNow()
      const timer = setInterval(ctl.tick, 2400)
      return () => {
        ctl.running = false
        clearInterval(timer)
        if (ctl.pend) clearTimeout(ctl.pend)
        ctl.pend = null
        off()
        watchExtra([])
      }
    },
  }
  return ctl
}

/** Runs the landing previews while the page is mounted. */
export function usePreviews() {
  const router = useRouter()
  const [draws, setDraws] = useState(ZERO)
  const [rev, setRev] = useState(0)
  const [ctl] = useState(() => createController(
    (kinds) => setDraws((d) => { const next = { ...d }; kinds.forEach((k) => { next[k]++ }); return next }),
    () => setRev((r) => r + 1),
  ))

  useEffect(() => ctl.route((href) => router.push(href)), [ctl, router])

  useEffect(() => ctl.mount(), [ctl])

  return { draws, rev, ctl }
}
