"use client"

import Link from "next/link"
import { useEffect } from "react"
import { routes } from "@/lib/routes"
import { S, historyKey, loadHistory, type SignalItem } from "@/lib/store/data"
import { ExternalIcon } from "@/components/mybestbet/shared/icons"
import { BookLink, BookLogo } from "@/components/mybestbet/shared/logos"
import { Sparkline, historyPoints } from "@/components/mybestbet/shared/sparkline"
import { Loading } from "@/components/mybestbet/shared/status"
import { useOdds } from "@/components/mybestbet/shared/use-odds"
import styles from "./signals-page.module.css"
import { webHref } from "@/lib/utils"

/** Price history of the moved book under an open steam move row. */
export function MoveDetail({ s }: { s: SignalItem }) {
  const F = useOdds()
  const f = s.f, l = s.legs[0]
  const h = S.hist[historyKey(s.ks, l.src)]
  useEffect(() => {
    loadHistory(s.ks, l.src).catch(() => {})
  }, [s.ks, l.src])
  const points = h ? historyPoints(h, l.side, l.label) : []
  return (
    <div className={styles.evD + " " + styles.mvD}>
      {h ? (
        <div className={styles.hpR}>
          <span className={styles.hpL}><BookLink name={l.src} f={f} deep={l.deep} size={16}>{l.src}</BookLink></span>
          <Sparkline points={points} format={F} className={styles.spk} />
          <span className={styles.hpV + " mono"}>{points.length ? F(points[0].v) + " → " + F(points[points.length - 1].v) : "–"}</span>
        </div>
      ) : (
        <Loading label="Loading history" />
      )}
      <div className={styles.evA}>
        <Link className="btn sm" href={routes.event(f.id)}>All prices</Link>
        {webHref(l.deep) && (
          <a className="btn sm primary" href={webHref(l.deep)} target="_blank" rel="noopener noreferrer">
            <BookLogo name={l.src} size={16} />Open {l.src} <ExternalIcon />
          </a>
        )}
      </div>
    </div>
  )
}
