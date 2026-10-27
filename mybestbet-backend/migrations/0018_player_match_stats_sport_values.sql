ALTER TABLE player_match_stats ADD COLUMN stats jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(stats) = 'object');
