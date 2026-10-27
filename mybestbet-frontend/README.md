# mybestbet

The web front end for mybestbet: a landing page and an odds app that puts every sportsbook's and prediction market's price side by side, and flags arbitrage, value bets, line gaps and steam moves as they happen.

mybestbet is an educational demo. It is not a bookmaker, and it is not affiliated with or licensed by any book, league or team shown. Names and logos belong to their owners.

## Overview

This is a [Next.js](https://nextjs.org) app (App Router). `/` is the landing page. Everything else is the odds app, which needs a signed-in account. It reads the mybestbet API (OpenAPI 3.1, `/v1/*`) from the browser at the same origin under `/api` and keeps prices live over its event stream.

| Route | Screen |
|---|---|
| `/` | Landing page (sample data when signed out, live when signed in) |
| `/login`, `/signup` | Email and password, or Continue with Google |
| `/verify?email=` | 6-digit email code after sign up |
| `/forgot`, `/reset?email=` | Password reset by emailed code |
| `/sports/[sport]` | Match list for a sport, by day, with market tabs and line picker |
| `/leagues/[code]` | Match list for one competition |
| `/favourites` | Starred matches |
| `/events/[id]` | Every book's price for a match, price history, player props (`?market=PROPS`) |
| `/players/[id]` | A player's last 20 matches against a prop line |
| `/signals/arbs`, `/value`, `/lines`, `/moves` | Arbitrage with a stake calculator, value bets, line gaps, steam moves |
| `/sources` | Feed health per book |

## Tech stack

- **Framework:** Next.js 16, React 19, TypeScript
- **Styling:** CSS modules per component, tokens and shared primitives in `app/globals.css`, Tailwind CSS v4 available
- **Data:** the mybestbet API over `fetch` and `EventSource`, held in a small external store (`lib/store`)
- **Fonts:** Mona Sans and Outfit, self-hosted
- **Analytics:** Vercel Analytics
- **Auth:** HttpOnly session cookies set by the API; `proxy.ts` sends signed-out visitors to `/login`
- **Hosting:** Docker (`output: "standalone"`), behind Caddy

## Getting started

### 1. Install dependencies

```bash
pnpm install
```

### 2. Configure environment variables

Copy `.env.example` to `.env.local`:

```bash
API_URL="http://localhost:8080"
NEXT_PUBLIC_MBB_SNAPSHOT="0"
```

The browser always calls the API at `/api` on the site's own origin, so session cookies stay first party and no CORS is needed. In development `next.config.mjs` rewrites `/api/:path*` to `API_URL` (default `http://localhost:8080`). In production Caddy proxies `/api/*` to the API before requests reach Next. Rewrites are fixed at build time, so pass `API_URL` as a build argument if the image needs a different target.

Set `NEXT_PUBLIC_MBB_SNAPSHOT="1"` to run on the bundled sample responses in `public/mybestbet/snapshot/` with no API at all. Timestamps are moved forward so kick-offs stay upcoming, and prices tick on their own. The sign-in guard is off in this mode.

### 3. Run the dev server

```bash
pnpm dev
```

Open [http://localhost:4040](http://localhost:4040). The API must allow `http://localhost:4040` as an origin and use insecure cookies (`AUTH_SECURE_COOKIES=false`) for local http.

### Checks

```bash
pnpm lint
pnpm typecheck
pnpm build
```

### Docker

```bash
docker build -t mybestbet-web .
docker run -p 3000:3000 mybestbet-web
```

The image is a multi-stage `node:22-alpine` build of the standalone output. It runs `node server.js` as the unprivileged `node` user on port 3000.

## Sign-in

- The API sets an access cookie (15 minutes) and a refresh cookie (30 days). Both are HttpOnly; the app never sees the tokens.
- `proxy.ts` redirects odds routes to `/login?next=` when neither cookie is present, and sends signed-in visitors on `/login` or `/signup` to `next` or `/sports/football`. The API is the real gate.
- `lib/api/client.ts` retries a request once after a single shared `POST /api/v1/auth/refresh` on a 401. If the refresh fails it signs out and returns to `/login?next=`.
- `lib/store/session.ts` loads `/v1/me` before the data store starts, syncs viewer settings with `/v1/me/settings` (a full `PUT` 800 ms after the last change, with localStorage as the offline copy) and signs out.
- The live stream refreshes the session and reconnects whenever it drops, since the API closes streams when the access token expires.
- "Continue with Google" shows when `GET /v1/meta` reports `auth.google`.

## Project structure

```
app/                 routes, root layout, global styles, robots and sitemap
  (odds)/            the odds app, wrapped in the shared shell
  (auth)/            sign in, sign up, email code, password reset
components/
  ui/                shadcn/ui primitives
  mybestbet/         every mybestbet screen, grouped by area (see docs/architecture.md)
lib/
  api/               API types, client and sample data mode
  auth/              safe redirect targets and auth page links
  odds/              odds formatting, dates, market labels
  store/             data store, viewer settings and bet slip, selectors
  data/catalog.json  team tags, competitions and logo paths, generated
public/mybestbet/    fonts, logos, banners, photos, hero video, sample responses
scripts/             asset extraction
proxy.ts             sign-in guard for the odds routes
docs/                architecture notes
```

## What reads what

| Screen | Endpoints |
|---|---|
| Session, account in settings | `/v1/me`, `/v1/me/settings`, `/v1/auth/*`, `/v1/meta` |
| Sidebar, sport and league lists, market tabs | `/v1/sports` |
| Match list, day counts, calendar | `/v1/events?from&to` (next 3 days on load, the rest of 30 days when a later day is opened) |
| Match page, period and line switches, consensus row, prediction market depth | `/v1/events/{id}`, live via `/v1/stream?event={id}` |
| Price history | `/v1/markets/{key}/history?source=` |
| Player props and research panel | `/v1/events/{id}/props` |
| Game log | `/v1/players/{id}/stats?last=20` |
| Arbitrage, value bets, line gaps, steam moves | `/v1/signals`, live via `/v1/stream` |
| Sources page, books in settings | `/v1/sources` |

Polling backs up the streams: near-term events every 30 s, signals and source health every 45 s.

## Assets

Logos, banners, photos and the hero video live in `public/mybestbet/`. `lib/data/catalog.json` maps team names, competitions and books to them. Both are generated from the asset packs:

```bash
python3 scripts/extract-assets.py ../mybestbet-mono/packages/assets --snapshot
```

`--snapshot` also unpacks the sample API responses.

## Assumptions to check against the real API

- Prop codes map to stat fields in `PROP_STAT` in `lib/odds/markets.ts` (`P_SOT` to `shotsOnTarget` and so on). Add the real codes there.
- On `move` signals, `margin` is read as the price change as a fraction (negative means the price shortened). Without it the row shows "Moved" and the history chart tells the rest.
- On a list row with no event detail loaded, the main market's key is built as `eventId|code|||`.
