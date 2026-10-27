# kalshi

REST at `baseUrl` (`https://api.elections.kalshi.com/trade-api/v2`), reads need no auth. Websocket needs an API key.

## Flow

- Every competition has a series prefix; its game series is `<prefix>GAME` (`<prefix>MATCH` for tennis). Every other market series of the same game shares the game key after the dash (`KXNBAGAME-26OCT23HOUSAS` -> `KXNBASPREAD-26OCT23HOUSAS`, `KXNBAPTS-26OCT23HOUSAS`). Per-map / per-set series append `-1` (`KXCS2MAP-26OCT240300MGLZAUR-1`, `KXATPSETWINNER-26OCT24MEDBUB-1`).
- Discover: `GET /events?series_ticker=<game series>&status=open&with_milestones=true&with_nested_markets=true` (cursor paged, one call per game series even when several leagues share it). Source event id = game event ticker.
  - Kickoff: milestone `start_date` keyed by `details.main_game_event_ticker`. EuroLeague/EuroCup/ACB/NBL/Copa del Rey events often have no milestone; then the key's `YYMMMDDHHMM` stamp is used, which is US Eastern time (the market rules say "12:00 PM EDT" for `...261200...`). Events with neither are skipped.
  - Competitors come from the game markets: each non-`TIE` market's ticker suffix is the team code, `yes_sub_title` the name, `custom_strike.<sport>_team` / `tennis_competitor` / `esports_competitor` the id. Listing order is the order of the codes in the key (`HOUSAS` = HOU then SAS); falls back to title prefix, then to splitting `title` on ` vs ` / ` vs. ` when the listing has no markets.
  - Orientation: basketball and NFL/NCAAF titles list the AWAY team first (milestones read "HOU Rockets at SA Spurs", verified for NBA, WNBA, NCAAB, NFL, NCAAF, EuroLeague, EuroCup, ACB, NBL), so those families flip. Soccer titles are home first; tennis/esports keep first-listed as home.
  - Tennis: one series per tour (`KXATPMATCH`, `KXWTAMATCH`, `KXATPCHALLENGERMATCH`); the league is chosen by `product_metadata.competition`.
- Price: for each game, each series of its family is listed once per call (`GET /events?series_ticker=<series>&status=open&with_nested_markets=true`), cached across the requested games, and the event with the matching key is mapped. Missing series/events are simply absent.
- Prices arrive as dollar strings (`yes_bid_dollars`, `yes_ask_dollars`, sizes `*_size_fp`), converted to cents (`FormCents`, Prob = cents/100). The No side is the complement of the Yes book: No ask = 1 - Yes bid with Yes bid size. Liquidity is dollars at the touch.
- Sides of spreads and player props come from `custom_strike` team ids (fallback: team code prefix of the ticker suffix).

## Market table (series suffix after the league prefix)

| Family | Series | Canon | Notes |
| --- | --- | --- | --- |
| soccer | `GAME` | 1X2 | suffix = team code or `TIE` |
| soccer | `1H` | 1H_1X2 | |
| soccer | `SCORE` | CS | label `custom_strike.home_score-away_score` |
| soccer | `TOTAL` | OU | `floor_strike`, `strike_type` `greater` |
| soccer | `CORNERS` | CORN_OU | `greater_or_equal` N -> line N-0.5 |
| soccer | `BTTS` | BTTS | |
| soccer | `SPREAD` | AH | `ARS2` = ARS wins by more than strike: Line = -strike Yes home; away flips |
| soccer | `ADVANCE` | QUALIFY | two-way "advances", includes ET/pens (RegulationOnly=false) |
| basketball, gridiron | `GAME` | ML | dropped if a `TIE` outcome exists |
| basketball, gridiron | `SPREAD` / `TOTAL` | SPREAD / TOTAL | |
| NBA-style, gridiron | `1HSPREAD` `2HSPREAD` `1QSPREAD`..`4QSPREAD` | SPREAD period 1H/2H/Q1..Q4 | |
| NBA-style, gridiron | `1HTOTAL` `2HTOTAL` `1QTOTAL`..`4QTOTAL` | TOTAL period 1H/2H/Q1..Q4 | |
| NBA-style | `PTS` `REB` `AST` `3PT` `PRA` | BK_PTS BK_REB BK_AST BK_3PM BK_PRA | ladder "N+" = over N-0.5 (`floor_strike`); player = `custom_strike.basketball_player`, name before `:` in `yes_sub_title` |
| gridiron | `PASSYDS` `RSHYDS` `RECYDS` `REC` `PASSTDS` | NF_PASS_YDS NF_RUSH_YDS NF_REC_YDS NF_REC NF_PASS_TD | same ladder shape |
| gridiron | `TD` | NF_ANYTIME_TD | only the 1+ rung (strike 0.5) as yes/no; 2+ dropped |
| tennis | `MATCH` | ML | |
| tennis | `SETWINNER` + `-1` | ML period S1 | |
| tennis | `GTOTAL` | TOTAL_GAMES | |
| esports | `GAME` | ML | |
| esports | `MAP` + `-1` | ML period M1 | LoL calls it "Game 1 Winner" |
| esports | `SPREAD` | MAP_HCP | `INOX2` = wins by over 1.5 maps |
| esports | `TOTALMAPS` | TOTAL_MAPS | |

Families: NBA-style = NBA (`KXNBA`), WNBA (`KXWNBA`), NCAAB (`KXNCAAMB`); basketball core (GAME/SPREAD/TOTAL only) = EUROLEAGUE `KXEUROLEAGUE`, EUROCUP `KXEUROCUP`, ACB `KXACB`, NBL `KXNBL`; gridiron = NFL `KXNFL`, NCAAF `KXNCAAF`; esports = CS2 `KXCS2`, DOTA2 `KXDOTA2`, VALORANT `KXVALORANT`, LOL `KXLOL`.

## League table

Soccer prefixes: WC `KXWC`, FRIENDLY `KXINTLFRIENDLY`, UCL `KXUCL`, UEL `KXUEL`, UECL `KXUECL`, UWCL `KXUCLW`, EPL `KXEPL`, CHAMP `KXEFLCHAMPIONSHIP`, FACUP `KXFACUP`, EFLCUP `KXEFLCUP`, LALIGA `KXLALIGA`, COPADELREY `KXCOPADELREY`, SERIEA `KXSERIEA`, COPPAITALIA `KXCOPPAITALIA`, BUNDES `KXBUNDESLIGA`, DFBPOKAL `KXDFBPOKAL`, LIGUE1 `KXLIGUE1`, CDF `KXCOUPEDEFRANCE`, ERED `KXEREDIVISIE`, PRIMEIRA `KXLIGAPORTUGAL`, SUPERLIG `KXSUPERLIG`, SPFL `KXSCOTTISHPREM`, JPL `KXBELGIANPL`, MLS `KXMLS`, LIGAMX `KXLIGAMX`, BRA1 `KXBRASILEIRO`, BRA2 `KXBRASILEIROB`, ARG1 `KXARGPREMDIV`, SPL `KXSAUDIPL`, SSL `KXSWISSLEAGUE`, GSL `KXSLGREECE`, ELITESERIEN `KXELITESERIEN`, DSL `KXDENSUPERLIGA`, J1 `KXJLEAGUE`, J2 `KXJ2LEAGUE`, KLEAGUE `KXKLEAGUE`, CSL `KXCHNSL`, ISL `KXINDSL`, ALEAGUE `KXALEAGUE`, LEAGUESCUP `KXLEAGUESCUP`, LIBERTADORES `KXCONMEBOLLIB`, SUDAMERICANA `KXCONMEBOLSUD`, SUPERCOPA `KXESPSUPERCUP`.

Tennis competitions (`product_metadata.competition`, names taken from settled events): ATP_MIAMI "ATP Miami", ATP_RG "French Open Men Singles", ATP_WIM "Wimbledon Men Singles", ATP_AO "Australian Open Men Singles", ATP_USO "US Open Men Singles", WTA_* the same with "WTA Miami" / "Women Singles"; challengers on `KXATPCHALLENGERMATCH` as "ATP Challenger Zadar/Murcia/Morelos/Asuncion".

`leagueIds` overrides a known code: the value replaces the series prefix, or the competition name for tennis codes. Unknown codes are ignored.

## Stream

`Stream` returns `ErrStreamNeedsAPIKey` immediately when `SOURCE_KALSHI_API_KEY` is unset; the error has a `PollsInstead() bool` method, which the ingestor detects with `errors.As` and then polls REST without retrying the stream. With a key, `SOURCE_KALSHI_API_SECRET` must hold the PEM private key (RSA PKCS#1/PKCS#8 or Ed25519). The handshake sends `KALSHI-ACCESS-KEY`, `KALSHI-ACCESS-TIMESTAMP` (ms) and `KALSHI-ACCESS-SIGNATURE` = base64 RSA-PSS-SHA256 (salt = hash length) over `timestamp + "GET" + /trade-api/ws/v2`. It subscribes `{"cmd":"subscribe","params":{"channels":["ticker"],"market_tickers":[...]}}`; `ticker` messages carry the Yes touch and sizes. Sessions resync from REST and reconnect every 5 minutes. `Capabilities().Push` is true only when a key is configured.

## Thin books

A selection is `Suspended` unless both sides of its book are quoted, the best ask has size and ask - bid is at most 0.10 dollar.

## Known gaps

- Half/quarter winner series (`1H`, `2H`, `1Q`..`4Q` in basketball/NFL) always carry a `TIE` outcome, so they are 3-way regulation lines, not ML, and are not requested.
- Tennis game spreads (`GSPREAD`) are a games handicap, which has no canon code; set spreads (`SSPREAD`), total sets and aces/double faults (`KXATPACES`, `KXWTAACES`) had no open events and are not mapped (no SET_HCP, TN_ACES, TN_DF).
- Esports: `KXLOLKILLS` is a team total, not a player prop, so ES_KILLS is not supplied; only map 1 winners are mapped (canon has M1 only); DOTA2/LOL spreads had no open events.
- Soccer player props (`GOAL`, `FIRSTGOAL`, `SOA`, `AST`) had no open events at capture and are not mapped. Many non-EPL soccer leagues only list `GAME`/`ADVANCE`; the other series simply return nothing.
- UPL and RPL have no Kalshi series. None of the configured tennis tournaments were running at capture (Oct 2026), so the tennis fixtures are ATP Almaty/Basel and the test maps ATP_MIAMI to "ATP Almaty" via `leagueIds`.
- Fixtures are real captures from 2026-10-24 trimmed to a few events/markets; football event fixtures were regrouped into series listings (`markets_<series>.json`). Websocket frames in `ws_frames.jsonl` are synthesized from the documented `ticker` schema (no key available here).

## Event link (`links.go`)

`EventURL` builds `https://kalshi.com/markets/{series}/{home}-vs-{away}/{event_ticker}` (lowercase). Checked 2026-10-27: opens the exact game (title "Manchester United vs. Chelsea Odds ..."); the middle slug is free text, but the bare `/markets/{event_ticker}` form does not resolve.
