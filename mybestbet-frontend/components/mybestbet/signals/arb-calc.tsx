"use client"

import { money, signedMoney } from "@/lib/odds/format"
import { TERM } from "@/lib/odds/glossary"
import type { SignalItem } from "@/lib/store/data"
import { arbStakes } from "@/lib/store/selectors"
import { ChevronDownIcon, ExternalIcon } from "@/components/mybestbet/shared/icons"
import { InfoTip } from "@/components/mybestbet/shared/info-tip"
import { BookLink } from "@/components/mybestbet/shared/logos"
import { useOdds } from "@/components/mybestbet/shared/use-odds"
import styles from "./signals-page.module.css"
import { webHref } from "@/lib/utils"

const QUICK = [10000, 50000, 100000]
const ROUNDING: [number, string][] = [[0, "Exact"], [10, "10"], [100, "100"], [1000, "1,000"]]

/** Inline stake calculator under an open arbitrage row: stake per book for an equal return. */
export function ArbCalc({ a, bank, round, onBank, onRound }: {
  a: SignalItem
  bank: string
  round: number
  onBank: (bank: string) => void
  onRound: (round: number) => void
}) {
  return (
    <section className={styles.calc + " " + styles.inl} aria-label="Stake calculator" data-tour="arb-calc">
      <div className={styles.cbar}>
        <label className={styles.cst} htmlFor="bank">
          <span>Stake <InfoTip label="Stake split" text={TERM.arbStake} /></span>
          <input id="bank" type="number" min="1" step="1000" inputMode="decimal" value={bank} onChange={(e) => onBank(e.target.value)} />
        </label>
        <span className={styles.qs}>
          {QUICK.map((v) => (
            <button key={v} onClick={() => onBank(String(v))}>{v / 1000}k</button>
          ))}
        </span>
        <label className={styles.crd}>
          <span>Round to <InfoTip label="Rounding" text={TERM.rounding} /></span>
          <select value={round} onChange={(e) => onRound(+e.target.value)}>
            {ROUNDING.map(([v, label]) => <option key={v} value={v}>{label}</option>)}
          </select>
          <ChevronDownIcon />
        </label>
      </div>
      <div id="calcBody">
        <CalcBody a={a} bank={Math.max(0, parseFloat(bank) || 0)} round={round} />
      </div>
    </section>
  )
}

function CalcBody({ a, bank, round }: { a: SignalItem; bank: number; round: number }) {
  const F = useOdds()
  const c = arbStakes(a, bank, round)
  const same = c.hi - c.lo < 0.01
  return (
    <>
      <div className={styles.cl}>
        {c.legs.map((l, i) => (
          <div key={i} className={styles.clR}>
            <span className={styles.clB}><BookLink name={l.src} f={a.f} deep={l.deep} size={16}>{l.src}</BookLink></span>
            <span className={styles.clO}>{l.lab} <i className="mono">{F(l.d)}</i></span>
            <b className="mono">{money(l.stake)}</b>
            {webHref(l.deep) ? (
              <a className={styles.clX} href={webHref(l.deep)} target="_blank" rel="noopener noreferrer" aria-label={"Open " + l.src + ": " + a.f.h + " v " + a.f.a}><ExternalIcon /></a>
            ) : (
              <span />
            )}
          </div>
        ))}
      </div>
      <div className={styles.clP}>
        <span>Profit <InfoTip label="Profit" text={TERM.arbProfit} /></span>
        <b className={"mono " + (c.profitLo >= 0 ? styles.pos : styles.neg)}>
          {same ? signedMoney(c.profitLo) : <>{signedMoney(c.profitLo)}<i> to </i>{signedMoney(c.profitHi)}</>}
        </b>
      </div>
      {c.profitLo < 0 && <p className={styles.cwarn}>Rounding is eating the edge. Raise the stake or round less.</p>}
    </>
  )
}
