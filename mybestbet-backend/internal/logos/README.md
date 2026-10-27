# internal/logos

Self-hosted team crests, league logos and source logos (table `logos`, migration 0019).

- Candidates: seeding records the upstream image URL it already reads (FotMob `images.fotmob.com` team/league logos, ESPN `team.logo` / `team.logos[0]` / scoreboard `leagues[0].logos[0]`, EuroLeague `club.images.crest`, ACB `logo`, bo3 `image_url`, Liquipedia team icon `srcset` largest, OpenDota `logo_url`). Source logos come from `sources.<name>.logoUrl` in config.yaml (official apple-touch-icon / site logo; App Store icon published by the operator where the site is geo-blocked from our network: kalshi, bet9ja, betano, bet365).
- Candidate upsert is idempotent. A changed URL replaces the row (clearing sha/bytes so it refetches) only when no image is stored yet or the new URL is on the same host. A team seeded by two sources therefore keeps the first image that was stored instead of flip-flopping every run.
- The fetcher (stats process, hourly, 8 req/s, 4 workers) refetches rows without bytes or older than 7 days and backs off 24 h after a failure (`failedAt`, `lastError`). Bytes are rewritten only when the sha256 changes.
- Safety: http(s) only, no credentials in URLs, max 3 redirects, no proxy, every dialed IP must be public (checked at connect time, so DNS rebinding and redirects to internal hosts are refused), 512 KiB cap, type taken from content sniffing (png, jpeg, webp, gif, svg), never the upstream header. SVGs are served with `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'`.
- Not captured: Sofascore (geo-blocked here), ESPN tennis (only a generic ESPN icon), Flashscore participant images (the feed's `OA`/`OB` images are blank 30x30 placeholders).
