"use client"

import Link from "next/link"
import { BookLink, Crest, LeagueMark } from "@/components/mybestbet/shared/logos"
import { CloseIcon, ExternalIcon } from "@/components/mybestbet/shared/icons"
import { useOdds } from "@/components/mybestbet/shared/use-odds"
import { arbPct, signalList } from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { clearSlip, prefs, removePick, setPickSource, setStake } from "@/lib/store/prefs"
import { pickInfo } from "@/lib/store/selectors"
import { marketLabel } from "@/lib/odds/markets"
import { money } from "@/lib/odds/format"
import { routes } from "@/lib/routes"
import { useShell } from "./shell-context"
import { useNav } from "./use-nav"
import { SlipBooking } from "./slip-booking"
import styles from "./bet-slip.module.css"
import { webHref } from "@/lib/utils"

/** Picks with their chosen book, plus total stake and returns. */
export function useSlipTotals() {
  useStore()
  const picks = prefs.slip.map((p, k) => ({ p, k, info: pickInfo(p) })).filter((x) => x.info.f)
  const stake = picks.reduce((a, x) => a + (parseFloat(x.p.stake) || 0), 0)
  const returns = picks.reduce((a, x) => a + (parseFloat(x.p.stake) || 0) * (x.info.price || 0), 0)
  return { picks, stake, returns }
}

/** The bet slip: a right column on wide screens, a drawer below 1280px and a bottom sheet on phones. */
export function BetSlip() {
  useStore()
  const shell = useShell()
  const nav = useNav()
  const odds = useOdds()
  const { picks, stake, returns } = useSlipTotals()
  const count = prefs.slip.length
  const arbs = signalList("arb")

  return (
    <aside className={styles.slip} aria-label="Bet slip">
      <span className={styles.grab} aria-hidden="true" />
      <div className={styles.slipHead}>
        <h2>Bet slip</h2>
        {count > 0 && (
          <>
            <span className={styles.c}>{count}</span>
            <button className={styles.x} data-clear onClick={clearSlip}>Clear</button>
          </>
        )}
        <button className={styles.x + " " + styles.slipClose} style={count ? { marginLeft: 0 } : undefined} aria-label="Close" onClick={shell.closeAll}>
          <CloseIcon />
        </button>
      </div>
      {picks.length ? (
        <>
          <div className={styles.picks}>
            {picks.map(({ p, k, info }) => {
              const f = info.f!
              return (
                <div className={styles.pk} key={k}>
                  <div className={styles.pkA}>
                    {info.team ? <Crest team={info.team} size={22} /> : <LeagueMark code={f.lg} size={20} />}
                    <b className={styles.pkS}>{info.sel}</b>
                    <b className={styles.pkO + " mono"}>{odds(info.price)}</b>
                    <button className={styles.pkX} aria-label={"Remove " + info.sel} onClick={() => removePick(k)}>
                      <CloseIcon />
                    </button>
                  </div>
                  <div className={styles.pkM}>{info.market} · {f.h} v {f.a}</div>
                  <div className={styles.pkB}>
                    <span className={styles.pkSrc}>
                      {info.books.length > 1 ? <BookLink name={info.src} f={f} deep={info.deep} size={14} /> : null}
                      {info.books.length > 1 ? (
                        <select className={styles.pkBook} aria-label={"Book for " + info.sel} value={info.src} onChange={(e) => setPickSource(k, e.target.value)}>
                          {info.books.map((b) => <option key={b.name} value={b.name}>{b.name} · {odds(b.price)}</option>)}
                        </select>
                      ) : <BookLink name={info.src} f={f} deep={info.deep} size={14}>{info.src}</BookLink>}
                      <i>{info.n > 1 ? (info.rank === 1 ? "best of " + info.n : "#" + info.rank + " of " + info.n) : "best price"}</i>
                    </span>
                    {webHref(info.deep) && (
                      <a className={styles.pkGo} href={webHref(info.deep)} target="_blank" rel="noopener noreferrer">
                        Open <ExternalIcon />
                      </a>
                    )}
                  </div>
                  <div className={styles.pkC}>
                    <label className={styles.pkIn} htmlFor={"stake-" + k}>
                      <span className="sr">Stake</span>
                      <input
                        id={"stake-" + k}
                        type="number"
                        min="0"
                        step="100"
                        inputMode="decimal"
                        value={p.stake}
                        placeholder="Stake"
                        onChange={(e) => setStake(k, e.target.value)}
                      />
                    </label>
                    <span className={styles.pkR}>
                      Returns <b className="mono">{money((parseFloat(p.stake) || 0) * (info.price || 0))}</b>
                    </span>
                  </div>
                </div>
              )
            })}
          </div>
          <div className={styles.tot}>
            <div className="muted" style={{ fontSize: 13 }}>
              <span>Total stake</span>
              <span className="mono">{money(stake)}</span>
            </div>
            <div style={{ fontSize: 15 }}>
              <span>Returns</span>
              <span className="mono">{money(returns)}</span>
            </div>
          </div>
          <SlipBooking rows={picks} />
        </>
      ) : (
        <div className="empty">
          <p>No picks yet</p>
          <p className="faint">Tap a price to add it.</p>
        </div>
      )}
      {arbs.length > 0 && nav.section !== "signals" && (
        <section className={"card " + styles.marbs} aria-label="Arbs">
          <h2>
            Arbs right now<Link href={routes.signals("arb")}>All</Link>
          </h2>
          {arbs
            .slice()
            .sort((x, y) => arbPct(y) - arbPct(x))
            .slice(0, 5)
            .map((a) => (
              <Link key={a.id} className={styles.arb} href={routes.signals("arb", a.id)}>
                <span className={styles.pairCr}>
                  <Crest team={a.f.hT} size={18} />
                  <Crest team={a.f.aT} size={18} />
                </span>
                <span className={styles.tx}>
                  {a.f.h} v {a.f.a}
                  <span className="faint"> · {marketLabel(a.key.code, a.f.sp)}</span>
                </span>
                <span className="badge arb">{arbPct(a).toFixed(2)}%</span>
              </Link>
            ))}
        </section>
      )}
    </aside>
  )
}
