"use client"

import Link from "next/link"
import { Fragment, useEffect, useId, useState, type FormEvent, type InputHTMLAttributes, type ReactNode } from "react"
import { ApiError, publicPost } from "@/lib/api/client"
import type { CodeSent, OtpPurpose } from "@/lib/api/types"
import { googleHref } from "@/lib/auth/paths"
import { loadMeta } from "@/lib/store/session"
import styles from "./auth.module.css"

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const CODE_ERRORS = ["invalid_code"]

export const rules = {
  email: (v: string) => (!v ? "Enter your email address." : EMAIL.test(v) ? "" : "Enter an email address like name@example.com."),
  newPassword: (v: string) => (v.length < 10 ? "Use at least 10 characters." : v.length > 128 ? "Use 128 characters or fewer." : ""),
  password: (v: string) => (v ? "" : "Enter your password."),
  code: (v: string) => (v.length === 6 ? "" : "Enter all 6 digits."),
  optional: () => "",
}

const plural = (n: number, unit: string) => n + " " + unit + (n === 1 ? "" : "s")
const wait = (s: number) => (s < 90 ? plural(s, "second") : plural(Math.ceil(s / 60), "minute"))

export function errorText(e: unknown) {
  const err = e instanceof ApiError ? e : new ApiError(0, "network", "")
  switch (err.code) {
    case "invalid_credentials":
      return "That email and password don't match. Check both and try again."
    case "invalid_code":
      return "That code isn't right or has expired. Use the latest email from mybestbet, or send a new code."
    case "rate_limited":
      return "Too many attempts. Try again in " + (err.retryAfter ? wait(err.retryAfter) : "a few minutes") + "."
    case "bad_request":
      return err.message && !err.message.startsWith("HTTP") ? err.message : "Some details aren't valid. Check them and try again."
    case "network":
      return "Can't reach mybestbet. Check your connection and try again."
    default:
      return err.status >= 500 || !err.message ? "Something went wrong on our side. Try again in a moment." : err.message
  }
}

export const hold = () => new Promise<never>(() => {})

export const leave = (href: string) => {
  location.assign(href)
  return hold()
}

type Values = Record<string, string>

export function useAuthForm(fields: Record<string, (v: string) => string>) {
  const [errors, setErrors] = useState<Values>({})
  const [failure, setFailure] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true)
    setFailure(null)
    try {
      await fn()
    } catch (e) {
      if (e instanceof ApiError && CODE_ERRORS.includes(e.code) && fields.code) setErrors((x) => ({ ...x, code: errorText(e) }))
      else setFailure(e)
    } finally {
      setBusy(false)
    }
  }

  const onSubmit = (fn: (values: Values) => Promise<unknown>) => (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (busy) return
    const form = e.currentTarget
    const data = new FormData(form)
    const values = Object.fromEntries(Object.keys(fields).map((k) => [k, String(data.get(k) ?? "").trim()]))
    const found = Object.fromEntries(Object.entries(fields).map(([k, rule]) => [k, rule(values[k])]))
    setErrors(found)
    const bad = Object.keys(found).find((k) => found[k])
    if (bad) form.querySelector<HTMLElement>(`[name="${bad}"]:not([type="hidden"]), [data-for="${bad}"]`)?.focus()
    else run(() => fn(values))
  }

  const bind = (name: string) => ({
    name,
    error: errors[name],
    onChange: () => {
      if (errors[name]) setErrors((x) => ({ ...x, [name]: "" }))
    },
  })

  return { busy, failure, onSubmit, bind }
}

type FieldProps = InputHTMLAttributes<HTMLInputElement> & { label: string; error?: string; hint?: string; aside?: ReactNode; action?: ReactNode }

export function Field({ label, error, hint, aside, action, ...input }: FieldProps) {
  const id = useId()
  const note = error || hint
  return (
    <div className={styles.field}>
      <div className={styles.labelRow}>
        <label htmlFor={id}>{label}</label>
        {aside}
      </div>
      <div className={styles.box}>
        <input
          id={id}
          className={styles.input + (action ? " " + styles.withAction : "")}
          aria-invalid={error ? true : undefined}
          aria-describedby={note ? id + "-n" : undefined}
          {...input}
        />
        {action}
      </div>
      {note ? <p id={id + "-n"} className={error ? styles.err : styles.hint}>{note}</p> : null}
    </div>
  )
}

export function PasswordField(props: Omit<FieldProps, "type" | "action">) {
  const [shown, setShown] = useState(false)
  return (
    <Field
      {...props}
      type={shown ? "text" : "password"}
      action={
        <button type="button" className={styles.reveal} aria-label={shown ? "Hide password" : "Show password"} aria-pressed={shown} onClick={() => setShown(!shown)}>
          {shown ? "Hide" : "Show"}
        </button>
      }
    />
  )
}

export const EMPTY_CODE = ["", "", "", "", "", ""]

export function CodeInput({ digits, onChange, onComplete, error }: {
  digits: string[]
  onChange: (digits: string[]) => void
  onComplete: () => void
  error?: string
}) {
  const id = useId()
  const focus = (i: number) => document.getElementById(id + "-" + Math.max(0, Math.min(5, i)))?.focus()

  const fill = (from: number, text: string) => {
    const chars = text.replace(/\D/g, "")
    if (chars.length >= 6) from = 0
    const next = digits.slice()
    chars.slice(0, 6 - from).split("").forEach((c, k) => { next[from + k] = c })
    onChange(next)
    focus(from + chars.length)
    if (chars && next.every(Boolean)) setTimeout(onComplete)
  }

  const clear = (i: number) => {
    const next = digits.slice()
    next[i] = ""
    onChange(next)
  }

  return (
    <div className={styles.field}>
      <span className={styles.labelRow} id={id + "-l"}>6-digit code</span>
      <div className={styles.code} role="group" aria-labelledby={id + "-l"} aria-describedby={error ? id + "-n" : undefined} data-invalid={error ? "" : undefined}>
        <input type="hidden" name="code" value={digits.join("")} />
        {digits.map((d, i) => (
          <Fragment key={i}>
            {i === 3 ? <span className={styles.dash} aria-hidden="true">-</span> : null}
            <input
              id={id + "-" + i}
              data-for={i === 0 ? "code" : undefined}
              className={d ? styles.filled : undefined}
              value={d}
              inputMode="numeric"
              pattern="[0-9]*"
              autoComplete={i === 0 ? "one-time-code" : "off"}
              aria-label={"Digit " + (i + 1) + " of 6"}
              aria-invalid={error ? true : undefined}
              onFocus={(e) => e.currentTarget.select()}
              onChange={(e) => {
                const v = e.currentTarget.value.replace(/\D/g, "")
                if (!v) return clear(i)
                fill(i, d && v.length === 2 ? (v[0] === d ? v[1] : v[0]) : v)
              }}
              onPaste={(e) => {
                e.preventDefault()
                fill(i, e.clipboardData.getData("text"))
              }}
              onKeyDown={(e) => {
                if (e.key === "Backspace" && !d && i > 0) {
                  e.preventDefault()
                  clear(i - 1)
                  focus(i - 1)
                } else if (e.key === "ArrowLeft") focus(i - 1)
                else if (e.key === "ArrowRight") focus(i + 1)
              }}
            />
          </Fragment>
        ))}
      </div>
      {error ? <p id={id + "-n"} className={styles.err}>{error}</p> : null}
    </div>
  )
}

function useCountdown(initial: number) {
  const [end, setEnd] = useState(() => Date.now() + initial * 1000)
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => {
      const t = Date.now()
      setNow(t)
      if (t >= end) clearInterval(id)
    }, 250)
    return () => clearInterval(id)
  }, [end])
  const start = (seconds: number) => {
    const t = Date.now()
    setNow(t)
    setEnd(t + seconds * 1000)
  }
  return [Math.max(0, Math.ceil((end - now) / 1000)), start] as const
}

const clock = (s: number) => Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0")

export function ResendCode({ email, purpose, initial }: { email: string; purpose: OtpPurpose; initial: number }) {
  const [left, start] = useCountdown(initial)
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState<{ ok: boolean; text: string } | null>(null)

  const resend = async () => {
    setBusy(true)
    setNote(null)
    try {
      const r = await publicPost<CodeSent>("/v1/auth/otp/resend", { email, purpose })
      start(r?.resendIn || 60)
      setNote({ ok: true, text: "We sent a new code to " + email + "." })
    } catch (e) {
      if (e instanceof ApiError && e.code === "rate_limited") start(e.retryAfter || 60)
      setNote({ ok: false, text: errorText(e) })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className={styles.resend}>
      <div className={styles.resendRow}>
        <span>Didn&apos;t get it?</span>
        <button type="button" className={styles.textBtn} disabled={busy || left > 0} onClick={resend}>
          {busy ? "Sending" : left > 0 ? "Resend code in " + clock(left) : "Resend code"}
        </button>
      </div>
      <p className={note?.ok ? styles.hint : styles.err} role="status">{note?.text}</p>
    </div>
  )
}

export function Alert({ children }: { children?: ReactNode }) {
  return children ? <div className={styles.alert} role="alert">{children}</div> : null
}

export function Submit({ busy, label, busyLabel }: { busy: boolean; label: string; busyLabel: string }) {
  return (
    <button type="submit" className={styles.submit} disabled={busy} aria-busy={busy}>
      {busy ? <span className={styles.spin} aria-hidden="true" /> : null}
      <span>{busy ? busyLabel : label}</span>
    </button>
  )
}

function GoogleMark() {
  return (
    <svg viewBox="0 0 18 18" width="18" height="18" aria-hidden="true">
      <path fill="#4285F4" d="M17.64 9.2c0-.64-.06-1.25-.16-1.84H9v3.48h4.84a4.14 4.14 0 0 1-1.8 2.72v2.26h2.92c1.7-1.57 2.68-3.88 2.68-6.62Z" />
      <path fill="#34A853" d="M9 18c2.43 0 4.47-.8 5.96-2.18l-2.92-2.26c-.8.54-1.84.86-3.04.86-2.34 0-4.32-1.58-5.03-3.7H.96v2.33A9 9 0 0 0 9 18Z" />
      <path fill="#FBBC05" d="M3.97 10.72a5.4 5.4 0 0 1 0-3.44V4.95H.96a9 9 0 0 0 0 8.1l3.01-2.33Z" />
      <path fill="#EA4335" d="M9 3.58c1.32 0 2.5.45 3.44 1.35l2.58-2.58A9 9 0 0 0 .96 4.95l3.01 2.33C4.68 5.16 6.66 3.58 9 3.58Z" />
    </svg>
  )
}

export function GoogleOption({ next }: { next: string }) {
  const [enabled, setEnabled] = useState(false)
  useEffect(() => {
    let live = true
    loadMeta().then((m) => {
      if (live) setEnabled(!!m.auth?.google)
    })
    return () => {
      live = false
    }
  }, [])
  if (!enabled) return null
  return (
    <div className={styles.google}>
      <a className={styles.googleBtn} href={googleHref(next)}>
        <GoogleMark />
        Continue with Google
      </a>
      <p className={styles.or}>Or continue with</p>
    </div>
  )
}

export function AuthHead({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className={styles.head}>
      <h1>{title}</h1>
      <p>{children}</p>
    </div>
  )
}

export function Switch({ text, href, label }: { text: string; href: string; label: string }) {
  return (
    <p className={styles.switch}>
      {text} <Link href={href}>{label}</Link>
    </p>
  )
}
