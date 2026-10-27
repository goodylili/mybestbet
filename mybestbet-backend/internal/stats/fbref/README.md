# internal/stats/fbref

FBref season aggregates per league from three HTML pages, parsed with goquery: `/comps/{id}/stats/{Slug}-Stats` (`stats_standard`), `/shooting/` (`stats_shooting`), `/misc/` (`stats_misc`). Tables are read from inside HTML comments when not rendered. Rows are keyed by the FBref player id in the player link and summed when a player appears twice (mid-season transfer inside a league). Competitions are in `DefaultCompetitions` (`id/Slug`), overridable through `sources.fbref.leagueIds`.

Fixtures: `testdata/fbref` (Premier League 2026-2027, three Tottenham players).
