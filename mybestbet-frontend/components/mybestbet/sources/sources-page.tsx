"use client"

import { useRef } from "react"
import { msText } from "@/lib/odds/time"
import { countryName } from "@/lib/geo"
import { S, isAvailable, type Source } from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { userCountry } from "@/lib/store/prefs"
import { BookLogo } from "@/components/mybestbet/shared/logos"
import { Pager, usePage } from "@/components/mybestbet/shared/pager"
import { ApiError, Loading } from "@/components/mybestbet/shared/status"
import styles from "./sources-page.module.css"

/** Feed health for every sportsbook, prediction market and pick'em source. */
export function SourcesPage() {
  useStore()
  if (S.boot === "error") return <ApiError />
  if (S.boot === "loading") return <Loading label="Connecting to the API" />
  const groups = ([
    ["Sportsbooks", S.sources.filter((s) => s.kind === "book")],
    ["Prediction markets", S.sources.filter((s) => s.kind === "pm")],
    ["Pick'em", S.sources.filter((s) => s.kind === "pick")],
  ] as [string, Source[]][]).filter((g) => g[1].length)
  const ok = S.sources.filter((s) => !s.down && !s.h.consecutiveFailures).length
  return (
    <>
      <div className={styles.intro}>
        <h1>Sources</h1>
        <p className={"edu " + styles.eduFlat}>Educational demo. These books are listed for illustration only; mybestbet has no licence or partnership with them.</p>
        <span className={"row muted " + styles.health}>
          <span className={"dot" + (ok < S.sources.length ? " amber" : "")} />
          {ok} of {S.sources.length} healthy · {Object.keys(S.lg).length} competitions · {S.sports.length} sports
        </span>
      </div>
      {groups.map(([name, list]) => <SourceGroup key={name} name={name} list={list} />)}
    </>
  )
}

function SourceGroup({ name, list }: { name: string; list: Source[] }) {
  const anchor = useRef<HTMLSpanElement>(null)
  const page = usePage(list, "src|" + name)
  return (
    <section className={styles.group}>
      <h2 className={styles.groupTitle}>{name} <span className="faint mono">{list.length}</span></h2>
      <span className={styles.pgTop} ref={anchor} />
      <div className={styles.list}>
        <div className={styles.src + " " + styles.h}>
          <span></span><span>Source</span><span>Fetch</span><span>Freshness</span><span>Requests</span><span>Errors</span><span>Status</span>
        </div>
        {page.items.map((s) => <SourceRow key={s.name} s={s} />)}
      </div>
      <Pager page={page.page} total={page.total} onPage={page.setPage} label={name} anchor={anchor} />
    </section>
  )
}

const ERROR_LABELS: [RegExp, string][] = [
  [/status (401|403|451)|forbidden|geo|region|blocked|captcha|cloudflare|access denied|not available/i, "Blocked from this region"],
  [/status 429|rate.?limit|too many requests/i, "Rate limited"],
  [/time(d)? ?out|deadline exceeded|context canceled/i, "Timed out"],
  [/no odds|no events|no markets|empty (page|payload|response)/i, "No odds returned"],
  [/unexpected eof|connection (reset|refused)|no such host|tls|dns|network/i, "Connection failed"],
]
const KNOWN_LABELS = new Set([...ERROR_LABELS.map(([, label]) => label), "Upstream error"])

function errorLabel(raw: string) {
  const text = raw.trim()
  if (KNOWN_LABELS.has(text)) return text
  return ERROR_LABELS.find(([re]) => re.test(text))?.[1] || "Upstream error"
}

function SourceRow({ s }: { s: Source }) {
  const h = s.h
  const failing = h.enabled && !h.degraded && h.consecutiveFailures > 0
  const bad = !h.enabled || h.degraded || failing
  const status = !h.enabled ? "Off" : h.degraded ? "Degraded" : failing ? (h.lastSuccess ? "Retrying" : "Failing") : "OK"
  const away = !isAvailable(s.name)
  return (
    <div className={styles.src + (bad ? " " + styles.down : "") + (away ? " " + styles.away : "")}>
      <BookLogo name={s.name} size={26} />
      <span className={styles.nm2}>
        <span className={styles.name}>{s.name}</span>
        {away ? <span>Not available in {countryName(userCountry())}</span> : null}
        {bad && h.lastError ? <span className={styles.err}>{errorLabel(h.lastError)}</span> : s.note ? <span>{s.note}</span> : null}
      </span>
      <span className={"mono muted " + styles.cell}>{h.lastFetchMs} ms</span>
      <span className={"mono " + styles.cell + " " + (bad ? styles.bad : styles.ok)}>{h.lastSuccess ? msText(h.freshnessMs) : "–"}</span>
      <span className={"mono faint " + styles.cell}>{h.requests.toLocaleString("en-US")}</span>
      <span className={"mono " + styles.cell + " " + (bad ? styles.bad : styles.okSoft)}>
        {(h.errorRate * 100).toFixed(1)}%
        {h.consecutiveFailures ? <> <i className="faint">· {h.consecutiveFailures} in a row</i></> : null}
      </span>
      <span className={styles.st + " " + (bad ? styles.bad : styles.ok)}>
        <span className={"dot" + (bad ? " amber" : "")} />
        {status}
      </span>
    </div>
  )
}
