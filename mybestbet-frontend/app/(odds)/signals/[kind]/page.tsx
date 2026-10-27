import type { Metadata } from "next"
import { notFound } from "next/navigation"
import { SignalsPage } from "@/components/mybestbet/signals/signals-page"
import { SIGNALS, signalBySlug } from "@/lib/odds/markets"

type Props = {
  params: Promise<{ kind: string }>
  searchParams: Promise<{ open?: string }>
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { kind } = await params
  const type = signalBySlug(kind)
  return {
    title: type ? SIGNALS[type].title : "Signals",
    alternates: { canonical: `/signals/${kind}` },
  }
}

export function generateStaticParams() {
  return Object.values(SIGNALS).map((s) => ({ kind: s.slug }))
}

export default async function Page({ params, searchParams }: Props) {
  const [{ kind }, { open }] = await Promise.all([params, searchParams])
  const type = signalBySlug(kind)
  if (!type) notFound()
  return <SignalsPage type={type} open={open} />
}
