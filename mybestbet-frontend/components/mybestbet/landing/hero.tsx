"use client"

import { useEffect, useRef, useState, type CSSProperties, type RefObject } from "react"
import { heroVideo, photo } from "@/lib/catalog"
import { Crest } from "@/components/mybestbet/shared/logos"
import { Count } from "./live-text"
import { Preview } from "./previews"
import { bg, reveal, words } from "./reveal"
import { RING_TEAMS } from "./ring-teams"
import { useFitHeading, useOrbit } from "./use-landing-motion"
import type { usePreviews } from "./use-previews"
import styles from "./landing.module.css"

type Previews = ReturnType<typeof usePreviews>

/** Plays the stadium clip behind the hero once the page has settled, when motion is allowed and the browser can decode it. */
function useHeroVideo(box: RefObject<HTMLDivElement | null>) {
  const [on, setOn] = useState(false)
  const video = useRef<HTMLVideoElement>(null)

  useEffect(() => {
    const t = setTimeout(() => {
      if (!heroVideo || matchMedia("(prefers-reduced-motion: reduce)").matches) return
      const probe = document.createElement("video")
      if (!probe.canPlayType('video/webm; codecs="av01.0.05M.08"') && !probe.canPlayType('video/webm; codecs="vp9"')) return
      setOn(true)
    }, 200)
    return () => clearTimeout(t)
  }, [])

  useEffect(() => {
    const v = video.current, el = box.current
    if (!on || !v || !el) return
    v.muted = true
    v.play().catch(() => {})
    const io = new IntersectionObserver((es) => es.forEach((e) => {
      if (e.isIntersecting) v.play().catch(() => {})
      else v.pause()
    }))
    io.observe(el)
    return () => io.disconnect()
  }, [on, box])

  const element = on && heroVideo ? (
    <video
      ref={video}
      className={styles.heroVid}
      src={heroVideo}
      muted
      loop
      autoPlay
      playsInline
      aria-hidden="true"
      onPlaying={() => box.current?.setAttribute("data-vid", "")}
      onError={() => { box.current?.removeAttribute("data-vid"); setOn(false) }}
    />
  ) : null
  return element
}

export function Hero({ previews }: { previews: Previews }) {
  const { draws, ctl } = previews
  const photoBox = useRef<HTMLDivElement>(null)
  const heading = useRef<HTMLHeadingElement>(null)
  const ring = useRef<HTMLDivElement>(null)
  const left = useRef<HTMLDivElement>(null)
  const centre = useRef<HTMLDivElement>(null)
  const right = useRef<HTMLDivElement>(null)
  const video = useHeroVideo(photoBox)

  useOrbit(ring)
  useFitHeading(heading, left, right)

  useEffect(() => {
    const frames: number[] = []
    ;[centre, left, right].forEach((r, i) => {
      const el = r.current
      if (!el) return
      el.style.setProperty("--lp-d", 250 + i * 160 + "ms")
      frames.push(requestAnimationFrame(() => frames.push(requestAnimationFrame(() => el.setAttribute("data-up", "")))))
    })
    frames.push(requestAnimationFrame(() => ring.current?.setAttribute("data-vis", "")))
    return () => frames.forEach(cancelAnimationFrame)
  }, [])

  return (
    <section className={styles.hero} aria-labelledby="lp-h1">
      <div ref={photoBox} className={styles.heroPh} aria-hidden="true">
        <div className={styles.photoBg} style={bg(photo("hero"))} />
        {video}
      </div>
      <div className={styles.heroGlow} aria-hidden="true" />
      <div className={styles.wrap}>
        <p className={styles.eyebrow} {...reveal(0)}>Live prices from <span><Count kind="books" /></span> sportsbooks and prediction markets</p>
        <h1 ref={heading} className={styles.h1} id="lp-h1" data-words="">{words("The Best Price\nOn Every Bet")}</h1>
      </div>
      <div className={styles.cluster}>
        <div ref={ring} className={styles.ring} aria-hidden="true">
          <div className={styles.ringIn} style={{ "--step": 360 / RING_TEAMS.length + "deg" } as CSSProperties}>
            {RING_TEAMS.map((name, i) => (
              <span key={name} style={{ "--i": i } as CSSProperties}><Crest team={name} size={40} /></span>
            ))}
          </div>
        </div>
        <div ref={left} className={styles.hc + " " + styles.hcL}>
          <div className={styles.slot}><Preview kind="hero-books" n={draws["hero-books"]} ctl={ctl} /></div>
        </div>
        <div ref={centre} className={styles.hc + " " + styles.hcC}>
          <div className={styles.slot}><Preview kind="hero-odds" n={draws["hero-odds"]} ctl={ctl} /></div>
        </div>
        <div ref={right} className={styles.hc + " " + styles.hcR}>
          <div className={styles.slot}><Preview kind="hero-arbs" n={draws["hero-arbs"]} ctl={ctl} /></div>
        </div>
      </div>
      <p className={styles.cap} {...reveal(0)}><span className={styles.dot} aria-hidden="true" />A live look inside the demo app. Odds are illustrative.</p>
    </section>
  )
}
