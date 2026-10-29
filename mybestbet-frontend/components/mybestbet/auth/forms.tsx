"use client"

import Link from "next/link"
import { useRouter } from "next/navigation"
import { useId, useState } from "react"
import { publicPost } from "@/lib/api/client"
import type { CodeSent, OtpPurpose } from "@/lib/api/types"
import { authHref, safeNext } from "@/lib/auth/paths"
import { startGuest } from "@/lib/auth/guest"
import { Alert, AuthHead, CodeInput, EMPTY_CODE, Field, GoogleOption, PasswordField, ResendCode, Submit, Switch, errorText, hold, leave, rules, useAuthForm } from "./kit"
import styles from "./auth.module.css"

type Props = { next: string; email?: string; resend?: number; googleFailed?: boolean }

export function LoginForm({ next, email, googleFailed }: Props) {
  const form = useAuthForm({ email: rules.email, password: rules.password })
  const submit = form.onSubmit(async (v) => {
    await publicPost("/v1/auth/login", { email: v.email, password: v.password })
    return leave(next)
  })
  return (
    <>
      <AuthHead title="Welcome back">Sign in with your Google account or email.</AuthHead>
      <Alert>{googleFailed && !form.failure ? "Google sign-in didn't finish. Try again, or use your email and password." : form.failure ? errorText(form.failure) : null}</Alert>
      <GoogleOption next={next} />
      <form className={styles.form} onSubmit={submit} noValidate>
        <fieldset className={styles.fs} disabled={form.busy}>
          <Field label="Email" type="email" autoComplete="email" placeholder="m@example.com" defaultValue={email} {...form.bind("email")} />
          <PasswordField
            label="Password"
            autoComplete="current-password"
            aside={<Link className={styles.link} href={authHref("/forgot", { next })}>Forgot your password?</Link>}
            {...form.bind("password")}
          />
        </fieldset>
        <Submit busy={form.busy} label="Sign in" busyLabel="Signing in" />
      </form>
      <Switch text="Don&apos;t have an account?" href={authHref("/signup", { next })} label="Sign up" />
      <p className={styles.skip}>
        <button type="button" onClick={() => { startGuest(); location.assign(safeNext(next)) }}>Skip for now</button>
        <span>Browse live odds without an account.</span>
      </p>
    </>
  )
}

export function SignupForm({ next, email }: Props) {
  const router = useRouter()
  const form = useAuthForm({ name: rules.optional, email: rules.email, password: rules.newPassword })
  const submit = form.onSubmit(async (v) => {
    const r = await publicPost<CodeSent>("/v1/auth/signup", { email: v.email, password: v.password, name: v.name || undefined })
    router.push(authHref("/verify", { email: v.email, next, resend: r?.resendIn }))
    return hold()
  })
  return (
    <>
      <AuthHead title="Create an account">Sign up with your Google account or email.</AuthHead>
      <Alert>{form.failure ? errorText(form.failure) : null}</Alert>
      <GoogleOption next={next} />
      <form className={styles.form} onSubmit={submit} noValidate>
        <fieldset className={styles.fs} disabled={form.busy}>
          <Field label="Name" autoComplete="name" placeholder="Optional" maxLength={80} {...form.bind("name")} />
          <Field label="Email" type="email" autoComplete="email" placeholder="m@example.com" defaultValue={email} {...form.bind("email")} />
          <PasswordField label="Password" autoComplete="new-password" hint="At least 10 characters." {...form.bind("password")} />
        </fieldset>
        <Submit busy={form.busy} label="Create account" busyLabel="Sending code" />
        <p className={styles.fine}>By creating an account you confirm you&apos;re 18 or over.</p>
      </form>
      <Switch text="Already have an account?" href={authHref("/login", { next })} label="Sign in" />
    </>
  )
}

export function ForgotForm({ next, email }: Props) {
  const router = useRouter()
  const form = useAuthForm({ email: rules.email })
  const submit = form.onSubmit(async (v) => {
    const r = await publicPost<CodeSent>("/v1/auth/password/forgot", { email: v.email })
    router.push(authHref("/reset", { email: v.email, next, resend: r?.resendIn }))
    return hold()
  })
  return (
    <>
      <AuthHead title="Reset your password">Enter the email you signed up with. If it has an account, we&apos;ll send it a 6-digit code.</AuthHead>
      <Alert>{form.failure ? errorText(form.failure) : null}</Alert>
      <form className={styles.form} onSubmit={submit} noValidate>
        <fieldset className={styles.fs} disabled={form.busy}>
          <Field label="Email" type="email" autoComplete="email" placeholder="m@example.com" defaultValue={email} {...form.bind("email")} />
        </fieldset>
        <Submit busy={form.busy} label="Send code" busyLabel="Sending code" />
      </form>
      <Switch text="Remembered it?" href={authHref("/login", { next })} label="Sign in" />
    </>
  )
}

function CodeForm({ email, next, resend, purpose }: Props & { email: string; purpose: OtpPurpose }) {
  const formId = useId()
  const formEl = () => document.getElementById(formId) as HTMLFormElement | null
  const [digits, setDigits] = useState(EMPTY_CODE)
  const reset = purpose === "reset"
  const form = useAuthForm(reset ? { code: rules.code, password: rules.newPassword } : { code: rules.code })
  const code = form.bind("code")

  const submit = form.onSubmit(async (v) => {
    try {
      await publicPost(reset ? "/v1/auth/password/reset" : "/v1/auth/verify", reset ? { email, code: v.code, password: v.password } : { email, code: v.code })
    } catch (e) {
      setDigits(EMPTY_CODE)
      formEl()?.querySelector<HTMLElement>('[data-for="code"]')?.focus()
      throw e
    }
    return leave(next)
  })

  const complete = () => {
    const el = formEl()
    const password = el?.querySelector<HTMLInputElement>('[name="password"]')
    if (password && rules.newPassword(password.value)) password.focus()
    else el?.requestSubmit()
  }

  return (
    <>
      <AuthHead title={reset ? "Choose a new password" : "Check your email"}>
        Enter the code we sent to <b>{email}</b>. It expires in 10 minutes.{reset ? " Saving signs you out everywhere else." : ""}
      </AuthHead>
      <Alert>{form.failure ? errorText(form.failure) : null}</Alert>
      <form id={formId} className={styles.form} onSubmit={submit} noValidate>
        <fieldset className={styles.fs} disabled={form.busy}>
          <CodeInput
            digits={digits}
            error={code.error}
            onChange={(d) => {
              setDigits(d)
              code.onChange()
            }}
            onComplete={complete}
          />
          {reset ? <PasswordField label="New password" autoComplete="new-password" hint="At least 10 characters." {...form.bind("password")} /> : null}
        </fieldset>
        <Submit busy={form.busy} label={reset ? "Save and sign in" : "Verify email"} busyLabel={reset ? "Saving" : "Verifying"} />
      </form>
      <ResendCode email={email} purpose={purpose} initial={resend ?? 60} />
      <Switch text="Wrong email?" href={authHref(reset ? "/forgot" : "/signup", { next })} label="Start again" />
    </>
  )
}

function MissingEmail({ next, purpose }: { next: string; purpose: OtpPurpose }) {
  const reset = purpose === "reset"
  return (
    <>
      <AuthHead title="This link is missing an email">
        {reset ? "Ask for a new reset code to continue." : "Start sign up again and we'll send you a fresh code."}
      </AuthHead>
      <Link className={styles.submit} href={authHref(reset ? "/forgot" : "/signup", { next })}>
        {reset ? "Reset password" : "Create account"}
      </Link>
    </>
  )
}

export function VerifyForm(props: Props) {
  return props.email ? <CodeForm {...props} email={props.email} purpose="signup" /> : <MissingEmail next={props.next} purpose="signup" />
}

export function ResetForm(props: Props) {
  return props.email ? <CodeForm {...props} email={props.email} purpose="reset" /> : <MissingEmail next={props.next} purpose="reset" />
}
