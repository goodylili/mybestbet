CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL UNIQUE CHECK (email = lower(btrim(email))),
    "emailVerifiedAt" timestamptz,
    "passwordHash" text,
    name text,
    "avatarUrl" text,
    settings jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(settings) = 'object'),
    "failedLogins" int NOT NULL DEFAULT 0,
    "lockedUntil" timestamptz,
    "createdAt" timestamptz NOT NULL DEFAULT now(),
    "updatedAt" timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_identities (
    provider text NOT NULL,
    subject text NOT NULL,
    "userId" uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email text,
    "createdAt" timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, subject)
);
CREATE INDEX user_identities_user_idx ON user_identities ("userId");

CREATE TABLE sessions (
    id uuid PRIMARY KEY,
    "userId" uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    "familyId" uuid NOT NULL,
    "tokenHash" bytea NOT NULL UNIQUE,
    "familyExpiresAt" timestamptz NOT NULL,
    "expiresAt" timestamptz NOT NULL,
    "revokedAt" timestamptz,
    "replacedBy" uuid,
    "userAgent" text,
    ip text,
    "createdAt" timestamptz NOT NULL DEFAULT now(),
    "lastUsedAt" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions ("userId");
CREATE INDEX sessions_family_idx ON sessions ("familyId");

CREATE TABLE email_codes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL,
    purpose text NOT NULL CHECK (purpose IN ('signup', 'reset')),
    "codeHash" bytea NOT NULL,
    "passwordHash" text,
    name text,
    attempts int NOT NULL DEFAULT 0,
    "expiresAt" timestamptz NOT NULL,
    "consumedAt" timestamptz,
    "sentAt" timestamptz,
    "createdAt" timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX email_codes_live_idx ON email_codes (email, purpose) WHERE "consumedAt" IS NULL;
