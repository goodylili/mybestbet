# mybestbet architecture

How the Next.js app is put together, and where each piece of the old single-file build went. Read this before adding a screen.

## Layout

```
app/
  layout.tsx              fonts, metadata, JSON-LD, the pre-paint theme script
  globals.css             tokens (dark default, light under html[data-theme="light"]) and shared primitives
  page.tsx                the landing page
  (odds)/layout.tsx       wraps every odds screen in <AppShell>
  (odds)/sports/[sport]   match list for a sport
  (odds)/leagues/[code]   match list for one competition
  (odds)/favourites       starred matches
  (odds)/events/[id]      match page (?market=1X2, ?market=PROPS for player props)
  (odds)/players/[id]     game log (?event=&key= ties it to a prop)
  (odds)/signals/[kind]   arbs | value | lines | moves (?open= expands one)
  (odds)/sources          feed health
  (auth)/layout.tsx       the sign-in frame, outside the AppShell
  (auth)/login, signup, verify, forgot, reset
proxy.ts                  redirects signed-out visitors away from odds routes
components/
  ui/                     shadcn primitives
  mybestbet-wordmark.tsx  the lowercase "mybestbet" wordmark
  mybestbet/
    shared/               icons, logos (Crest, BookLogo, LeagueMark), FitRow, Pager, Sparkline, Countdown, status
    shell/                AppShell, top bar, notch, sidebar rail, bet slip, settings sheet, tab bar, ShellProvider
    onboarding/           welcome gate and spotlight tour
    landing/              the landing page and its live previews
    auth/                 sign-in frame, form kit (fields, code input, resend countdown) and the five auth forms
    matches/              match list, rows, calendar
    match/                match page, odds grid, price history, props
    player/               game log
    signals/              arbs, value bets, line gaps, steam moves, stake calculator
    sources/              sources page
lib/
  api/                    types, client (same-origin /api, refresh on 401, sample data switch), snapshot mode
  auth/                   safe `next` targets and auth links
  odds/                   formatting, time, market labels, hashing
  store/                  data store, viewer prefs and slip, selectors, change emitter
  catalog.ts              team tags, logos, banners, competitions from lib/data/catalog.json
  routes.ts               every URL in one place
public/mybestbet/         fonts, logos, banners, photos, hero video, sample API responses
scripts/                  extract-assets.py
```

## Data

The store is a plain module, not React state. `lib/store/data.ts` holds `S` (sports, leagues, events, sources, signals, event details, props, player stats, price history, price flashes) and every loader. Loaders mutate `S` and call `emit()`. Components call `useStore()` from `lib/store/emitter.ts` to re-render on any change, then read `S` directly.

```tsx
"use client"

import { useStore } from "@/lib/store/emitter"
import { S, dayFixtures } from "@/lib/store/data"

export function Count() {
  useStore()
  return <span>{dayFixtures(0).length}</span>
}
```

`AppShell` calls `startSession()` from `lib/store/session.ts`, which hydrates prefs, loads `/v1/me` and only then calls `startStore()`, and syncs settings with `/v1/me/settings`. The landing page calls `startSession(true)`: signed out, it switches the client to the bundled snapshot so it never calls protected endpoints. The store loads sports, sources, near-term events and signals, opens the live stream, then polls events every 30s and signals and sources every 45s.

Per-screen loads are triggered from effects: `loadDetail(id)`, `loadProps(id)`, `loadStats(pid)`, `loadHistory(key, src)`, `want(id)` (queued detail for list rows), `wantProps(id)`, `ensureDay(d)`. Each is deduplicated, so calling them on every render is harmless. `watchEvent(id)` streams live prices for the open match; `watchExtra(ids)` does the same for the landing page.

`lib/store/prefs.ts` holds what the viewer chose: odds format, books switched off, favourites, display toggles, theme, the slip, and the line and market picked on the match list. Changes persist to localStorage and emit.

`lib/store/selectors.ts` has the derived views: `rowBoard`, `defaultBoard`, `lineLabel`, `listLines`, `fairOdds`, `chances`, `signalMarket`, `arbStakes`, `pickInfo`, `mainKey`, `boardBest`.

Sample data: with `NEXT_PUBLIC_MBB_SNAPSHOT=1` the client reads `public/mybestbet/snapshot/` instead of the API, shifts every timestamp so kick-offs stay upcoming, fakes price history from loaded boards, and nudges prices every 1.3s.

## Shell state

`useShell()` from `components/mybestbet/shell/shell-context.tsx` carries what several parts of the odds app share: the search text `q`, the chosen day `date`, the list `filter` (all, soon, arb, ev), and whether the slip or settings sheet is open. Anything that belongs to one screen (open rows, the current page, the chosen period) is local state and resets when you leave.

## Styling

- Tokens and the shared primitives (`.btn`, `.chip`, `.seg`, `.mtabs`, `.card`, `.empty`, `.badge`, `.mono`, `.faint`, `.muted`, `.sr`, `.dot`, `.ib`, `.kbd`, logo image classes) are global in `app/globals.css`.
- Everything else is a CSS module next to its component, with camelCase class names, as in `components/prydd/*.module.css` upstream.
- Theme-dependent rules use `:global(:root[data-theme="light"]) .x` or `:global(:root:not([data-theme="light"])) .x`.
- Body flags set from settings (`no-prob`, `no-flash`, `no-cd`) are matched with `:global(body.no-flash) .x`.

## Lists

Every list shows 10 rows a page through `usePage(items, resetKey)` and `<Pager>` from `components/mybestbet/shared/pager.tsx`. `resetKey` combines the filters, so changing one returns to page 1.

## Conventions

- Files are kebab-case; components are named exports; client components start with `"use client"`.
- Double quotes, no semicolons in components and lib code.
- Brand copy: lowercase "mybestbet", no em dashes, positives in blue, the site is an educational demo that is not affiliated with any book, league or team.
