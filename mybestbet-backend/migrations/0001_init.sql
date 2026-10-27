CREATE EXTENSION IF NOT EXISTS unaccent;

CREATE TABLE leagues (
    code text PRIMARY KEY,
    name text NOT NULL,
    country text NOT NULL,
    "sofascoreId" bigint UNIQUE,
    "createdAt" timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE teams (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    "leagueCode" text NOT NULL REFERENCES leagues (code),
    name text NOT NULL,
    "normalizedName" text NOT NULL,
    "sofascoreId" bigint UNIQUE,
    "createdAt" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX teams_league_idx ON teams ("leagueCode");

CREATE TABLE team_aliases (
    source text NOT NULL,
    "sourceName" text NOT NULL,
    "teamId" uuid NOT NULL REFERENCES teams (id),
    confidence double precision NOT NULL,
    "createdAt" timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source, "sourceName")
);
CREATE INDEX team_aliases_team_idx ON team_aliases ("teamId");

CREATE TABLE players (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    "sofascoreId" bigint UNIQUE,
    name text NOT NULL,
    "normalizedName" text NOT NULL,
    "teamId" uuid REFERENCES teams (id),
    position text,
    "createdAt" timestamptz NOT NULL DEFAULT now(),
    "updatedAt" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX players_team_idx ON players ("teamId");

CREATE TABLE player_aliases (
    source text NOT NULL,
    "sourceId" text NOT NULL,
    "playerId" uuid NOT NULL REFERENCES players (id),
    confidence double precision NOT NULL,
    "createdAt" timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source, "sourceId")
);
CREATE INDEX player_aliases_player_idx ON player_aliases ("playerId");

CREATE TABLE events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    "leagueCode" text NOT NULL REFERENCES leagues (code),
    "homeTeamId" uuid NOT NULL REFERENCES teams (id),
    "awayTeamId" uuid NOT NULL REFERENCES teams (id),
    kickoff timestamptz NOT NULL,
    "sofascoreId" bigint UNIQUE,
    status text NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'live', 'finished', 'postponed', 'cancelled')),
    "createdAt" timestamptz NOT NULL DEFAULT now(),
    UNIQUE ("homeTeamId", "awayTeamId", kickoff)
);
CREATE INDEX events_league_kickoff_idx ON events ("leagueCode", kickoff);
CREATE INDEX events_kickoff_idx ON events (kickoff);
CREATE INDEX events_away_idx ON events ("awayTeamId");

CREATE TABLE event_links (
    source text NOT NULL,
    "sourceEventId" text NOT NULL,
    "eventId" uuid NOT NULL REFERENCES events (id),
    flipped boolean NOT NULL DEFAULT false,
    "createdAt" timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source, "sourceEventId")
);
CREATE INDEX event_links_event_idx ON event_links ("eventId");

CREATE TABLE prices_current (
    "marketKey" text NOT NULL,
    source text NOT NULL,
    side text NOT NULL DEFAULT '',
    label text NOT NULL DEFAULT '',
    "eventId" uuid NOT NULL REFERENCES events (id),
    code text NOT NULL,
    line double precision,
    "playerId" uuid REFERENCES players (id),
    period text NOT NULL DEFAULT '',
    raw double precision NOT NULL,
    form text NOT NULL,
    prob double precision NOT NULL,
    bid double precision,
    ask double precision,
    liquidity double precision,
    suspended boolean NOT NULL DEFAULT false,
    "deepLink" text NOT NULL DEFAULT '',
    "regulationOnly" boolean NOT NULL DEFAULT true,
    "observedAt" timestamptz NOT NULL,
    PRIMARY KEY ("marketKey", source, side, label)
);
CREATE INDEX prices_current_event_idx ON prices_current ("eventId");
CREATE INDEX prices_current_player_idx ON prices_current ("playerId") WHERE "playerId" IS NOT NULL;

CREATE TABLE prices_history (
    "marketKey" text NOT NULL,
    source text NOT NULL,
    side text NOT NULL DEFAULT '',
    label text NOT NULL DEFAULT '',
    "eventId" uuid NOT NULL,
    raw double precision NOT NULL,
    form text NOT NULL,
    prob double precision NOT NULL,
    suspended boolean NOT NULL DEFAULT false,
    "observedAt" timestamptz NOT NULL,
    PRIMARY KEY ("marketKey", source, side, label, "observedAt")
) PARTITION BY RANGE ("observedAt");
CREATE INDEX prices_history_event_idx ON prices_history ("eventId", "observedAt");
CREATE TABLE prices_history_default PARTITION OF prices_history DEFAULT;

CREATE FUNCTION ensure_prices_history_partition(day date) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    partition_name text := 'prices_history_' || to_char(day, 'YYYYMMDD');
BEGIN
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF prices_history FOR VALUES FROM (%L) TO (%L)',
        partition_name, day::timestamptz, (day + 1)::timestamptz
    );
END;
$$;

CREATE TABLE closing_prices (
    "marketKey" text NOT NULL,
    source text NOT NULL,
    side text NOT NULL DEFAULT '',
    label text NOT NULL DEFAULT '',
    "eventId" uuid NOT NULL REFERENCES events (id),
    raw double precision NOT NULL,
    prob double precision NOT NULL,
    "observedAt" timestamptz NOT NULL,
    PRIMARY KEY ("marketKey", source, side, label)
);
CREATE INDEX closing_prices_event_idx ON closing_prices ("eventId");

CREATE TABLE signals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    "dedupeKey" text NOT NULL,
    type text NOT NULL CHECK (type IN ('arb', 'ev', 'line', 'move')),
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    "eventId" uuid NOT NULL REFERENCES events (id),
    "leagueCode" text NOT NULL,
    "marketKey" text NOT NULL,
    margin double precision NOT NULL DEFAULT 0,
    ev double precision NOT NULL DEFAULT 0,
    "fairProb" double precision NOT NULL DEFAULT 0,
    "peakMargin" double precision NOT NULL DEFAULT 0,
    legs jsonb NOT NULL,
    "oldestLegAgeMs" bigint NOT NULL DEFAULT 0,
    "detectedAt" timestamptz NOT NULL,
    "closedAt" timestamptz
);
CREATE UNIQUE INDEX signals_open_dedupe_idx ON signals ("dedupeKey") WHERE status = 'open';
CREATE INDEX signals_type_detected_idx ON signals (type, "detectedAt" DESC) WHERE status = 'open';
CREATE INDEX signals_event_idx ON signals ("eventId");

CREATE TABLE review_queue (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind text NOT NULL CHECK (kind IN ('team', 'event', 'player')),
    source text NOT NULL,
    "sourceKey" text NOT NULL,
    payload jsonb NOT NULL,
    candidates jsonb NOT NULL DEFAULT '[]',
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'linked', 'created', 'ignored')),
    "decidedId" uuid,
    "decidedAt" timestamptz,
    "createdAt" timestamptz NOT NULL DEFAULT now(),
    UNIQUE (kind, source, "sourceKey")
);
CREATE INDEX review_queue_pending_idx ON review_queue ("createdAt") WHERE status = 'pending';

CREATE TABLE player_match_stats (
    "playerId" uuid NOT NULL REFERENCES players (id),
    "eventId" uuid NOT NULL REFERENCES events (id),
    "teamId" uuid REFERENCES teams (id),
    "opponentTeamId" uuid REFERENCES teams (id),
    "isHome" boolean NOT NULL,
    "playedAt" timestamptz NOT NULL,
    minutes integer NOT NULL DEFAULT 0,
    started boolean NOT NULL DEFAULT false,
    goals integer NOT NULL DEFAULT 0,
    assists integer NOT NULL DEFAULT 0,
    shots integer NOT NULL DEFAULT 0,
    "shotsOnTarget" integer NOT NULL DEFAULT 0,
    fouls integer NOT NULL DEFAULT 0,
    tackles integer NOT NULL DEFAULT 0,
    yellow integer NOT NULL DEFAULT 0,
    red integer NOT NULL DEFAULT 0,
    rating double precision,
    "valueSources" jsonb NOT NULL DEFAULT '{}',
    "updatedAt" timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY ("playerId", "eventId")
);
CREATE INDEX player_match_stats_player_played_idx ON player_match_stats ("playerId", "playedAt" DESC);
CREATE INDEX player_match_stats_event_idx ON player_match_stats ("eventId");

CREATE TABLE lineups (
    "eventId" uuid NOT NULL REFERENCES events (id),
    "playerId" uuid NOT NULL REFERENCES players (id),
    status text NOT NULL CHECK (status IN ('starter', 'bench', 'out')),
    "updatedAt" timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY ("eventId", "playerId")
);
CREATE INDEX lineups_player_idx ON lineups ("playerId");

CREATE TABLE telegram_subscriptions (
    "chatId" bigint PRIMARY KEY,
    "minArb" double precision NOT NULL DEFAULT 0.5,
    "minEv" double precision NOT NULL DEFAULT 3,
    leagues text[] NOT NULL DEFAULT '{}',
    props boolean NOT NULL DEFAULT false,
    "moveThreshold" double precision,
    "mutedUntil" timestamptz,
    "createdAt" timestamptz NOT NULL DEFAULT now(),
    "updatedAt" timestamptz NOT NULL DEFAULT now()
);
