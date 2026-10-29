import type { SVGProps } from "react"

type IconProps = { size?: number; fill?: boolean } & Omit<SVGProps<SVGSVGElement>, "fill">

function icon(paths: string, defaultSize = 16, strokeWidth = 2) {
  function Icon({ size = defaultSize, fill = false, ...rest }: IconProps) {
    return (
      <svg
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill={fill ? "currentColor" : "none"}
        stroke="currentColor"
        strokeWidth={strokeWidth}
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
        dangerouslySetInnerHTML={{ __html: paths }}
        {...rest}
      />
    )
  }
  return Icon
}

export const CheckIcon = icon('<path d="M20 6 9 17l-5-5"/>', 16)
export const ChevronRightIcon = icon('<path d="m9 18 6-6-6-6"/>', 14)
export const ChevronLeftIcon = icon('<path d="m15 18-6-6 6-6"/>', 15)
export const ChevronDownIcon = icon('<path d="m6 9 6 6 6-6"/>', 14)
export const ChevronUpIcon = icon('<path d="m18 15-6-6-6 6"/>', 14)
export const CloseIcon = icon('<path d="M18 6 6 18M6 6l12 12"/>', 15)
export const ExternalIcon = icon('<path d="M7 17 17 7M8 7h9v9"/>', 14)
export const StarIcon = icon('<path d="m12 3 2.7 5.6 6.1.9-4.4 4.3 1 6.1L12 17l-5.4 2.9 1-6.1-4.4-4.3 6.1-.9z"/>', 15)
export const BallIcon = icon('<circle cx="12" cy="12" r="9"/><path d="m12 7 4 3-1.5 4.5h-5L8 10z"/>', 18)
export const ZapIcon = icon('<path d="M13 2 4 14h7l-1 8 9-12h-7z"/>', 18)
export const TrendIcon = icon('<path d="m3 17 6-6 4 4 8-8M15 7h6v6"/>', 18)
export const PulseIcon = icon('<path d="M3 12h4l3 7 4-14 3 7h4"/>', 18)
export const ScaleIcon = icon('<path d="M12 3v18M7 21h10M5 7h14"/><path d="m5 7-3 7a3 3 0 0 0 6 0zM19 7l-3 7a3 3 0 0 0 6 0z"/>', 18)
export const GapIcon = icon('<path d="M4 7h16M4 17h16M9 7v10M15 7v10"/>', 18)
export const SteamIcon = icon('<path d="M3 17l5-5 4 3 8-9"/><path d="M14 6h6v6"/>', 18)
export const HistoryIcon = icon('<path d="M3 3v18h18"/><path d="m7 15 4-4 3 3 5-6"/>', 14)
export const TicketIcon = icon('<path d="M3 8a2 2 0 0 0 2-2h14a2 2 0 0 0 2 2v2a2 2 0 0 0 0 4v2a2 2 0 0 0-2 2H5a2 2 0 0 0-2-2v-2a2 2 0 0 0 0-4z"/><path d="M14 6v12" stroke-dasharray="2 2.5"/>', 18)
export const SearchIcon = icon('<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>', 16)
export const CalendarIcon = icon('<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M3 10h18M8 3v4M16 3v4"/>', 15)
export const InfoIcon = icon('<circle cx="12" cy="12" r="9"/><path d="M12 16v-5M12 8h.01"/>', 13)
export const UserIcon = icon('<circle cx="12" cy="8" r="4"/><path d="M4 21c1.5-4 4.5-6 8-6s6.5 2 8 6"/>', 16)

const SPORT_PATHS: Record<string, string> = {
  football: '<circle cx="12" cy="12" r="9"/><path d="m12 7 4 3-1.5 4.5h-5L8 10z"/>',
  basketball: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3v18M5.6 5.6c3.4 3.4 3.4 9.4 0 12.8M18.4 5.6c-3.4 3.4-3.4 9.4 0 12.8"/>',
  tennis: '<circle cx="12" cy="12" r="9"/><path d="M5.5 5.6c3.6 3.5 3.6 9.3 0 12.8M18.5 5.6c-3.6 3.5-3.6 9.3 0 12.8"/>',
  "american-football": '<ellipse cx="12" cy="12" rx="10" ry="6" transform="rotate(-45 12 12)"/><path d="m9 15 6-6M10.5 10.5l1.5 1.5M12 9l1.5 1.5M9 12l1.5 1.5"/>',
  esports: '<rect x="2" y="7" width="20" height="11" rx="4"/><path d="M7 11v3M5.5 12.5h3M15 12h.01M18 12h.01"/>',
}

export function SportIcon({ sport, size = 18 }: { sport: string; size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" dangerouslySetInnerHTML={{ __html: SPORT_PATHS[sport] || SPORT_PATHS.football }} />
  )
}

export function TelegramIcon({ size = 22 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true">
      <path fill="#26A5E4" d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z" />
    </svg>
  )
}

export const SIGNAL_ICONS = { arb: ZapIcon, ev: TrendIcon, line: GapIcon, move: SteamIcon } as const
