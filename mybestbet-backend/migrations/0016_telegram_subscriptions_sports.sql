ALTER TABLE telegram_subscriptions ADD COLUMN IF NOT EXISTS sports text[] NOT NULL DEFAULT '{}'
    CHECK (sports <@ ARRAY['football', 'basketball', 'tennis', 'american-football', 'esports']::text[]);
