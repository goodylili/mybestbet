# internal/notify

The Telegram bot run by `cmd/notifier`. One process does two things (errgroup): long-polls `getUpdates` for commands, and consumes the `signals` stream with consumer group `notifier`.

## Commands

`/start`, `/help`, `/arbs 1.0`, `/ev 5`, `/league EPL LALIGA NBA` (no codes = all leagues), `/sport football basketball tennis american-football esports` (no names = all sports), `/props on|off` (player props of every sport), `/move 10` / `/move off`, `/mute 2h` / `/mute off`. Thresholds are percentages, stored as such in `telegram_subscriptions` (`minArb` 0.5 and `minEv` 3 are the table defaults). Every command is one upsert on `"chatId"`, so repeating it changes nothing. The sport filter is stored in `telegram_subscriptions.sports` (migration `0016`).

## Delivery rules

- Offset: `telegram:updates:offset` in Redis is written *before* an update is handled, so a restart never re-handles one (at most once; the handlers are idempotent anyway).
- Filter: open signals only; muted chats skip; league filter; sport filter (a signal's sport is its league's `sport` from `config.yaml`, else the market code's only sport; a chat with a sport filter skips signals whose sport is unknown); prop markets need `/props on`; arb compares `margin`, EV compares `ev`, move compares `margin` (the engine's relative move) against `/move`, line gaps go to chats with props on.
- Dedupe and throttle are one Lua script (`claim.go`) so they are atomic: `alert:<dedupeKey>:<chatId>` holds the last alerted strength (24 h TTL) and a re-alert needs `strength ≥ previous × (1 + realertGrowth)`; `alert:rate:<chatId>` is a sliding one-hour sorted set capped at `maxAlertsPerHour`. If `sendMessage` fails the claim is rolled back (previous strength and rate slot restored).
- Alert text: header `ARB 2.13% · NBA · basketball`, and the market line always names the period (`SPREAD -1.5 (1st quarter)`, `ML (map 1)`, `TOTAL 44.5 (full game)`).
- Stake split uses the engine's `Leg.Stake` (bankroll 100) when every leg has one, otherwise `100 · p_i / Σp`.
- Telegram errors never contain the bot token: `*url.Error` is unwrapped before wrapping.

## Config

`TELEGRAM_BOT_TOKEN` (empty = notifier idles and only serves `/healthz`), `TELEGRAM_API_URL` (defaults to `https://api.telegram.org`, tests use httptest), `PUBLIC_WEB_URL` (deep link to `/event/<id>`), `NOTIFIER_CONSUMER` (defaults to hostname), `METRICS_ADDR` (default `:9105`).
