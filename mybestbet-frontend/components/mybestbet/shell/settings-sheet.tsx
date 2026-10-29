"use client"

import Image from "next/image"
import { useEffect, useRef, useState } from "react"
import { BookLogo } from "@/components/mybestbet/shared/logos"
import { CloseIcon } from "@/components/mybestbet/shared/icons"
import { replayIntro } from "@/components/mybestbet/onboarding/onboarding"
import { ALL_LOCATIONS, countryList, countryName } from "@/lib/geo"
import { S, isAvailable, type Source } from "@/lib/store/data"
import { useStore } from "@/lib/store/emitter"
import { prefs, setAllSources, setCountry, setFormat, setTheme, toggleDisplay, toggleLocalOnly, toggleSource, userCountry } from "@/lib/store/prefs"
import { auth, signOut } from "@/lib/store/session"
import type { OddsFormat } from "@/lib/odds/format"
import { useShell } from "./shell-context"
import styles from "./settings-sheet.module.css"

const FORMATS: [OddsFormat, string][] = [["dec", "Decimal"], ["us", "American"], ["pct", "%"]]

function SourceGroup({ title, list }: { title: string; list: Source[] }) {
  if (!list.length) return null
  return (
    <>
      <h3>{title}</h3>
      <div>
        {list.map((s) => {
          const away = prefs.geo.only && !isAvailable(s.name)
          return (
            <button key={s.name} className={styles.sw} role="switch" aria-checked={!away && !prefs.off[s.name]} disabled={away} onClick={() => toggleSource(s.name)}>
              <BookLogo name={s.name} size={28} />
              <span className={styles.tx}>
                {s.name}
                {away ? <span>Not available in {countryName(userCountry())}</span> : s.down ? <span style={{ color: "var(--amber)" }}>Not responding</span> : s.note ? <span>{s.note}</span> : null}
              </span>
              <span className={styles.tog} aria-hidden="true" />
            </button>
          )
        })}
      </div>
    </>
  )
}

function Location() {
  const country = userCountry()
  return (
    <div>
      <h3>Your location</h3>
      <div className={styles.setG}>
        <label className={styles.setR}>
          <span>Country</span>
          <select className={styles.sel} value={country} onChange={(e) => setCountry(e.target.value)} aria-label="Your country">
            {country ? null : <option value="">Choose a country</option>}
            <option value={ALL_LOCATIONS}>All locations</option>
            {countryList().map((c) => <option key={c.code} value={c.code}>{c.name}</option>)}
          </select>
        </label>
      </div>
      {country === ALL_LOCATIONS ? (
        <p className={styles.note}>Every book and signal is shown, wherever it operates.</p>
      ) : (
        <button className={styles.sw} role="switch" aria-checked={prefs.geo.only} onClick={toggleLocalOnly}>
          <span className={styles.tx}>
            Only books available in my location
            <span>Hide books and signals you cannot use{country ? " in " + countryName(country) : ""}</span>
          </span>
          <span className={styles.tog} aria-hidden="true" />
        </button>
      )}
    </div>
  )
}

function DisplaySwitch({ k, title, sub }: { k: "prob" | "flash" | "cd"; title: string; sub: string }) {
  return (
    <button className={styles.sw} role="switch" aria-checked={prefs.display[k]} onClick={() => toggleDisplay(k)}>
      <span className={styles.tx}>
        {title}
        <span>{sub}</span>
      </span>
      <span className={styles.tog} aria-hidden="true" />
    </button>
  )
}

function Account() {
  const [leaving, setLeaving] = useState(false)
  const user = auth.user
  if (!user) return null
  const label = user.name || user.email
  return (
    <div className={styles.acct}>
      {user.avatarUrl ? (
        <Image className={styles.av} src={user.avatarUrl} alt="" width={40} height={40} referrerPolicy="no-referrer" unoptimized />
      ) : (
        <span className={styles.av} aria-hidden="true">{label.trim().charAt(0).toUpperCase()}</span>
      )}
      <span className={styles.who}>
        {label}
        {user.name ? <span>{user.email}</span> : null}
      </span>
      <button className="btn sm" disabled={leaving} onClick={() => { setLeaving(true); signOut() }}>
        {leaving ? "Signing out" : "Sign out"}
      </button>
    </div>
  )
}

/** Settings: theme, odds format, display toggles and which books count towards best prices. */
export function SettingsSheet() {
  useStore()
  const shell = useShell()
  const [opened, setOpened] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  if (shell.sheetOpen && !opened) setOpened(true)
  useEffect(() => {
    if (!shell.sheetOpen) return
    const back = document.activeElement instanceof HTMLElement ? document.activeElement : null
    ref.current?.querySelector<HTMLElement>("button")?.focus({ preventScroll: true })
    return () => back?.focus({ preventScroll: true })
  }, [shell.sheetOpen])
  const theme = prefs.theme
  const allOn = S.sources.every((s) => !prefs.off[s.name])

  return (
    <div ref={ref} className={styles.sheet} role="dialog" aria-modal="true" aria-label="Settings" inert={!shell.sheetOpen}>
      {opened && (
        <>
          <div className={styles.shH}>
            <h2>Settings</h2>
            <button className="ib" style={{ marginLeft: "auto", width: 32, height: 32 }} aria-label="Close" onClick={shell.closeAll}>
              <CloseIcon />
            </button>
          </div>
          <div className={styles.shB}>
            <Account />
            <div className={styles.setG}>
              <div className={styles.setR}>
                <span>Appearance</span>
                <div className="seg" role="group" aria-label="Theme">
                  <button aria-pressed={theme !== "light"} onClick={() => setTheme("dark")}>Dark</button>
                  <button aria-pressed={theme === "light"} onClick={() => setTheme("light")}>Light</button>
                </div>
              </div>
              <div className={styles.setR}>
                <span>Odds format</span>
                <div className="seg" role="group" aria-label="Odds format">
                  {FORMATS.map(([k, label]) => (
                    <button key={k} aria-pressed={prefs.fmt === k} onClick={() => setFormat(k)}>{label}</button>
                  ))}
                </div>
              </div>
            </div>
            <Location />
            <div>
              <h3>Display</h3>
              <div>
                <DisplaySwitch k="prob" title="Win probability" sub="Percent chance next to each team" />
                <DisplaySwitch k="flash" title="Price movement" sub="Flash odds when they shorten or drift" />
                <DisplaySwitch k="cd" title="Kick-off countdowns" sub="Live timer for games in the next 12 hours" />
              </div>
            </div>
            <div>
              <h3>
                Books in best prices
                <button onClick={() => (allOn ? setAllSources(S.sources.map((s) => s.name), false) : setAllSources([], true))}>
                  {allOn ? "Turn all off" : "Turn all on"}
                </button>
              </h3>
              <SourceGroup title="Sportsbooks" list={S.sources.filter((s) => s.kind === "book")} />
              <SourceGroup title="Prediction markets" list={S.sources.filter((s) => s.kind === "pm")} />
              <SourceGroup title="Pick'em" list={S.sources.filter((s) => s.kind === "pick")} />
            </div>
            <p className={"edu " + styles.edu}>
              mybestbet is an educational demo. It is not affiliated with, endorsed by or licensed by any sportsbook, prediction market, league or team shown. Names and logos belong to their owners.
            </p>
            <button className="btn sm" style={{ alignSelf: "flex-start" }} onClick={() => { shell.closeAll(); replayIntro() }}>
              Replay the intro
            </button>
          </div>
        </>
      )}
    </div>
  )
}
