CREATE EXTENSION IF NOT EXISTS pg_trgm;

ALTER TABLE events
    ADD COLUMN IF NOT EXISTS score jsonb CHECK (score IS NULL OR jsonb_typeof(score) = 'object'),
    ADD COLUMN IF NOT EXISTS "liveUpdatedAt" timestamptz;

CREATE INDEX IF NOT EXISTS teams_normalized_trgm_idx ON teams USING gin ("normalizedName" gin_trgm_ops);
CREATE INDEX IF NOT EXISTS players_normalized_trgm_idx ON players USING gin ("normalizedName" gin_trgm_ops);
