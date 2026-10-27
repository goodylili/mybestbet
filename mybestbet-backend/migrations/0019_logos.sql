CREATE TABLE logos (
    kind text NOT NULL CHECK (kind IN ('team', 'league', 'source')),
    "entityId" text NOT NULL,
    "sourceUrl" text NOT NULL,
    "contentType" text,
    sha256 text,
    bytes bytea,
    "fetchedAt" timestamptz,
    "failedAt" timestamptz,
    "lastError" text,
    "createdAt" timestamptz NOT NULL DEFAULT now(),
    "updatedAt" timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, "entityId")
);
CREATE INDEX logos_due_idx ON logos ("fetchedAt" NULLS FIRST);
