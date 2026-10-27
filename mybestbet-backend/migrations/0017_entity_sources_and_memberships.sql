CREATE TABLE team_leagues (
    "teamId" uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    "leagueCode" text NOT NULL REFERENCES leagues (code),
    "createdAt" timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY ("teamId", "leagueCode")
);
CREATE INDEX team_leagues_league_idx ON team_leagues ("leagueCode");

INSERT INTO team_leagues ("teamId", "leagueCode")
SELECT id, "leagueCode" FROM teams
ON CONFLICT DO NOTHING;

CREATE TABLE team_sources (
    source text NOT NULL,
    "sourceId" text NOT NULL,
    "teamId" uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    "createdAt" timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source, "sourceId")
);
CREATE INDEX team_sources_team_idx ON team_sources ("teamId");

INSERT INTO team_sources (source, "sourceId", "teamId")
SELECT 'sofascore', "sofascoreId"::text, id FROM teams WHERE "sofascoreId" IS NOT NULL
ON CONFLICT DO NOTHING;

CREATE TABLE event_sources (
    source text NOT NULL,
    "sourceId" text NOT NULL,
    "eventId" uuid NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    "createdAt" timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source, "sourceId")
);
CREATE INDEX event_sources_event_idx ON event_sources ("eventId");

INSERT INTO event_sources (source, "sourceId", "eventId")
SELECT 'sofascore', "sofascoreId"::text, id FROM events WHERE "sofascoreId" IS NOT NULL
ON CONFLICT DO NOTHING;

CREATE TABLE player_teams (
    "playerId" uuid NOT NULL REFERENCES players (id) ON DELETE CASCADE,
    "teamId" uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    "createdAt" timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY ("playerId", "teamId")
);
CREATE INDEX player_teams_team_idx ON player_teams ("teamId");

INSERT INTO player_teams ("playerId", "teamId")
SELECT id, "teamId" FROM players WHERE "teamId" IS NOT NULL
ON CONFLICT DO NOTHING;

CREATE INDEX teams_league_normalized_idx ON teams ("leagueCode", "normalizedName");
CREATE INDEX players_team_normalized_idx ON players ("teamId", "normalizedName");
