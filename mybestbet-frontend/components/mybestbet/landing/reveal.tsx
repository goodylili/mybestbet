import type { CSSProperties, ReactNode } from "react"
import styles from "./landing.module.css"

/** Props for an element that fades up when scrolled into view, staggered 60ms per sibling. */
export const reveal = (index = 0) => ({ "data-reveal": "", style: { "--lp-d": index * 60 + "ms" } as CSSProperties })

/** Inline background for a photo layer, or nothing when the image is missing. */
export const bg = (src: string | null | undefined): CSSProperties | undefined => (src ? { backgroundImage: 'url("' + src + '")' } : undefined)

/** Split a heading into word spans that blur in one after another. "\n" becomes a line break. */
export function words(text: string): ReactNode[] {
  let i = 0
  return text.split("\n").flatMap((line, li) => {
    const parts = line.split(/( +)/).filter(Boolean).map((part, k) =>
      /^ +$/.test(part) ? " " : (
        <span key={li + "-" + k} className={styles.w} style={{ "--lp-d": i++ * 45 + "ms" } as CSSProperties}>
          {part}
        </span>
      ),
    )
    return li ? [<br key={"br" + li} />, ...parts] : parts
  })
}
