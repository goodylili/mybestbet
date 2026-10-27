ALTER TABLE leagues ADD COLUMN sport text NOT NULL DEFAULT 'football'
    CHECK (sport IN ('football', 'basketball', 'tennis', 'american-football', 'esports'));
CREATE INDEX leagues_sport_idx ON leagues (sport);
