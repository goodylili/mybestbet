"use client"

import { useEffect, type RefObject } from "react"
import styles from "./landing.module.css"

const easeOut = (x: number) => 1 - Math.pow(1 - x, 3)
const clamp = (v: number, a: number, b: number) => Math.min(b, Math.max(a, v))
const REDUCE = "(prefers-reduced-motion: reduce)"
const reducedNow = () => matchMedia(REDUCE).matches
const find = (el: ParentNode, cls: string) => el.querySelector<HTMLElement>("." + cls)
const findAll = (el: ParentNode, sel: string) => [...el.querySelectorAll<HTMLElement>(sel)]

type Root = RefObject<HTMLElement | null>

/** Fade elements up as they scroll into view, and slide the footer wordmark up. */
export function useReveal(root: Root) {
  useEffect(() => {
    const el = root.current
    if (!el) return
    const show = (x: Element) => x.setAttribute("data-in", "")
    const items = findAll(el, "[data-reveal],[data-words]"), big = find(el, styles.big)
    if (!("IntersectionObserver" in window) || reducedNow()) {
      items.forEach(show)
      if (big) show(big)
      return
    }
    const io = new IntersectionObserver((es) => es.forEach((en) => {
      if (en.isIntersecting) { show(en.target); io.unobserve(en.target) }
    }), { rootMargin: "0px 0px -8% 0px", threshold: 0.1 })
    items.forEach((x) => { if (!x.hasAttribute("data-in")) io.observe(x) })
    const ioBig = new IntersectionObserver((es) => es.forEach((en) => {
      if (en.isIntersecting) { show(en.target); ioBig.unobserve(en.target) }
    }), { threshold: 0.35 })
    if (big) ioBig.observe(big)
    return () => { io.disconnect(); ioBig.disconnect() }
  }, [root])
}

/**
 * The nav's scrolled state, the current section in the notch, and the
 * parallax drift and settle of the feature photos.
 */
export function useScrollFrame(root: Root, menuOpen: RefObject<boolean>) {
  useEffect(() => {
    const el = root.current, nav = el && find(el, styles.nav)
    if (!el || !nav) return
    const mq = matchMedia(REDUCE), visible = new Set<HTMLElement>()
    const photos = findAll(el, "[data-banner]")
    let ticking = false, lastY = 0, raf = 0

    const frame = () => {
      ticking = false
      const y = window.scrollY || 0
      nav.toggleAttribute("data-scrolled", y > 8)
      if (!menuOpen.current && (y < lastY - 4 || y < 160)) nav.removeAttribute("data-hide")
      lastY = y
      if (mq.matches) return
      const vh = window.innerHeight
      visible.forEach((p) => {
        const r = p.getBoundingClientRect(), bg = find(p, styles.photoBg)
        if (!bg) return
        const pos = clamp((r.top + r.height / 2 - vh / 2) / (vh / 2 + r.height / 2), -1, 1)
        const e = easeOut(clamp((vh - r.top) / (vh * 0.85), 0, 1))
        bg.style.setProperty("--lp-py", (-pos * r.height * 0.09).toFixed(1) + "px")
        bg.style.setProperty("--lp-ps", (1.08 - 0.08 * e).toFixed(4))
      })
    }
    const onScroll = () => { if (!ticking) { ticking = true; raf = requestAnimationFrame(frame) } }

    window.scrollTo(0, 0)
    nav.removeAttribute("data-hide")
    window.addEventListener("scroll", onScroll, { passive: true })
    window.addEventListener("resize", onScroll, { passive: true })

    const ioPhoto = new IntersectionObserver((es) => {
      es.forEach((en) => { if (en.isIntersecting) visible.add(en.target as HTMLElement); else visible.delete(en.target as HTMLElement) })
      onScroll()
    }, { rootMargin: "20% 0px 20% 0px" })
    photos.forEach((p) => ioPhoto.observe(p))

    const links = findAll(nav, "." + styles.links + " [data-to]")
    const ioSec = new IntersectionObserver((es) => es.forEach((en) => {
      if (!en.isIntersecting) return
      links.forEach((l) => l.toggleAttribute("data-cur", l.dataset.to === en.target.id))
    }), { rootMargin: "-45% 0px -50% 0px" })
    ;["odds", "arbitrage", "value", "faq"].forEach((id) => { const s = document.getElementById(id); if (s) ioSec.observe(s) })

    onScroll()
    return () => {
      window.removeEventListener("scroll", onScroll)
      window.removeEventListener("resize", onScroll)
      cancelAnimationFrame(raf)
      ioPhoto.disconnect()
      ioSec.disconnect()
    }
  }, [root, menuOpen])
}

/**
 * The full-bleed hero photo pinches into a rounded tab shape as you scroll
 * away, and the nav and headline go white while they sit on it.
 */
export function useHeroMorph(root: Root) {
  useEffect(() => {
    const el = root.current
    if (!el) return
    const ph = find(el, styles.heroPh), hero = find(el, styles.hero), nav = find(el, styles.nav)
    if (!ph || !hero) return
    const reduced = reducedNow()
    let raf = 0

    const frame = () => {
      raf = 0
      const W = ph.offsetWidth, H = ph.offsetHeight, span = Math.max(320, Math.min(H, innerHeight) * 0.95)
      let p = Math.min(1, Math.max(0, scrollY / span))
      if (reduced) p = scrollY > 40 ? 1 : 0
      const t = p < 0.5 ? 2 * p * p : 1 - Math.pow(-2 * p + 2, 2) / 2
      const a = W * 0.05 * t, b = W * 0.44 * t, yT = H * 0.04 * t, yB = H * (1 - 0.42 * t), m = (yB - yT) / 2
      const r = 24 * Math.min(1, t * 4)
      const d = "M" + (a + r) + " " + yT + " L" + (W - a - r) + " " + yT + " Q" + (W - a) + " " + yT + " " + (W - a) + " " + (yT + r) +
        " C" + (W - a) + " " + (yT + m) + " " + (W - b) + " " + (yB - m) + " " + (W - b) + " " + yB +
        " L" + b + " " + yB + " C" + b + " " + (yB - m) + " " + a + " " + (yT + m) + " " + a + " " + (yT + r) +
        " Q" + a + " " + yT + " " + (a + r) + " " + yT + " Z"
      ph.style.clipPath = p <= 0 ? "none" : "path('" + d + "')"
      ph.style.opacity = String(1 - Math.max(0, (p - 0.72) / 0.28))
      hero.style.setProperty("--lp-hp", p.toFixed(3))
      hero.toggleAttribute("data-onph", p < 0.3)
      if (nav) nav.toggleAttribute("data-onph", p < 0.22 && scrollY < H * 0.5)
    }
    const req = () => { if (!raf) raf = requestAnimationFrame(frame) }

    addEventListener("scroll", req, { passive: true })
    addEventListener("resize", req)
    const timer = setInterval(req, 500)
    req()
    return () => {
      removeEventListener("scroll", req)
      removeEventListener("resize", req)
      clearInterval(timer)
      cancelAnimationFrame(raf)
    }
  }, [root])
}

/** The "how it works" cards fly in from a tilt and shrink back as the next one stacks on top. */
export function useStackCards(root: Root) {
  useEffect(() => {
    const el = root.current
    if (!el) return
    const reduced = reducedNow()
    let raf = 0

    const frame = () => {
      raf = 0
      if (reduced) return
      const cards = findAll(el, "." + styles.sc), vh = innerHeight
      cards.forEach((li, i) => {
        const inner = li.firstElementChild as HTMLElement | null
        if (!inner) return
        const r = li.getBoundingClientRect(), top = parseFloat(getComputedStyle(li).top) || 0
        const enter = Math.min(1, Math.max(0, (vh - r.top) / (vh - top)))
        const e = 1 - Math.pow(1 - enter, 3)
        let covered = 0
        for (let j = i + 1; j < cards.length; j++) {
          const rj = cards[j].getBoundingClientRect(), tj = parseFloat(getComputedStyle(cards[j]).top) || 0
          covered += Math.min(1, Math.max(0, (vh - rj.top) / (vh - tj)))
        }
        const k = 1 - e, side = i % 2 ? -1 : 1
        inner.style.setProperty("--lp-rx", (k * 58).toFixed(2) + "deg")
        inner.style.setProperty("--lp-ry", (k * -28 * side).toFixed(2) + "deg")
        inner.style.setProperty("--lp-rz", (k * 9 * side).toFixed(2) + "deg")
        inner.style.setProperty("--lp-tx", (k * 38 * side).toFixed(2) + "%")
        inner.style.setProperty("--lp-tz", (k * -520).toFixed(1) + "px")
        inner.style.setProperty("--lp-ty", (k * 220).toFixed(1) + "px")
        inner.style.setProperty("--lp-op", Math.min(1, e * 1.6).toFixed(3))
        inner.style.setProperty("--lp-sc", (1 - covered * 0.045).toFixed(4))
        inner.style.setProperty("--lp-br", (1 - covered * 0.07).toFixed(3))
        const img = find(li, styles.scImg)
        if (img) img.style.setProperty("--lp-is", (1.14 - e * 0.14).toFixed(3))
      })
    }
    const req = () => { if (!raf) raf = requestAnimationFrame(frame) }

    addEventListener("scroll", req, { passive: true })
    addEventListener("resize", req)
    const start = setTimeout(req, 50)
    const timer = setInterval(req, 800)
    return () => {
      removeEventListener("scroll", req)
      removeEventListener("resize", req)
      clearTimeout(start)
      clearInterval(timer)
      cancelAnimationFrame(raf)
    }
  }, [root])
}

/**
 * The inverted bands grow from an inset rounded card to full width as they
 * scroll in, and the nav flips colour while it sits over one.
 */
export function useBands(root: Root) {
  useEffect(() => {
    const el = root.current
    if (!el) return
    let raf = 0

    const frame = () => {
      raf = 0
      const vh = innerHeight, nav = find(el, styles.nav), navB = nav ? nav.getBoundingClientRect().bottom : 60
      let onDark = false
      findAll(el, "." + styles.band).forEach((b) => {
        const r = b.getBoundingClientRect()
        let x = Math.min(1, Math.max(0, (vh - r.top) / (vh * 0.7)))
        if (reducedNow()) x = 1
        const e = 1 - Math.pow(1 - x, 3)
        b.style.setProperty("--lp-x", e.toFixed(4))
        if (r.top < navB - 10 && r.bottom > navB && e > 0.6) onDark = true
      })
      if (nav) nav.toggleAttribute("data-ondk", onDark && !nav.hasAttribute("data-onph"))
    }
    const req = () => { if (!raf) raf = requestAnimationFrame(frame) }

    addEventListener("scroll", req, { passive: true })
    addEventListener("resize", req)
    const start = setTimeout(req, 60)
    const timer = setInterval(req, 700)
    return () => {
      removeEventListener("scroll", req)
      removeEventListener("resize", req)
      clearTimeout(start)
      clearInterval(timer)
      cancelAnimationFrame(raf)
    }
  }, [root])
}

/** Crests circle the hero cards on a flat ellipse, larger and brighter at the front. */
export function useOrbit(ring: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const t0 = performance.now(), reduced = reducedNow()
    let raf = 0
    const frame = (now: number) => {
      raf = requestAnimationFrame(frame)
      const el = ring.current, inner = el && find(el, styles.ringIn)
      if (!el || !inner) return
      const box = el.getBoundingClientRect()
      if (box.bottom < 0 || box.top > innerHeight) return
      const items = inner.children, n = items.length
      if (!n) return
      const W = el.offsetWidth, H = el.offsetHeight, mobile = W < 700
      const cx = W / 2, cy = mobile ? 210 : H * 0.43, rx = mobile ? W * 0.47 : Math.min(W * 0.47, 640), ry = mobile ? 70 : Math.min(H * 0.3, 250)
      const a0 = reduced ? 0 : ((now - t0) / 1000) * ((2 * Math.PI) / 90)
      for (let i = 0; i < n; i++) {
        const a = a0 + (i / n) * 2 * Math.PI, sx = Math.cos(a), sy = Math.sin(a)
        const depth = (sy + 1) / 2, item = items[i] as HTMLElement, size = item.offsetWidth || 52
        item.style.setProperty("--x", (cx + sx * rx - size / 2).toFixed(1) + "px")
        item.style.setProperty("--y", (cy + sy * ry - size / 2).toFixed(1) + "px")
        item.style.setProperty("--s", (0.62 + depth * 0.48).toFixed(3))
        item.style.setProperty("--o", (0.35 + depth * 0.65).toFixed(3))
        item.style.setProperty("--z", String(Math.round(depth * 100)))
      }
    }
    raf = requestAnimationFrame(frame)
    return () => cancelAnimationFrame(raf)
  }, [ring])
}

/** On wide screens, size the headline so it spans from the middle of the left hero card to the middle of the right one. */
export function useFitHeading(heading: RefObject<HTMLElement | null>, left: RefObject<HTMLElement | null>, right: RefObject<HTMLElement | null>) {
  useEffect(() => {
    let cancelled = false
    const fit = () => {
      const h = heading.current, l = left.current, r = right.current
      if (!h || cancelled) return
      if (!l || !r || innerWidth < 901 || l.offsetParent !== r.offsetParent) { h.style.removeProperty("font-size"); return }
      const span = r.offsetLeft + r.offsetWidth / 2 - (l.offsetLeft + l.offsetWidth / 2)
      if (span <= 0) return
      const c = h.cloneNode(true) as HTMLElement
      c.removeAttribute("id")
      c.style.cssText = "position:absolute;left:-9999px;top:0;visibility:hidden;width:max-content;max-width:none;white-space:nowrap;font-size:100px"
      c.style.setProperty("font-size", "100px", "important")
      h.parentNode?.appendChild(c)
      const w = c.getBoundingClientRect().width
      c.remove()
      if (w > 0) h.style.setProperty("font-size", ((100 * span) / w).toFixed(2) + "px", "important")
    }
    let t: ReturnType<typeof setTimeout> | undefined
    const onResize = () => { clearTimeout(t); t = setTimeout(fit, 80) }
    addEventListener("resize", onResize)
    if (document.fonts && document.fonts.ready) document.fonts.ready.then(fit)
    const a = setTimeout(fit, 0), b = setTimeout(fit, 600)
    return () => {
      cancelled = true
      removeEventListener("resize", onResize)
      clearTimeout(t)
      clearTimeout(a)
      clearTimeout(b)
    }
  }, [heading, left, right])
}
