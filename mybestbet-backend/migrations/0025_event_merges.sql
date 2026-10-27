ALTER TABLE events ADD COLUMN IF NOT EXISTS "mergedInto" uuid REFERENCES events(id);

CREATE INDEX IF NOT EXISTS events_merged_into_idx ON events ("mergedInto") WHERE "mergedInto" IS NOT NULL;
