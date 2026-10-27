ALTER TABLE events ADD COLUMN IF NOT EXISTS "mergedAt" timestamptz;

ALTER TABLE events ADD COLUMN IF NOT EXISTS "mergedFlipped" boolean NOT NULL DEFAULT false;
