package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	json "github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

const userColumns = `id::text, email, "emailVerifiedAt", "passwordHash", name, "avatarUrl", "createdAt",
	COALESCE((SELECT array_agg(provider ORDER BY provider) FROM user_identities WHERE "userId" = users.id), '{}')`

func scanUser(row pgx.Row) (userRecord, bool, error) {
	var u userRecord
	err := row.Scan(&u.id, &u.email, &u.verifiedAt, &u.passwordHash, &u.name, &u.avatarURL, &u.createdAt, &u.identities)
	if errors.Is(err, pgx.ErrNoRows) {
		return userRecord{}, false, nil
	}
	if err != nil {
		return userRecord{}, false, fmt.Errorf("scan user: %w", err)
	}
	return u, true, nil
}

func userByID(ctx context.Context, q querier, id string) (userRecord, bool, error) {
	return scanUser(q.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

func userByEmail(ctx context.Context, q querier, email string) (userRecord, bool, error) {
	return scanUser(q.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email))
}

func verifiedUserExists(ctx context.Context, q querier, email string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE email = $1 AND "emailVerifiedAt" IS NOT NULL)`, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check user: %w", err)
	}
	return exists, nil
}

func upsertVerifiedUser(ctx context.Context, q querier, email string, passwordHash, name *string) (string, error) {
	var id string
	err := q.QueryRow(ctx, `INSERT INTO users (email, "passwordHash", name, "emailVerifiedAt") VALUES ($1, $2, $3, now())
		ON CONFLICT (email) DO UPDATE SET "emailVerifiedAt" = COALESCE(users."emailVerifiedAt", EXCLUDED."emailVerifiedAt")
		RETURNING id::text`, email, passwordHash, name).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert user: %w", err)
	}
	return id, nil
}

func setPassword(ctx context.Context, q querier, email, passwordHash string) (string, error) {
	var id string
	err := q.QueryRow(ctx, `UPDATE users SET "passwordHash" = $2,
		"emailVerifiedAt" = COALESCE("emailVerifiedAt", now()), "updatedAt" = now() WHERE email = $1 RETURNING id::text`, email, passwordHash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidCode
	}
	if err != nil {
		return "", fmt.Errorf("set password: %w", err)
	}
	return id, nil
}

func userSettings(ctx context.Context, q querier, id string) (json.RawMessage, error) {
	var settings json.RawMessage
	err := q.QueryRow(ctx, `SELECT settings FROM users WHERE id = $1`, id).Scan(&settings)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	return settings, nil
}

func replaceSettings(ctx context.Context, q querier, id string, settings json.RawMessage) (json.RawMessage, error) {
	var stored json.RawMessage
	err := q.QueryRow(ctx, `UPDATE users SET settings = $2::jsonb, "updatedAt" = now() WHERE id = $1 RETURNING settings`, id, string(settings)).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, fmt.Errorf("replace settings: %w", err)
	}
	return stored, nil
}

type googleProfile struct {
	subject   string
	email     string
	name      *string
	avatarURL *string
}

func linkGoogleUser(ctx context.Context, q querier, p googleProfile) (string, error) {
	var id string
	err := q.QueryRow(ctx, `SELECT "userId"::text FROM user_identities WHERE provider = $1 AND subject = $2`, ProviderGoogle, p.subject).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("find identity: %w", err)
	}
	err = q.QueryRow(ctx, `INSERT INTO users (email, name, "avatarUrl", "emailVerifiedAt") VALUES ($1, $2, $3, now())
		ON CONFLICT (email) DO UPDATE SET "emailVerifiedAt" = COALESCE(users."emailVerifiedAt", now()),
			name = COALESCE(users.name, EXCLUDED.name), "avatarUrl" = COALESCE(users."avatarUrl", EXCLUDED."avatarUrl"), "updatedAt" = now()
		RETURNING id::text`, p.email, p.name, p.avatarURL).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert google user: %w", err)
	}
	if _, err := q.Exec(ctx, `INSERT INTO user_identities (provider, subject, "userId", email) VALUES ($1, $2, $3, $4) ON CONFLICT (provider, subject) DO NOTHING`,
		ProviderGoogle, p.subject, id, p.email); err != nil {
		return "", fmt.Errorf("link identity: %w", err)
	}
	if err := q.QueryRow(ctx, `SELECT "userId"::text FROM user_identities WHERE provider = $1 AND subject = $2`, ProviderGoogle, p.subject).Scan(&id); err != nil {
		return "", fmt.Errorf("read identity: %w", err)
	}
	return id, nil
}

const liveCode = `email_codes."expiresAt" > now()`

type codeRow struct {
	id           string
	codeHash     []byte
	clientHash   []byte
	passwordHash *string
	name         *string
	attempts     int
	expiresAt    time.Time
}

type newCodeRow struct {
	email        string
	purpose      string
	codeHash     []byte
	clientHash   []byte
	passwordHash *string
	name         *string
	expiresAt    time.Time
}

func upsertCode(ctx context.Context, q querier, c newCodeRow) (string, bool, error) {
	var id string
	err := q.QueryRow(ctx, `INSERT INTO email_codes (email, purpose, "codeHash", "clientHash", "passwordHash", name, "expiresAt") VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (email, purpose) WHERE "consumedAt" IS NULL DO UPDATE SET id = gen_random_uuid(), "codeHash" = EXCLUDED."codeHash",
			"clientHash" = CASE WHEN `+liveCode+` THEN email_codes."clientHash" ELSE EXCLUDED."clientHash" END,
			"passwordHash" = CASE WHEN `+liveCode+` THEN email_codes."passwordHash" ELSE EXCLUDED."passwordHash" END,
			name = CASE WHEN `+liveCode+` THEN email_codes.name ELSE EXCLUDED.name END,
			attempts = 0, "expiresAt" = EXCLUDED."expiresAt", "sentAt" = NULL, "createdAt" = now()
		WHERE NOT `+liveCode+` OR email_codes."clientHash" IS NOT DISTINCT FROM EXCLUDED."clientHash"
		RETURNING id::text`, c.email, c.purpose, c.codeHash, c.clientHash, c.passwordHash, c.name, c.expiresAt).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("issue code: %w", err)
	}
	return id, true, nil
}

func reissueCode(ctx context.Context, q querier, email, purpose string, codeHash, clientHash []byte, expiresAt time.Time) (string, bool, error) {
	var id string
	err := q.QueryRow(ctx, `UPDATE email_codes SET id = gen_random_uuid(), "codeHash" = $3, attempts = 0, "expiresAt" = $5, "sentAt" = NULL, "createdAt" = now()
		WHERE email = $1 AND purpose = $2 AND "consumedAt" IS NULL AND "clientHash" IS NOT DISTINCT FROM $4 RETURNING id::text`, email, purpose, codeHash, clientHash, expiresAt).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reissue code: %w", err)
	}
	return id, true, nil
}

func markCodeSent(ctx context.Context, q querier, id string) error {
	if _, err := q.Exec(ctx, `UPDATE email_codes SET "sentAt" = now() WHERE id = $1 AND "sentAt" IS NULL`, id); err != nil {
		return fmt.Errorf("mark code sent: %w", err)
	}
	return nil
}

func attemptCode(ctx context.Context, q querier, email, purpose string) (codeRow, error) {
	var c codeRow
	err := q.QueryRow(ctx, `UPDATE email_codes SET attempts = attempts + 1 WHERE email = $1 AND purpose = $2 AND "consumedAt" IS NULL
		RETURNING id::text, "codeHash", "clientHash", "passwordHash", name, attempts, "expiresAt"`, email, purpose).
		Scan(&c.id, &c.codeHash, &c.clientHash, &c.passwordHash, &c.name, &c.attempts, &c.expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return codeRow{}, ErrInvalidCode
	}
	if err != nil {
		return codeRow{}, fmt.Errorf("attempt code: %w", err)
	}
	return c, nil
}

func consumeCode(ctx context.Context, q querier, id string) error {
	tag, err := q.Exec(ctx, `UPDATE email_codes SET "consumedAt" = now() WHERE id = $1 AND "consumedAt" IS NULL`, id)
	if err != nil {
		return fmt.Errorf("consume code: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidCode
	}
	return nil
}

type sessionRow struct {
	id              string
	userID          string
	familyID        string
	familyExpiresAt time.Time
	revokedAt       *time.Time
	replacedBy      *string
	lastUsedAt      time.Time
}

type newSessionRow struct {
	userID          string
	familyID        string
	tokenHash       []byte
	familyExpiresAt time.Time
	client          Client
}

func insertSession(ctx context.Context, q querier, s newSessionRow) (string, error) {
	id := uuid.NewString()
	_, err := q.Exec(ctx, `INSERT INTO sessions (id, "userId", "familyId", "tokenHash", "familyExpiresAt", "expiresAt", "userAgent", ip)
		VALUES ($1, $2, $3, $4, $5, $5, $6, $7)`, id, s.userID, s.familyID, s.tokenHash, s.familyExpiresAt, s.client.UserAgent, s.client.IP)
	if err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}
	return id, nil
}

func sessionByHash(ctx context.Context, q querier, tokenHash []byte, lock bool) (sessionRow, error) {
	sql := `SELECT id::text, "userId"::text, "familyId"::text, "familyExpiresAt", "revokedAt", "replacedBy"::text, "lastUsedAt" FROM sessions WHERE "tokenHash" = $1`
	if lock {
		sql += ` FOR UPDATE`
	}
	var s sessionRow
	err := q.QueryRow(ctx, sql, tokenHash).Scan(&s.id, &s.userID, &s.familyID, &s.familyExpiresAt, &s.revokedAt, &s.replacedBy, &s.lastUsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return sessionRow{}, ErrUnauthorized
	}
	if err != nil {
		return sessionRow{}, fmt.Errorf("read session: %w", err)
	}
	return s, nil
}

func markReplaced(ctx context.Context, q querier, id, successorID string, at time.Time) error {
	tag, err := q.Exec(ctx, `UPDATE sessions SET "replacedBy" = $2, "lastUsedAt" = $3 WHERE id = $1 AND "replacedBy" IS NULL`, id, successorID, at)
	if err != nil {
		return fmt.Errorf("rotate session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

func revokeFamily(ctx context.Context, q querier, familyID string) error {
	if _, err := q.Exec(ctx, `UPDATE sessions SET "revokedAt" = now() WHERE "familyId" = $1 AND "revokedAt" IS NULL`, familyID); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func revokeUserSessions(ctx context.Context, q querier, userID string) ([]string, error) {
	rows, err := q.Query(ctx, `UPDATE sessions SET "revokedAt" = now() WHERE "userId" = $1 AND "revokedAt" IS NULL RETURNING "familyId"::text`, userID)
	if err != nil {
		return nil, fmt.Errorf("revoke sessions: %w", err)
	}
	families, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("revoke sessions: %w", err)
	}
	return families, nil
}

func familyActive(ctx context.Context, q querier, familyID string) (bool, error) {
	var active bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sessions WHERE "familyId" = $1 AND "revokedAt" IS NULL AND "familyExpiresAt" > now())`, familyID).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("check session: %w", err)
	}
	return active, nil
}
