package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	goredis "github.com/redis/go-redis/v9"
)

const (
	refreshGrace     = 10 * time.Second
	maxGraceHops     = 4
	familyCacheTTL   = 30 * time.Second
	familyActiveFlag = "1"
	familyRevoked    = "0"
)

var errReuse = errors.New("auth: refresh token reuse")

type issuedSession struct {
	userID          string
	familyID        string
	refreshToken    string
	familyExpiresAt time.Time
}

func familyKey(familyID string) string { return "auth:family:" + familyID }

func (s *Service) newSession(ctx context.Context, q querier, userID string, c Client) (issuedSession, error) {
	token, err := randomToken(refreshTokenLen)
	if err != nil {
		return issuedSession{}, err
	}
	issued := issuedSession{userID: userID, familyID: uuid.NewString(), refreshToken: token, familyExpiresAt: s.now().Add(s.cfg.RefreshTTL)}
	_, err = insertSession(ctx, q, newSessionRow{userID: userID, familyID: issued.familyID, tokenHash: hashToken(token), familyExpiresAt: issued.familyExpiresAt, client: c})
	return issued, err
}

func (s *Service) Refresh(ctx context.Context, refreshToken string, c Client) (Session, error) {
	if refreshToken == "" {
		return Session{}, ErrUnauthorized
	}
	if err := s.limitIP(ctx, refreshPerIP, c); err != nil {
		return Session{}, err
	}
	var issued issuedSession
	var reused bool
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		issued, err = s.rotate(ctx, tx, refreshToken, c)
		if errors.Is(err, errReuse) {
			reused = true
			return revokeFamily(ctx, tx, issued.familyID)
		}
		return err
	})
	if err != nil {
		return Session{}, err
	}
	if reused {
		s.log.WarnContext(ctx, "refresh token reuse detected, session family revoked", "family_id", issued.familyID)
		s.markRevoked(ctx, issued.familyID)
		return Session{}, ErrUnauthorized
	}
	return s.complete(ctx, issued)
}

func (s *Service) rotate(ctx context.Context, tx pgx.Tx, token string, c Client) (issuedSession, error) {
	row, err := s.liveSession(ctx, tx, token)
	if err != nil {
		return issuedSession{}, err
	}
	if row.replacedBy == nil {
		successor := s.keys.successor(token)
		successorID, err := insertSession(ctx, tx, newSessionRow{userID: row.userID, familyID: row.familyID, tokenHash: hashToken(successor), familyExpiresAt: row.familyExpiresAt, client: c})
		if err != nil {
			return issuedSession{}, err
		}
		return issuedFrom(row, successor), markReplaced(ctx, tx, row.id, successorID, s.now())
	}
	for range maxGraceHops {
		if s.now().Sub(row.lastUsedAt) > refreshGrace {
			return issuedSession{familyID: row.familyID}, errReuse
		}
		token = s.keys.successor(token)
		if row, err = s.liveSession(ctx, tx, token); err != nil {
			return issuedSession{}, err
		}
		if row.replacedBy == nil {
			return issuedFrom(row, token), nil
		}
	}
	return issuedSession{}, ErrUnauthorized
}

func (s *Service) liveSession(ctx context.Context, tx pgx.Tx, token string) (sessionRow, error) {
	row, err := sessionByHash(ctx, tx, hashToken(token), true)
	if err != nil {
		return sessionRow{}, err
	}
	if row.revokedAt != nil || !s.now().Before(row.familyExpiresAt) {
		return sessionRow{}, ErrUnauthorized
	}
	return row, nil
}

func issuedFrom(row sessionRow, token string) issuedSession {
	return issuedSession{userID: row.userID, familyID: row.familyID, refreshToken: token, familyExpiresAt: row.familyExpiresAt}
}

func (s *Service) Logout(ctx context.Context, refreshToken, accessToken string, all bool) error {
	owner, err := s.sessionOwner(ctx, refreshToken, accessToken)
	if err != nil || owner.familyID == "" {
		return err
	}
	families := []string{owner.familyID}
	if all && owner.current {
		revoked, err := revokeUserSessions(ctx, s.pool, owner.userID)
		if err != nil {
			return err
		}
		families = append(families, revoked...)
	} else if err := revokeFamily(ctx, s.pool, owner.familyID); err != nil {
		return err
	}
	s.markRevoked(ctx, families...)
	return nil
}

type sessionOwner struct {
	userID   string
	familyID string
	current  bool
}

func (s *Service) sessionOwner(ctx context.Context, refreshToken, accessToken string) (sessionOwner, error) {
	if refreshToken != "" {
		if row, err := sessionByHash(ctx, s.pool, hashToken(refreshToken), false); err == nil {
			current := row.revokedAt == nil && row.replacedBy == nil && s.now().Before(row.familyExpiresAt)
			return sessionOwner{userID: row.userID, familyID: row.familyID, current: current}, nil
		}
	}
	p, err := s.signer.parse(accessToken)
	if err != nil {
		return sessionOwner{}, nil
	}
	active, err := s.familyActive(ctx, p.SessionID)
	if err != nil {
		return sessionOwner{}, err
	}
	return sessionOwner{userID: p.UserID, familyID: p.SessionID, current: active}, nil
}

func (s *Service) familyActive(ctx context.Context, familyID string) (bool, error) {
	cached, err := s.rdb.Get(ctx, familyKey(familyID)).Result()
	if err == nil {
		return cached == familyActiveFlag, nil
	}
	if !errors.Is(err, goredis.Nil) {
		s.log.WarnContext(ctx, "session cache read failed", "err", err)
	}
	active, err := familyActive(ctx, s.pool, familyID)
	if err != nil {
		return false, err
	}
	flag, ttl := familyRevoked, s.revokedTTL()
	if active {
		flag, ttl = familyActiveFlag, familyCacheTTL
	}
	if err := s.rdb.Set(ctx, familyKey(familyID), flag, ttl).Err(); err != nil {
		s.log.WarnContext(ctx, "session cache write failed", "err", err)
	}
	return active, nil
}

func (s *Service) markRevoked(ctx context.Context, families ...string) {
	s.active.forget(families...)
	pipe := s.rdb.Pipeline()
	for _, family := range families {
		pipe.Set(ctx, familyKey(family), familyRevoked, s.revokedTTL())
	}
	if _, err := pipe.Exec(ctx); err != nil {
		s.log.WarnContext(ctx, "session revocation cache write failed", "err", err)
	}
}

func (s *Service) revokedTTL() time.Duration { return s.cfg.AccessTTL + tokenLeeway + familyCacheTTL }
