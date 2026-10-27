ALTER TABLE prices_current ADD COLUMN IF NOT EXISTS "sourceMarketKey" text NOT NULL DEFAULT '';
