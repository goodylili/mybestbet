import Image from "next/image"
import Link from "next/link"
import type { ReactNode } from "react"
import { MybestbetWordmark } from "@/components/mybestbet-wordmark"
import photo from "@/public/mybestbet/photos/auth.jpg"
import styles from "./auth.module.css"

export function AuthFrame({ children }: { children: ReactNode }) {
  return (
    <div className={styles.page}>
      <aside className={styles.panel} aria-hidden="true">
        <Image className={styles.photo} src={photo} alt="" fill priority sizes="50vw" placeholder="blur" />
      </aside>
      <main className={styles.side}>
        <div className={styles.col}>
          <Link className={styles.brand} href="/" aria-label="mybestbet home">
            <MybestbetWordmark className={styles.wm} />
          </Link>
          {children}
        </div>
        <p className={styles.edu}>
          An educational demo for adults 18 and over. Not a bookmaker, and not affiliated with any sportsbook, prediction market, league or team.
        </p>
      </main>
    </div>
  )
}
