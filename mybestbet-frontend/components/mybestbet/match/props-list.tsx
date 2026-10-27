"use client"

import { useEffect, useRef, useState } from "react"
import { marketInfo } from "@/lib/odds/glossary"
import { marketLabel } from "@/lib/odds/markets"
import { loadProps, S, type Fixture } from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { FitRow } from "@/components/mybestbet/shared/fit-row"
import { InfoTip } from "@/components/mybestbet/shared/info-tip"
import { Pager, usePage } from "@/components/mybestbet/shared/pager"
import { Loading } from "@/components/mybestbet/shared/status"
import { PropRow } from "./prop-row"
import styles from "./props.module.css"

/** Player props for a match: a prop type filter and a paged list of players with research panels. */
export function PropsList({ f }: { f: Fixture }) {
  useStore()
  const [kind, setKind] = useState("all")
  const [open, setOpen] = useState("")
  const top = useRef<HTMLSpanElement>(null)
  const P = S.props[f.id]
  const rows = P ? P.b.rows.filter((r) => kind === "all" || r.key.code === kind) : []
  const page = usePage(rows, "pr|" + f.id + kind)

  useEffect(() => {
    loadProps(f.id).catch(() => {})
  }, [f.id])

  if (!P) return <Loading label="Loading player props" />
  if (!P.b.rows.length) {
    return (
      <div className="empty">
        <p>No player props yet</p>
        <p className="faint">{P.err || "Usually posted the day before."}</p>
      </div>
    )
  }
  const codes = [...new Set(P.b.rows.map((r) => r.key.code))]
  const about = kind === "all" ? null : marketInfo(kind, f.sp)
  return (
    <>
      <FitRow className="chips" label="Prop type" role="group">
        <button className="chip sm" aria-pressed={kind === "all"} onClick={() => setKind("all")}>All</button>
        {codes.map((k) => (
          <button key={k} className="chip sm" aria-pressed={kind === k} onClick={() => setKind(k)}>{marketLabel(k, f.sp)}</button>
        ))}
      </FitRow>
      {about && (
        <p className={styles.about}>
          <b>{marketLabel(kind, f.sp)}</b>
          <InfoTip label={marketLabel(kind, f.sp)} text={about} />
        </p>
      )}
      <span className={styles.pgTop} ref={top} />
      <div className={styles.list}>
        {page.items.map((r) => (
          <PropRow key={r.keyString} f={f} r={r} open={open === r.keyString} onToggle={() => setOpen(open === r.keyString ? "" : r.keyString)} />
        ))}
      </div>
      <Pager page={page.page} total={page.total} onPage={page.setPage} label="Props" anchor={top} />
    </>
  )
}
