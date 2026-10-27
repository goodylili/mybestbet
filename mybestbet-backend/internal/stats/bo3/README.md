# internal/stats/bo3

Esports series and per-player kills from the public bo3.gg API (`https://api.bo3.gg/api/v1`, plain JSON, no key) for `CS2`, `VALORANT` and `LOL` (bo3 `discipline_id` 1, 2, 3). Dota 2 stays on OpenDota.

| Call | Endpoint |
| --- | --- |
| `Matches` | `/matches?filter[matches.discipline_id][eq]=&filter[matches.start_date][gt]=&filter[matches.start_date][lt]=&filter[matches.tier][in]=&with=teams` (paged with `page[limit]`/`page[offset]`) |
| `Match` | `/matches/{slug}?with=teams` |
| `Roster` | `/teams/{slug}?with=players` (coaches dropped via `is_coach`; benched players, `status` 2, are kept) |
| `PlayerLines` CS2 | `/matches/{slug}/players_stats`: one row per player with series `kills`; maps played = `steam_profile.game_steam_profiles` |
| `PlayerLines` Valorant / LoL | `/{vlr,lol}/stats/matches/{id}/players_stats` (one entry per game with `vlr_game_players` / `lol_game_players`) + `/{vlr,lol}/stats/matches/{id}/lineup` for names; kills are summed over finished games and `maps` counts them |

## External facts

- Filter syntax is `filter[<table>.<column>][<op>]` with ops `eq, in, gt, lt, gte, lte, not_eq, overlap, like`; unknown filters are silently ignored (for example `players.team_id`), so check a filter actually narrows `total.count`.
- The CS2 match `players_stats` endpoint only answers by slug (by numeric id it returns `[]`); the Valorant/LoL stats endpoints only answer by numeric id.
- CS2 player rows link a Steam profile to a bo3 player (`steam_profile.player`); an unlinked profile falls back to source id `steam:<profile id>`.
- Valorant game rows key players by `vlr_player_id`, the lineup maps that to the bo3 player (`vlr_player.player`). LoL rows carry `player_id` directly (`lol_player_id` is null).
- `tier` is `s`/`a`/`b`/`c`/`d`; config `extra.tiers` (default `s,a`) limits seeding and lookups to the tiers books price.
- Player ids (`player.id`) are the source ids for `player_aliases` source `bo3`; names are in-game handles.

## Why not the other sources

HLTV answers 403 to non-browser clients (Cloudflare). vlr.gg HTML and gol.gg are reachable but HTML only; Leaguepedia's cargo API rate-limits anonymous clients after a couple of calls; lolesports needs its persisted-gateway key. bo3.gg covers all three games with one typed JSON API, so it is the single source.

Fixtures in `testdata/bo3` are real responses (2026-10-24) trimmed to the fields read (Valorant round data and LoL item/rune data removed).
