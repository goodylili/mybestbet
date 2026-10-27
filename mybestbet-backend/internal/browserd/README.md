# browserd

Pool of headless Chrome (go-rod + go-rod/stealth) that loads pages for sources without a usable API and returns what
the page fetched.

## API (same mux as /healthz and /metrics)

`POST /fetch` with `sources.BrowserRequest` -> `sources.BrowserResult`.

- One Chrome process per `profile`, with a persistent user data dir at `$BROWSERD_PROFILE_DIR/<profile>`, so cookies
  and anti-bot tokens survive restarts. Profile names are `[A-Za-z0-9][A-Za-z0-9_-]{0,31}`.
- One tab is kept open per URL (max 8 per profile, LRU); a repeat fetch reloads that tab instead of opening a new one.
- XHR/fetch responses whose URL contains any `capturePath` substring are returned with status and body; with
  `captureWs` every websocket frame received during the wait is returned (`ws: true`).
- `headers` carries the browser User-Agent plus User-Agent/Accept/Accept-Language/Referer/Origin/`X-*` request headers
  of the first captured request; `cookies` are the page cookies. Callers use these to replay requests directly.
- Navigation is paced per profile: at least 2s between loads plus up to 1.5s random jitter.

Errors: 400 invalid request, 403 host not allowed, 503 pool closed, 504 timeout, 502 other browser failures.

## Environment

| Var | Default |
| --- | --- |
| BROWSERD_ADDR | `:9222` |
| BROWSERD_POOL | `2` (max concurrent fetches and max live Chrome processes; idle profiles are evicted LRU) |
| BROWSERD_PROFILE_DIR | `$TMPDIR/browserd-profiles` |
| BROWSERD_ALLOWED_HOSTS | `bet365.com,prizepicks.com,sofascore.com,fbref.com` (suffix match; blocks SSRF to anything else) |
| BROWSERD_CHROME_BIN | system Chrome if found, else rod downloads Chromium |

Chrome is launched with `--remote-debugging-port=0`, so it never collides with the API port 9222.

## Tests

Unit tests cover request validation, capture matching and the handler without Chrome.
`BROWSERD_CHROME_TEST=1 go test ./internal/browserd/` runs a real-Chrome capture against a local page.
