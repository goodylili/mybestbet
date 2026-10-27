"use client"

import { useEffect, useId, useRef, useState } from "react"
import { BookLink, BookLogo } from "@/components/mybestbet/shared/logos"
import { CheckIcon, ExternalIcon } from "@/components/mybestbet/shared/icons"
import { ApiError, post } from "@/lib/api/client"
import type { BookingComparison, BookingLeg, BookingMode, BookingResponse, BookQuote, BookResult, ManualLeg } from "@/lib/api/types"
import { money } from "@/lib/odds/format"
import { S } from "@/lib/store/data"
import type { SlipPick } from "@/lib/store/prefs"
import type { pickInfo } from "@/lib/store/selectors"
import { appHref, webHref } from "@/lib/utils"
import styles from "./slip-booking.module.css"

const BOOKABLE = new Set(["sportybet", "footballcom", "bet9ja", "betking", "betpawa", "onexbet", "22bet", "paripesa", "betwinner", "betano", "bet365"])

export type SlipRow = { p: SlipPick; k: number; info: ReturnType<typeof pickInfo> }

type Run = { sig: string; busy: string[]; res: Record<string, BookResult>; manual: ManualLeg[] }

type Compare = { sig: string; data: BookingComparison | null }

function legOf(row: SlipRow): BookingLeg {
  const b = S.det[row.p.fx]?.by[row.p.key]?.b || S.props[row.p.fx]?.by[row.p.key]
  const s = b?.selections[row.p.i]
  const side = s?.side || ""
  return { eventId: row.p.fx, marketKey: row.p.key, source: row.info.src, side, label: side ? "" : s?.label || row.p.label }
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    const ta = document.createElement("textarea")
    ta.value = text
    ta.setAttribute("readonly", "")
    ta.style.position = "fixed"
    ta.style.opacity = "0"
    document.body.appendChild(ta)
    ta.select()
    const ok = document.execCommand("copy")
    ta.remove()
    return ok
  }
}

function errorText(e: unknown) {
  if (e instanceof ApiError) {
    if (e.status === 429) return "Too many booking requests." + (e.retryAfter ? " Try again in " + e.retryAfter + "s." : " Try again shortly.")
    if (e.status === 0) return "No connection. Check your network and retry."
    return e.message
  }
  return "Booking failed. Retry in a moment."
}

const hasCode = (r?: BookResult) => !!r && !!r.code && (r.status === "ok" || r.status === "partial")

const STATUS: Record<string, string> = { failed: "Failed", blocked: "Blocked here", unsupported: "No codes" }

function CopyButton({ text, label, children, onDone }: { text: string; label: string; children: string; onDone: (ok: boolean) => void }) {
  const [done, setDone] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => () => clearTimeout(timer.current), [])
  return (
    <button
      type="button"
      className={styles.copy}
      aria-label={label}
      onClick={async () => {
        const ok = await copyText(text)
        onDone(ok)
        if (!ok) return
        setDone(true)
        clearTimeout(timer.current)
        timer.current = setTimeout(() => setDone(false), 1600)
      }}
    >
      {done ? <><CheckIcon /> Copied</> : children}
    </button>
  )
}

type CardProps = { book: string; r?: BookResult; busy: boolean; names: string[]; onRetry: () => void; announce: (m: string) => void }

function ResultCard({ book, r, busy, names, onRetry, announce }: CardProps) {
  if (busy || !r) {
    return (
      <li className={styles.res} aria-busy="true">
        <div className={styles.resH}>
          <BookLogo name={book} size={20} />
          <b>{book}</b>
          <span className={styles.st}>Creating code</span>
        </div>
        <span className={styles.ghost} aria-hidden="true" />
      </li>
    )
  }
  const web = webHref(r.webUrl)
  if (!hasCode(r)) {
    const final = r.status === "blocked" || r.status === "unsupported"
    return (
      <li className={styles.res} data-state={r.status === "blocked" ? "blocked" : "failed"}>
        <div className={styles.resH}>
          <BookLogo name={book} size={20} />
          <b>{book}</b>
          <span className={styles.st}>{STATUS[r.status] || "Failed"}</span>
        </div>
        <p className={styles.why}>
          {r.message || (r.status === "blocked" ? book + " refuses booking requests from this region." : "The book did not return a code.")}
          {final ? " Place these legs on " + book + " yourself." : ""}
        </p>
        {final ? null : (
          <div className={styles.acts}>
            <button type="button" className="btn sm" onClick={onRetry}>Retry {book}</button>
          </div>
        )}
      </li>
    )
  }
  const code = r.code!
  const app = appHref(r.appUrl)
  const missed = r.legs.filter((l) => !l.ok)
  const booked = r.legs.length - missed.length
  return (
    <li className={styles.res} data-state={r.status}>
      <div className={styles.resH}>
        <BookLogo name={book} size={20} />
        <b>{book}</b>
        <span className={styles.st}>{booked} of {r.legs.length} legs{r.odds ? " at " + r.odds.toFixed(2) : ""}</span>
      </div>
      <div className={styles.code}>
        <span className="mono" aria-label={"Booking code " + code.split("").join(" ")}>{code}</span>
        <CopyButton text={code} label={"Copy " + book + " code"} onDone={(ok) => announce(ok ? book + " code copied" : "Copy failed. Select the code to copy it.")}>Copy</CopyButton>
      </div>
      <div className={styles.acts}>
        {web && <a className="btn sm primary" href={web} target="_blank" rel="noopener noreferrer">Open in {book} <ExternalIcon /></a>}
        {app && app !== web && <a className="btn sm" href={app} rel="noopener noreferrer">Open the {book} app</a>}
      </div>
      {missed.length > 0 && (
        <ul className={styles.miss} aria-label={"Legs not booked on " + book}>
          {missed.map((l) => (
            <li key={l.index}>
              <span>{names[l.index] || "Leg " + (l.index + 1)}</span>
              <i>{l.reason || "Not offered"}</i>
            </li>
          ))}
        </ul>
      )}
      {r.expiresAt && (
        <p className={styles.exp}>
          Expires {new Date(r.expiresAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
          {r.cached ? ", same code as your last request" : ""}
        </p>
      )}
    </li>
  )
}

function QuoteRow({ q, single, on, name, stake, onPick }: { q: BookQuote; single: boolean; on: boolean; name: string; stake: number; onPick: () => void }) {
  const full = q.covered === q.total
  const can = q.bookable && q.covered > 0
  return (
    <tr className={on ? styles.on : q.bookable ? undefined : styles.ro}>
      <th scope="row">
        {single ? (
          <label className={styles.bn}>
            <input type="radio" name={name} checked={on} disabled={!can} onChange={onPick} />
            <BookLogo name={q.book} size={16} />
            <span>{q.book}</span>
            {q.bookable ? null : <i className={styles.nb}>no codes</i>}
          </label>
        ) : (
          <span className={styles.bn}>
            <BookLogo name={q.book} size={16} />
            <span>{q.book}</span>
            {q.bookable ? null : <i className={styles.nb}>no codes</i>}
          </span>
        )}
      </th>
      <td className={"mono" + (full ? "" : " " + styles.part)}>{q.covered}/{q.total}</td>
      <td className="mono">{money(q.odds * stake)}</td>
    </tr>
  )
}

/** Turn the slip into booking codes: one per book, or the whole slip on one book. */
export function SlipBooking({ rows }: { rows: SlipRow[] }) {
  const id = useId()
  const [mode, setMode] = useState<BookingMode>("split")
  const [stake, setStakeText] = useState("1000")
  const [pick, setPick] = useState<string | null>(null)
  const [cmp, setCmp] = useState<Compare>({ sig: "", data: null })
  const [run, setRun] = useState<Run | null>(null)
  const [note, setNote] = useState("")

  const legs = rows.map(legOf)
  const names = rows.map((r) => r.info.sel)
  const legsSig = JSON.stringify(legs)

  useEffect(() => {
    if (!legsSig || legsSig === "[]") return
    let live = true
    const t = setTimeout(() => {
      post<BookingComparison>("/v1/bookings/compare", { legs: JSON.parse(legsSig) }).then(
        (data) => live && setCmp({ sig: legsSig, data }),
        () => live && setCmp({ sig: legsSig, data: null }),
      )
    }, 350)
    return () => {
      live = false
      clearTimeout(t)
    }
  }, [legsSig])

  const compare = cmp.sig === legsSig ? cmp.data : null
  const quotes = compare ? compare.books : []
  const stakeN = Math.max(0, parseFloat(stake) || 0)
  const single = mode === "single"
  const book = pick && quotes.some((q) => q.book === pick && q.bookable && q.covered > 0) ? pick : quotes.find((q) => q.bookable && q.covered > 0)?.book || null
  const canBook = (src: string) => (compare ? quotes.some((q) => q.book === src && q.bookable) : BOOKABLE.has(src))
  const splitBooks = [...new Set(rows.filter((r) => canBook(r.info.src)).map((r) => r.info.src))]
  const books = single ? (book ? [book] : []) : splitBooks
  const sig = mode + "|" + (single ? book : "") + "|" + legsSig
  const shown = run && run.sig === sig ? run : null
  const busy = !!shown && shown.busy.length > 0

  const announce = (m: string) => setNote((n) => (n === m ? m + " " : m))

  const submit = (sent: string[], idx: number[]) => {
    const at = sig
    setRun((cur) => {
      const same = cur && cur.sig === at
      return { sig: at, busy: [...new Set([...(same ? cur.busy : []), ...sent])], res: same ? cur.res : {}, manual: same ? cur.manual : [] }
    })
    const body = { legs: idx.map((i) => legs[i]), mode, book: single ? book || undefined : undefined, stake: stakeN || undefined }
    const done = (patch: (res: Record<string, BookResult>) => void, manual?: ManualLeg[]) =>
      setRun((cur) => {
        if (!cur || cur.sig !== at) return cur
        const res = { ...cur.res }
        patch(res)
        return { ...cur, busy: cur.busy.filter((b) => !sent.includes(b)), res, manual: manual ?? cur.manual }
      })
    post<BookingResponse>("/v1/bookings", body).then(
      (out) => {
        const remap = <T extends { index: number }>(x: T): T => ({ ...x, index: idx[x.index] ?? x.index })
        done(
          (res) => {
            for (const r of out.bookings) res[r.book] = { ...r, legs: r.legs.map(remap) }
            for (const b of sent) res[b] ??= { book: b, status: "failed", legs: [], cached: false, message: "The book did not answer." }
          },
          idx.length === legs.length ? out.manual.map(remap) : undefined,
        )
        const ok = out.bookings.filter(hasCode).length
        announce(ok ? ok + (ok === 1 ? " booking code ready" : " booking codes ready") : "No codes were created")
      },
      (e) => {
        const message = errorText(e)
        done((res) => {
          for (const b of sent) res[b] = { book: b, status: "failed", legs: [], cached: false, message }
        })
        announce(message)
      },
    )
  }

  const all = legs.map((_, i) => i)
  const bookAll = () => {
    if (!books.length) return
    setRun(null)
    submit(books, all)
  }
  const retry = (b: string) => submit([b], single ? all : all.filter((i) => legs[i].source === b))

  const ready = shown ? books.map((b) => shown.res[b]).filter(hasCode) : []
  const packText = ready.map((r) => r.book + ": " + r.code + (webHref(r.webUrl) ? " " + webHref(r.webUrl) : "")).join("\n")

  const share = async () => {
    if (typeof navigator.share === "function") {
      try {
        await navigator.share({ title: "Booking codes", text: packText })
        return
      } catch (e) {
        if (e instanceof DOMException && e.name === "AbortError") return
      }
    }
    const ok = await copyText(packText)
    announce(ok ? "Sharing isn't available here, so the codes were copied" : "Copy failed")
  }

  const bookedIdx = new Set(shown ? Object.values(shown.res).flatMap((r) => (hasCode(r) ? r.legs.filter((l) => l.ok).map((l) => l.index) : [])) : [])
  const manual: ManualLeg[] = shown && !busy
    ? shown.manual.filter((m) => !bookedIdx.has(m.index))
    : rows.flatMap((r, i) => (single ? !quotes.some((q) => q.book === book && !q.missing.includes(i)) : !splitBooks.includes(r.info.src))
      ? [{ index: i, source: r.info.src, deepLink: r.info.deep, reason: single ? "Not offered on " + (book || "this book") : "No booking codes" }]
      : [])
  const pending = !compare && cmp.sig !== legsSig
  const label = single ? (book ? "Book all on " + book : "No book takes these legs") : splitBooks.length > 1 ? "Book on " + splitBooks.length + " books" : splitBooks.length ? "Book on " + splitBooks[0] : "No book here makes codes yet"

  return (
    <section className={styles.bk} aria-labelledby={id + "h"}>
      <div className={styles.bkH}>
        <h3 id={id + "h"}>Booking codes</h3>
        <div className="seg" role="group" aria-label="How to book">
          <button type="button" aria-pressed={!single} onClick={() => setMode("split")}>Split across books</button>
          <button type="button" aria-pressed={single} onClick={() => setMode("single")}>One book</button>
        </div>
      </div>

      <label className={styles.stake} htmlFor={id + "s"}>
        <span>Stake</span>
        <input id={id + "s"} type="number" min="0" step="100" inputMode="decimal" value={stake} onChange={(e) => setStakeText(e.target.value)} />
      </label>

      {compare ? (
        <table className={styles.cmp}>
          <caption className="sr">Potential return per book for {compare.totalLegs} legs at a stake of {money(stakeN)}</caption>
          <thead>
            <tr>
              <th scope="col">Book</th>
              <th scope="col">Legs</th>
              <th scope="col">Returns</th>
            </tr>
          </thead>
          <tbody>
            <tr className={styles.best}>
              <th scope="row"><span className={styles.bn}>Best split</span></th>
              <td className="mono">{compare.bestSplit.covered}/{compare.totalLegs}</td>
              <td className="mono">{money(compare.bestSplit.odds * stakeN)}</td>
            </tr>
            <tr className={styles.pk}>
              <th scope="row"><span className={styles.bn}>Your picks</span></th>
              <td className="mono">{compare.picked.covered}/{compare.totalLegs}</td>
              <td className="mono">{money(compare.picked.odds * stakeN)}</td>
            </tr>
            {quotes.filter((q) => q.covered > 0).map((q) => (
              <QuoteRow key={q.book} q={q} single={single} on={single && q.book === book} name={id + "b"} stake={stakeN} onPick={() => setPick(q.book)} />
            ))}
          </tbody>
        </table>
      ) : (
        <p className={styles.why} aria-live="polite">{pending ? "Comparing books" : "Comparison unavailable right now."}</p>
      )}

      <button type="button" className={"btn primary " + styles.go} disabled={busy || !books.length} aria-busy={busy} onClick={bookAll}>
        {busy ? "Booking" : label}
      </button>

      {shown && (
        <>
          <ul className={styles.list} aria-label="Booking codes">
            {books.map((b) => (
              <ResultCard key={b} book={b} r={shown.res[b]} busy={shown.busy.includes(b)} names={names} onRetry={() => retry(b)} announce={announce} />
            ))}
          </ul>
          {ready.length > 0 && (
            <div className={styles.pack}>
              <CopyButton text={packText} label={"Copy all " + ready.length + " codes"} onDone={(ok) => announce(ok ? "All codes copied" : "Copy failed")}>Copy all codes</CopyButton>
              <button type="button" className={styles.copy} onClick={share}>Share</button>
            </div>
          )}
        </>
      )}

      {manual.length > 0 && (
        <div className={styles.man}>
          <p>{shown ? "Not in any code. Place these yourself." : "No booking code for these. Place them yourself."}</p>
          <ul>
            {manual.map((m) => {
              const href = webHref(m.deepLink || rows[m.index]?.info.deep)
              const sel = names[m.index] || "Leg " + (m.index + 1)
              return (
                <li key={m.index}>
                  <BookLink name={m.source} f={rows[m.index]?.info.f} deep={href} size={16} />
                  <span>
                    {sel}
                    <i>{m.reason}</i>
                  </span>
                  {href ? (
                    <a href={href} target="_blank" rel="noopener noreferrer" aria-label={"Place " + sel + " manually on " + m.source}>
                      Place manually <ExternalIcon />
                    </a>
                  ) : (
                    <i>No link</i>
                  )}
                </li>
              )
            })}
          </ul>
        </div>
      )}

      <p className="sr" role="status" aria-live="polite">{note}</p>
    </section>
  )
}
