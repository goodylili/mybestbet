"use client"

import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from "react"
import { COMPS, banner, compArt, photo } from "@/lib/catalog"
import { routes } from "@/lib/routes"
import { useStore } from "@/lib/store/emitter"
import { LeagueMark } from "@/components/mybestbet/shared/logos"
import { isoSvg, type IsoIcon } from "./iso-icons"
import { Count, SourceNames, counts, type CountKind } from "./live-text"
import { Preview } from "./previews"
import { bg, reveal, words } from "./reveal"
import type { PreviewKind, usePreviews } from "./use-previews"
import styles from "./landing.module.css"

type Previews = ReturnType<typeof usePreviews>
type Go = (href: string) => void

const cx = (...names: (string | false | undefined)[]) => names.filter(Boolean).join(" ")
const easeOut = (x: number) => 1 - Math.pow(1 - x, 3)

/** Photo for a section: its own picture, else the competition banner. */
const sectionPhoto = (name: string, code?: string) => photo(name) || banner(code) || (code === "UWCL" ? banner("CHAMP") : null)

const STATS: [CountKind, string][] = [
  ["books", "Books and markets compared"],
  ["comps", "Competitions covered"],
  ["arbs", "Arbs open right now"],
]

/** Three big numbers that count up from zero the first time they scroll into view. */
export function Stats() {
  useStore()
  const ref = useRef<HTMLElement>(null)
  const [roll, setRoll] = useState<{ from: Record<CountKind, number>; k: number } | null>(null)

  useEffect(() => {
    const el = ref.current
    if (!el) return
    let raf = 0, done = false
    const start = () => {
      if (done) return
      done = true
      const from = counts()
      if (matchMedia("(prefers-reduced-motion: reduce)").matches) { setRoll({ from, k: 1 }); return }
      const t0 = performance.now(), dur = 1600
      const tick = (now: number) => {
        const k = easeOut(Math.min(1, Math.max(0, (now - t0) / dur)))
        setRoll({ from, k })
        if (k < 1) raf = requestAnimationFrame(tick)
      }
      tick(t0)
    }
    if (!("IntersectionObserver" in window) || matchMedia("(prefers-reduced-motion: reduce)").matches) {
      start()
      return () => cancelAnimationFrame(raf)
    }
    const io = new IntersectionObserver((es) => {
      if (es.some((en) => en.isIntersecting)) { start(); io.disconnect() }
    }, { threshold: 0.4 })
    io.observe(el)
    return () => { io.disconnect(); cancelAnimationFrame(raf) }
  }, [])

  const live = counts()
  const value = (kind: CountKind) => (!roll ? 0 : roll.k < 1 ? Math.round(roll.from[kind] * roll.k) : live[kind])

  return (
    <section ref={ref} className={cx(styles.stats, styles.wrap)} aria-label="mybestbet in numbers">
      {STATS.map(([kind, label], i) => (
        <div key={kind} className={styles.stat} {...reveal(i)}>
          <span className={styles.statN}>{value(kind)}</span>
          <span className={styles.statL}>{label}</span>
        </div>
      ))}
    </section>
  )
}

/** Photo with a live preview card on it, beside a heading, copy, three checks and a button. */
export function Feature({ id, labelId, code, photoName, kind, title, body, checks, cta, flip, previews, onGo }: {
  id: string
  labelId: string
  code: string
  photoName: string
  kind: PreviewKind
  title: string
  body: string
  checks: string[]
  cta: string
  flip?: boolean
  previews: Previews
  onGo: Go
}) {
  return (
    <section className={cx(styles.feat, flip && styles.featRev, styles.wrap)} id={id} aria-labelledby={labelId}>
      <div className={styles.photo} data-banner={code}>
        <div className={styles.photoBg} style={bg(sectionPhoto(photoName, code))} />
        <div className={styles.fc}>
          <div className={styles.slot}>
            <Preview kind={kind} n={previews.draws[kind]} rev={kind === "arb" ? previews.rev : undefined} ctl={previews.ctl} />
          </div>
        </div>
      </div>
      <div className={styles.copy}>
        <h2 className={styles.h2} id={labelId} data-words="">{words(title)}</h2>
        <p className={styles.body} {...reveal(0)}>{body}</p>
        <ul className={styles.checks}>
          {checks.map((c, i) => <li key={c} {...reveal(i)}>{c}</li>)}
        </ul>
        <button className={cx(styles.btn, styles.btnSoft)} onClick={() => onGo(routes.sport("football"))} {...reveal(1)}>
          {cta} <span className={styles.arr} aria-hidden="true">↗</span>
        </button>
      </div>
    </section>
  )
}

const STEPS: { icon: IsoIcon; title: string; body: ReactNode; tags: string[]; photo: string }[] = [
  {
    icon: "radar",
    title: "We watch the books",
    body: <>Prices from <span><Count kind="books" /></span> sportsbooks and prediction markets stream in all day and all night, refreshed every few seconds.</>,
    tags: ["16 feeds", "Every 5s near kick-off"],
    photo: "watch",
  },
  {
    icon: "bars",
    title: "We line them up",
    body: "Each market is matched across books, so the same outcome sits in one row and the best number is obvious at a glance.",
    tags: ["1X2, totals, handicaps", "Lowest margin first"],
    photo: "lanes",
  },
  {
    icon: "bolt",
    title: "We flag what matters",
    body: "Best prices, arbs and value bets are marked the moment they appear and cleared the moment they close.",
    tags: ["Arbs and value bets", "Telegram alerts"],
    photo: "card",
  },
  {
    icon: "coins",
    title: "You place the bet",
    body: "Tap through to the book and bet on its own site, with your own account and your own money. We never hold funds.",
    tags: ["Stakes worked out", "Your account, your call"],
    photo: "phone",
  },
]

/** Sticky heading beside four cards that stack as you scroll. */
export function How() {
  return (
    <section className={cx(styles.how, styles.wrap)} aria-labelledby="lp-how-h">
      <div className={styles.secHd}>
        <h2 className={styles.h2} id="lp-how-h" data-words="">{words("From a price on the board to a bet you placed, in four steps.")}</h2>
      </div>
      <ol className={styles.stack}>
        {STEPS.map((s, i) => (
          <li key={s.title} className={styles.sc} style={{ "--i": i } as CSSProperties}>
            <div className={styles.scIn}>
              <div className={styles.scTx}>
                <div className={styles.scTop}><span className={styles.iso} aria-hidden="true" dangerouslySetInnerHTML={{ __html: isoSvg(s.icon) }} /></div>
                <h3>{s.title}</h3>
                <p>{s.body}</p>
                <ul className={styles.scTags}>{s.tags.map((t) => <li key={t}>{t}</li>)}</ul>
              </div>
              <div className={styles.scImg}><div className={styles.photoBg} style={bg(photo(s.photo))} /></div>
            </div>
          </li>
        ))}
      </ol>
    </section>
  )
}

const MARQUEE = COMPS.filter((c) => compArt(c.id))

/** Every competition's mark scrolling past, paused while hovered or off screen. */
export function Coverage() {
  const marq = useRef<HTMLDivElement>(null)
  const track = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = marq.current, t = track.current
    if (!el || !t) return
    const raf = requestAnimationFrame(() => {
      const g = t.firstElementChild as HTMLElement | null
      if (g && g.offsetWidth) t.style.animationDuration = Math.round(g.offsetWidth / 45) + "s"
    })
    if (!("IntersectionObserver" in window)) return () => cancelAnimationFrame(raf)
    const mq = matchMedia("(prefers-reduced-motion: reduce)")
    const io = new IntersectionObserver((es) => es.forEach((en) => el.toggleAttribute("data-on", en.isIntersecting && !mq.matches)))
    io.observe(el)
    const onChange = (e: MediaQueryListEvent) => { if (e.matches) el.removeAttribute("data-on") }
    mq.addEventListener("change", onChange)
    return () => { cancelAnimationFrame(raf); io.disconnect(); mq.removeEventListener("change", onChange) }
  }, [])

  const items = MARQUEE.map((c) => (
    <span key={c.id} className={styles.cv}><LeagueMark code={c.id} size={28} /><span>{c.name}</span></span>
  ))
  return (
    <section className={styles.cov} aria-labelledby="lp-cov-h">
      <div className={cx(styles.wrap, styles.covHd)}>
        <h2 className={styles.h2} id="lp-cov-h" data-words="">{words("Football, basketball, tennis, esports and more.")}</h2>
      </div>
      <div ref={marq} className={styles.marq} {...reveal(0)}>
        <div ref={track} className={styles.marqT}>
          <div className={styles.marqG}>{items}</div>
          <div className={styles.marqG} aria-hidden="true">{items}</div>
        </div>
      </div>
    </section>
  )
}

const QUESTIONS: [string, ReactNode][] = [
  ["What is an arb?", "Short for arbitrage. It happens when books price the same event so differently that backing every outcome, each at a different book, pays back more than your total stake. Whatever the result, you finish ahead. Most are small, around 1 to 3 percent, and they close quickly, so speed matters."],
  ["How often do prices refresh?", "Every few seconds while a market is open, and under a second for sources that stream. When a price moves, its cell flashes so you can see what changed. If a book stops responding, we mark it as down and leave its prices out of arbs and value bets."],
  ["Which books do you cover?", <><span><Count kind="books" /></span> sources in total, a mix of sportsbooks and prediction markets<span><SourceNames /></span>. The Sources page in the app shows each one with its live status.</>],
  ["Does mybestbet place bets for me?", "No. We only compare prices. When you find a bet you like, you place it yourself on the book's own site, with your own account. We never hold your money or log in anywhere for you."],
  ["How do Telegram alerts work?", "Link your Telegram in settings and pick what you care about: arbs above a margin you set, value bets above an edge you set, or only certain leagues. When one appears, you get a short message with the prices, the books and a link straight to it."],
]

/** One-at-a-time accordion. The first answer starts open. */
export function Faq() {
  const [open, setOpen] = useState(0)
  return (
    <section className={cx(styles.faq, styles.wrap)} id="faq" aria-labelledby="lp-faq-h">
      <div className={styles.faqHd}>
        <h2 className={styles.h2} id="lp-faq-h" data-words="">{words("Questions, answered plainly.")}</h2>
      </div>
      <div className={styles.acc}>
        {QUESTIONS.map(([q, a], i) => {
          const n = i + 1, isOpen = open === i
          return (
            <div key={n} className={cx(styles.q, isOpen && styles.isOpen)} {...reveal(i)}>
              <button className={styles.qB} aria-expanded={isOpen} aria-controls={"lp-a" + n} id={"lp-q" + n} onClick={() => setOpen(isOpen ? -1 : i)}>
                {q}<span className={styles.pm} aria-hidden="true" />
              </button>
              <div className={styles.a} id={"lp-a" + n} role="region" aria-labelledby={"lp-q" + n}><div><p>{a}</p></div></div>
            </div>
          )
        })}
      </div>
    </section>
  )
}

/** Closing photo panel with the last call to open the app. */
export function Cta({ onGo }: { onGo: Go }) {
  return (
    <section className={cx(styles.cta, styles.wrap)} aria-labelledby="lp-cta-h">
      <div className={styles.ctaP} data-banner="UWCL">
        <div className={styles.photoBg} style={bg(sectionPhoto("cta", "UWCL"))} />
        <div className={styles.ctaIn}>
          <h2 className={styles.ctaH} id="lp-cta-h" data-words="">{words("Check the price before you bet.")}</h2>
          <p {...reveal(0)}>Open the app and see what every book is offering today.</p>
          <button className={cx(styles.btn, styles.btnInv)} onClick={() => onGo(routes.sport("football"))} {...reveal(1)}>
            Open app <span className={styles.arr} aria-hidden="true">↗</span>
          </button>
        </div>
      </div>
    </section>
  )
}
