"use client"

import { memo, useLayoutEffect, useRef, type CSSProperties, type FocusEvent, type ReactNode } from "react"
import Link from "next/link"
import type { MarketBoard, Team } from "@/lib/api/types"
import { teamTag } from "@/lib/catalog"
import { decOf, formatOdds, money, signedMoney } from "@/lib/odds/format"
import { mainMarket } from "@/lib/odds/markets"
import { routes } from "@/lib/routes"
import { S, arbPct, isOn, leagueName, type Fixture, type SignalItem } from "@/lib/store/data"
import { inSlip, prefs } from "@/lib/store/prefs"
import { arbStakes, mainKey, signalMarket } from "@/lib/store/selectors"
import { Countdown } from "@/components/mybestbet/shared/countdown"
import { CheckIcon, ChevronDownIcon, ChevronLeftIcon, ChevronRightIcon, ExternalIcon } from "@/components/mybestbet/shared/icons"
import { BookLogo, Crest, LeagueMark } from "@/components/mybestbet/shared/logos"
import { arbsSorted, evsSorted, wrap, type PreviewController, type PreviewKind } from "./use-previews"
import styles from "./previews.module.css"

const FLASH = 1100

const cx = (...names: (string | false | null | undefined)[]) => names.filter(Boolean).join(" ")
const F = (d: number | null | undefined) => formatOdds(d, prefs.fmt)
const abbr = (team: Team) => teamTag(team.name)[0]
const pct = (x: number, digits = 1) => (x * 100).toFixed(digits) + "%"
const liveBooks = () => S.sources.filter((s) => s.kind !== "pick" && isOn(s.name)).length

type Outcome = { label: string; side: string; team?: Team }
type Rung = { n: string; v: number }

function flash(key: string) {
  const fl = S.flash[key]
  if (!fl) return null
  const age = Date.now() - fl.at
  if (age > FLASH) return null
  return { c: fl.dir > 0 ? styles.up : styles.dn, s: { animationDelay: "-" + age + "ms" } as CSSProperties }
}

function mainBoard(f: Fixture): MarketBoard | null {
  const det = S.det[f.id]
  if (!det) return null
  const m = det.d.markets.find((x) => x.code === mainMarket(f.sp))
  return (m && m.periods[0] && m.periods[0].boards[0]) || null
}

function outcomes(f: Fixture): Outcome[] {
  const s: Outcome[] = [{ label: f.h, side: "home", team: f.hT }]
  if (f.sp === "football") s.push({ label: "Draw", side: "draw" })
  s.push({ label: f.a, side: "away", team: f.aT })
  return s
}

function ladder(b: MarketBoard, i: number): Rung[] {
  const s = b.selections[i]
  if (!s) return []
  return Object.keys(s.cells)
    .map((n) => ({ n, c: s.cells[n], v: decOf(s.cells[n].price) as number }))
    .filter((r) => r.v && !r.c.price.suspended && !r.c.derived && isOn(r.n))
    .sort((a, z) => z.v - a.v)
    .map(({ n, v }) => ({ n, v }))
}

const outLabel = (s: Outcome) => (s.team ? abbr(s.team) : s.label)

function Loading() {
  return <p className={styles.empty}>{S.boot === "ok" ? "No fixtures right now" : S.boot === "error" ? "Live prices unavailable" : "Loading live prices"}</p>
}

function Note({ ctl, kind, hint }: { ctl: PreviewController; kind: PreviewKind; hint: string }) {
  const n = ctl.note[kind]
  if (n) {
    return (
      <p className={cx(styles.note, styles.inNote)} role="status">
        {n.add ? <><CheckIcon /><span>Added to slip</span></> : <span>Removed from slip</span>}
        <span className={styles.sep}>·</span>
        <Link href={routes.sport("football")}>Open app</Link>
      </p>
    )
  }
  return <p className={styles.note} role="status">{hint}</p>
}

function HeroBooks() {
  const list = S.sources.filter((s) => s.kind !== "pick"), live = liveBooks(), down = list.filter((s) => s.down).length
  if (!list.length) return <Loading />
  const nb = list.filter((s) => s.kind === "book").length, npm = list.length - nb
  return (
    <div className={styles.hb}>
      <div className={styles.ttl}><b className={styles.num}>{list.length}</b><span>books</span></div>
      <p className={styles.sub}><span className={styles.m}>{nb}</span> sportsbooks, <span className={styles.m}>{npm}</span> prediction markets</p>
      <div className={styles.tiles}>
        {list.map((s, k) => (
          <span key={s.name} className={cx(styles.tile, !isOn(s.name) && styles.off)} style={{ "--k": k } as CSSProperties} title={s.name + (s.down ? " (down)" : "")}>
            <BookLogo name={s.name} size={32} />
          </span>
        ))}
      </div>
      <p className={styles.scan}>
        <i className={styles.dot} aria-hidden="true" />
        <span>Scanning</span>
        <span className={styles.sep}>·</span>
        <span><b className={styles.m}>{live}</b> live now</span>
        {down ? <><span className={styles.sep}>·</span><span className={styles.faint}><span className={styles.m}>{down}</span> down</span></> : null}
      </p>
    </div>
  )
}

function HeroOdds({ ctl }: { ctl: PreviewController }) {
  const f = ctl.heroFixture()
  if (!f) return <Loading />
  const b = mainBoard(f), ss = outcomes(f), key = b ? b.keyString : mainKey(f), code = mainMarket(f.sp)
  const lads: Rung[][] = ss.map((_, i) => {
    if (b) return ladder(b, i)
    const x = f.best?.[i]
    return x && x.d ? [{ n: x.src, v: x.d }] : []
  })
  const best = ss.map((s, i) => {
    const r = lads[i][0]
    if (!r) return <div key={i} className={styles.bc}><span className={styles.ol}>{outLabel(s)}</span><span className={styles.big}>–</span></div>
    const fl = flash(key + "#" + i + "#" + r.n), on = inSlip(f.id, key, i)
    return (
      <button
        key={i}
        className={cx(styles.bc, styles.best, on && styles.on, fl?.c)}
        style={fl?.s}
        data-fk={"pick:" + key + "#" + i}
        aria-pressed={on}
        aria-label={s.label + " at " + F(r.v) + " on " + r.n}
        onClick={() => ctl.pickPrice("hero-odds", { fx: f.id, key, i, code, label: s.label })}
      >
        <span className={styles.ol}>{s.team ? <Crest team={s.team} size={16} /> : null}{outLabel(s)}</span>
        <span className={cx(styles.big, styles.m)}>{F(r.v)}</span>
        <span className={styles.bk}><BookLogo name={r.n} size={16} /><em>{r.n}</em></span>
      </button>
    )
  })
  const rows: ReactNode[] = []
  for (let j = 1; j <= 3; j++) {
    ss.forEach((s, i) => {
      const r = lads[i][j]
      if (!r) { rows.push(<span key={j + "-" + i} className={cx(styles.nc, styles.na)}>–</span>); return }
      const fl = flash(key + "#" + i + "#" + r.n), on = inSlip(f.id, key, i, r.n)
      rows.push(
        <button
          key={j + "-" + i}
          className={cx(styles.nc, on && styles.on, fl?.c)}
          style={fl?.s}
          data-fk={"pick:" + key + "#" + i + "#" + r.n}
          aria-pressed={on}
          aria-label={s.label + " at " + F(r.v) + " on " + r.n}
          onClick={() => ctl.pickPrice("hero-odds", { fx: f.id, key, i, code, label: s.label, src: r.n })}
        >
          <BookLogo name={r.n} size={16} /><span className={styles.nn}>{r.n}</span><span className={styles.m}>{F(r.v)}</span>
        </button>,
      )
    })
  }
  return (
    <div>
      <div className={styles.top}>
        <span className={styles.comp}><LeagueMark code={f.lg} size={18} /><span>{leagueName(f.lg)}</span></span>
        <span className={styles.ko}><span className={styles.faint}>Kick-off</span> <Countdown f={f} /></span>
      </div>
      <div className={styles.teams}>
        <div className={styles.team}><Crest team={f.hT} size={52} /><b>{f.h}</b></div>
        <span className={styles.vs}>v</span>
        <div className={styles.team}><Crest team={f.aT} size={52} /><b>{f.a}</b></div>
      </div>
      <div className={styles.sec}><span>Best price</span><span><i className={styles.dot} aria-hidden="true" /><span className={styles.m}>{liveBooks()}</span> books</span></div>
      <div className={styles.bests}>{best}</div>
      <div className={cx(styles.sec, styles.sec2)}><span>Next best</span></div>
      <div className={styles.next}>{b ? rows : <p className={styles.empty}>Loading every book</p>}</div>
      <Note ctl={ctl} kind="hero-odds" hint="Tap a price to add it to your slip" />
    </div>
  )
}

function HeroArbs({ ctl }: { ctl: PreviewController }) {
  const arbs = arbsSorted(), n = arbs.length
  if (!n) return <div className={styles.ha}><div className={styles.lab}>Arbs open now</div><b className={styles.num}>{S.boot === "ok" ? 0 : "–"}</b></div>
  const top = Math.min(3, n), k = ctl.ui.heroArb % top, a = arbs[k], f = a.f
  return (
    <div className={styles.ha}>
      <div className={styles.lab}><i className={styles.dot} aria-hidden="true" />Arbs open now</div>
      <b className={styles.num}>{n}</b>
      <div className={cx(styles.arbtop, styles.fade)} data-k={k}>
        <span className={styles.pair}><Crest team={f.hT} size={26} /><Crest team={f.aT} size={26} /></span>
        <span className={styles.am}><b>{abbr(f.hT)} v {abbr(f.aT)}</b><span>{signalMarket(a)}</span></span>
        <b className={cx(styles.pos, styles.m)}>+{arbPct(a).toFixed(2)}%</b>
      </div>
      <div className={cx(styles.legs, styles.fade)}>
        {a.legs.map((l, i) => (
          <div key={i} className={styles.leg}>
            <BookLogo name={l.src} size={18} />
            <span className={styles.ln}>{l.src}</span>
            <span className={styles.lo}>{l.lab === f.h ? abbr(f.hT) : l.lab === f.a ? abbr(f.aT) : l.lab}</span>
            <b className={styles.m}>{F(l.d)}</b>
          </div>
        ))}
      </div>
      <div className={styles.dots} aria-hidden="true">
        {Array.from({ length: top }, (_, d) => <i key={d} className={d === k ? styles.on : ""} />)}
      </div>
    </div>
  )
}

function OddsCard({ ctl }: { ctl: PreviewController }) {
  const fx = ctl.oddsList()
  if (!fx.length) return <Loading />
  const tab = ctl.ui.tab < fx.length ? ctl.ui.tab : 0
  const f = fx[tab], b = mainBoard(f), ss = outcomes(f), code = mainMarket(f.sp)
  const top = (
    <>
      <div className={styles.tabs} role="tablist" aria-label="Fixtures">
        {fx.map((x, k) => (
          <button key={x.id} role="tab" data-fk={"tab:" + k} aria-selected={k === tab} onClick={() => ctl.setTab(k)}>
            <LeagueMark code={x.lg} size={14} /><span>{abbr(x.hT)} v {abbr(x.aT)}</span>
          </button>
        ))}
      </div>
      <div className={styles.mh}>
        <span className={styles.mt}><Crest team={f.hT} size={22} /><b>{f.h}</b><span className={styles.faint}>v</span><Crest team={f.aT} size={22} /><b>{f.a}</b></span>
        <span className={styles.mi}><span>{leagueName(f.lg)}</span><span className={styles.sep}>·</span><Countdown f={f} /></span>
      </div>
    </>
  )
  if (!b) return <div className={styles.od}>{top}<p className={styles.empty}>Loading every book</p></div>
  const rows = S.sources
    .map(({ name }) => {
      const cells = b.selections.map((s) => s.cells[name])
      return { n: name, cells, vals: cells.map((c) => (c && !c.price.suspended ? decOf(c.price) : null)), mg: 0 }
    })
    .filter((r) => isOn(r.n) && r.vals.every((v) => v))
  rows.forEach((r) => { r.mg = (r.cells.reduce((a, c) => a + c.price.prob, 0) - 1) * 100 })
  rows.sort((a, z) => a.mg - z.mg)
  const shown = rows.slice(0, 5)
  const best = ss.map((_, i) => Math.max(...shown.map((r) => r.vals[i] as number)))
  return (
    <div className={styles.od}>
      {top}
      <div className={styles.grid} style={{ "--n": ss.length } as CSSProperties}>
        <span className={cx(styles.gc, styles.gh, styles.gb)}>Bookmaker</span>
        {ss.map((s, i) => {
          const c = b.selections[i] && b.selections[i].consensus
          return (
            <span key={"h" + i} className={cx(styles.gc, styles.gh)}>
              <span className={styles.oh}>{s.team ? <><Crest team={s.team} size={16} /><b>{abbr(s.team)}</b></> : <b>{s.label}</b>}</span>
              <i className={styles.m}>{c ? pct(c.price.prob, 0) : ""}</i>
            </span>
          )
        })}
        <span className={cx(styles.gc, styles.gh, styles.mg)}>Margin</span>
        {shown.map((r) => [
          <span key={r.n} className={cx(styles.gc, styles.gb)}><BookLogo name={r.n} size={20} /><span className={styles.bn}>{r.n}</span></span>,
          ...r.vals.map((v, i) => {
            const fl = flash(b.keyString + "#" + i + "#" + r.n), isBest = Math.abs((v as number) - best[i]) < 1e-9, on = inSlip(f.id, b.keyString, i, r.n)
            return (
              <button
                key={r.n + i}
                className={cx(styles.gc, styles.p, isBest && styles.best, on && styles.on, fl?.c)}
                style={fl?.s}
                data-fk={"pick:" + b.keyString + "#" + i + "#" + r.n}
                aria-pressed={on}
                aria-label={ss[i].label + " at " + F(v) + " on " + r.n}
                onClick={() => ctl.pickPrice("odds", { fx: f.id, key: b.keyString, i, code, label: ss[i].label, src: r.n })}
              >
                <span className={styles.m}>{F(v)}</span>
              </button>
            )
          }),
          <span key={r.n + "mg"} className={cx(styles.gc, styles.mg, styles.m)}>{r.mg.toFixed(1)}%</span>,
        ])}
      </div>
      <Note ctl={ctl} kind="odds" hint="Lowest margin first. Best price in each column in blue." />
    </div>
  )
}

function ArbBody({ a, bank: raw }: { a: SignalItem; bank: string }) {
  const bank = Math.max(0, parseFloat(raw) || 0), c = arbStakes(a, bank, 10), same = c.hi - c.lo < 0.01, f = a.f
  return (
    <>
      <div className={styles.ch}><span>Book</span><span>Outcome</span><span>Stake</span></div>
      {c.legs.map((l, i) => {
        const team = l.lab === f.h ? f.hT : l.lab === f.a ? f.aT : null
        return (
          <div key={i} className={styles.cr}>
            <span className={styles.cb}><BookLogo name={l.src} size={22} /><span>{l.src}</span></span>
            <span className={styles.co}>{team ? <Crest team={team} size={16} /> : null}<span>{l.lab}</span><i className={styles.m}>{F(l.d)}</i></span>
            <b className={cx(styles.cs, styles.m)}>{money(l.stake)}</b>
          </div>
        )
      })}
      <div className={styles.cp}>
        <span>Profit<em>Total staked <span className={styles.m}>{money(c.total)}</span></em></span>
        <b className={cx(styles.m, c.profitLo >= 0 ? styles.pos : styles.neg)}>
          {same ? signedMoney(c.profitLo) : <>{signedMoney(c.profitLo)}<i> to </i>{signedMoney(c.profitHi)}</>}
        </b>
      </div>
      {c.profitLo < 0 && bank > 0 ? <p className={styles.warn}>Rounding to 10 is eating the edge at this stake. Raise it a little.</p> : null}
    </>
  )
}

const QUICK = [["10000", "10k"], ["50000", "50k"], ["100000", "100k"]]

function ArbCard({ ctl }: { ctl: PreviewController }) {
  const arbs = arbsSorted(), n = arbs.length, ui = ctl.ui
  if (!n) return <p className={styles.empty}>{S.boot === "ok" ? "No arbs open right now" : "Loading live prices"}</p>
  const show = Math.min(3, n), slots = show + 1
  const at = wrap(ui.arb, slots)
  const nav = (
    <div className={styles.nav}>
      <button data-fk="arb:-1" aria-label="Previous arb" onClick={() => ctl.stepArb(-1)}><ChevronLeftIcon /></button>
      <span className={styles.m}>{at < show ? at + 1 + " of " + show : "All"}</span>
      <button data-fk="arb:1" aria-label="Next arb" onClick={() => ctl.stepArb(1)}><ChevronRightIcon /></button>
      <em className={styles.open}><span className={styles.m}>{n}</span> open now</em>
    </div>
  )
  if (at === show) {
    return (
      <div className={cx(styles.ar, styles.arc)}>
        {nav}
        <div className={styles.cta}>
          <b className={styles.num}>{n}</b>
          <p>arbs open right now across every sport. These were the top three.</p>
          <button className={styles.go} onClick={() => ctl.go(routes.signals("arb"))}>See them all in the app<ExternalIcon /></button>
        </div>
      </div>
    )
  }
  const a = arbs[at], f = a.f
  return (
    <div className={styles.ar}>
      <div className={styles.ah}>
        <span className={styles.pair}><Crest team={f.hT} size={28} /><Crest team={f.aT} size={28} /></span>
        <span className={styles.am}><b>{f.h} v {f.a}</b><span>{signalMarket(a)}<span className={styles.sep}>·</span><Countdown f={f} /></span></span>
        <span className={styles.ap}><b className={cx(styles.pos, styles.m)}>+{arbPct(a).toFixed(2)}%</b><span>profit</span></span>
      </div>
      {nav}
      <div className={styles.st}>
        <label>
          <span>Total stake</span>
          <input data-bank="" data-fk="bank" type="number" inputMode="decimal" min="0" step="any" value={ui.bank} aria-label="Total stake" onChange={(e) => ctl.setBank(e.target.value)} />
        </label>
        <div className={styles.qs} role="group" aria-label="Quick stake">
          {QUICK.map(([v, label]) => (
            <button key={v} data-fk={"bank:" + v} aria-pressed={ui.bank === v} onClick={() => ctl.setBank(v)}>{label}</button>
          ))}
        </div>
      </div>
      <div aria-live="polite"><ArbBody a={a} bank={ui.bank} /></div>
    </div>
  )
}

function ValueCard({ ctl }: { ctl: PreviewController }) {
  const evs = evsSorted().slice(0, 4)
  if (!evs.length) return <p className={styles.empty}>{S.boot === "ok" ? "No value bets right now" : "Loading live prices"}</p>
  return (
    <div>
      <div className={styles.vh}><span>Value bets</span><span>Edge</span></div>
      {evs.map((e) => {
        const f = e.f, l = e.legs[0], open = ctl.ui.ev === e.id, imp = l.d ? 1 / l.d : 0, fair = e.fair
        return (
          <div key={e.id} className={cx(styles.vr, open && styles.open)}>
            <button className={styles.vb} data-fk={"ev:" + e.id} aria-expanded={open} onClick={() => ctl.toggleEv(e.id)}>
              <span className={styles.pair}><Crest team={f.hT} size={22} /><Crest team={f.aT} size={22} /></span>
              <span className={styles.vp}><b>{l.lab} · {signalMarket(e)}</b><span>{f.h} v {f.a}</span></span>
              <span className={styles.vk}><BookLogo name={l.src} size={18} /><span className={styles.m}>{F(l.d)}</span></span>
              <b className={cx(styles.ve, styles.pos, styles.m)}>+{pct(e.ev)}</b>
              <span className={styles.chev} aria-hidden="true"><ChevronDownIcon /></span>
            </button>
            {open ? (
              <div className={styles.vx}>
                <div className={styles.bar}><span>{l.src} implies</span><span className={styles.m}>{pct(imp)}</span><i><b style={{ width: (imp * 100).toFixed(1) + "%" }} /></i></div>
                <div className={cx(styles.bar, styles.barF)}><span>Fair chance</span><span className={styles.m}>{pct(fair)}</span><i><b style={{ width: (fair * 100).toFixed(1) + "%" }} /></i></div>
                <p className={styles.vn}>
                  <span>Fair price <span className={styles.m}>{fair ? F(1 / fair) : "–"}</span></span>
                  <Link href={routes.event(f.id)}>Open in app<ExternalIcon /></Link>
                </p>
              </div>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}

const RENDER: Record<PreviewKind, (ctl: PreviewController) => ReactNode> = {
  "hero-books": () => <HeroBooks />,
  "hero-odds": (ctl) => <HeroOdds ctl={ctl} />,
  "hero-arbs": (ctl) => <HeroArbs ctl={ctl} />,
  odds: (ctl) => <OddsCard ctl={ctl} />,
  arb: (ctl) => <ArbCard ctl={ctl} />,
  value: (ctl) => <ValueCard ctl={ctl} />,
}

const KIND_CLASS: Record<PreviewKind, string | undefined> = {
  "hero-books": styles.kHeroBooks,
  "hero-odds": undefined,
  "hero-arbs": undefined,
  odds: styles.kOdds,
  arb: undefined,
  value: undefined,
}

/**
 * One live preview. `n` counts its draws: each new value remounts the content,
 * and focus returns to the control that had it. Nothing renders before the
 * first draw, so the slot shows its skeleton until then.
 */
export const Preview = memo(function Preview({ kind, n, ctl }: { kind: PreviewKind; n: number; rev?: number; ctl: PreviewController }) {
  const ref = useRef<HTMLDivElement>(null)
  const focused = useRef<string | null>(null)

  useLayoutEffect(() => {
    const key = focused.current, el = ref.current
    if (!key || !el) return
    const active = document.activeElement
    if (active && active !== document.body) return
    const target = [...el.querySelectorAll<HTMLElement>("[data-fk]")].find((x) => x.dataset.fk === key)
    target?.focus({ preventScroll: true })
  }, [n])

  if (!n) return null

  const onFocus = (e: FocusEvent<HTMLDivElement>) => { focused.current = (e.target as HTMLElement).dataset.fk || null }
  const onBlur = (e: FocusEvent<HTMLDivElement>) => {
    const target = e.target as HTMLElement, next = e.relatedTarget as Node | null
    if (next) { if (!ref.current?.contains(next)) focused.current = null; return }
    setTimeout(() => { if (target.isConnected && document.activeElement !== target) focused.current = null }, 0)
  }

  return (
    <div ref={ref} key={n} className={cx(styles.preview, KIND_CLASS[kind])} onFocus={onFocus} onBlur={onBlur}>
      <div className={styles.pad}>{RENDER[kind](ctl)}</div>
    </div>
  )
})
