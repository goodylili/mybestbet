package review

import (
	"context"
	"fmt"

	json "github.com/goccy/go-json"
	"github.com/jackc/pgx/v5"

	"mybestbet/pkg/canon"
)

func applyDecision(ctx context.Context, tx pgx.Tx, it storedItem, d Decision) (*canon.AliasUpdate, error) {
	if d.Action == ActionIgnore {
		return nil, nil
	}
	switch it.Kind {
	case canon.AliasTeam:
		return decideTeam(ctx, tx, it, d)
	case canon.AliasPlayer:
		return decidePlayer(ctx, tx, it, d)
	case canon.AliasEvent:
		return decideEvent(ctx, tx, it, d)
	}
	return nil, fmt.Errorf("%w: %q", errUnknownKind, it.Kind)
}

func decideTeam(ctx context.Context, tx pgx.Tx, it storedItem, d Decision) (*canon.AliasUpdate, error) {
	teamID := d.TargetID
	if d.Action == ActionCreate {
		var p TeamPayload
		if err := decodePayload(it, &p); err != nil {
			return nil, err
		}
		var err error
		if teamID, err = insertTeam(ctx, tx, p.League, p.Name); err != nil {
			return nil, err
		}
	}
	if err := upsertTeamAlias(ctx, tx, it.Source, it.SourceKey, teamID); err != nil {
		return nil, err
	}
	return &canon.AliasUpdate{Kind: canon.AliasTeam, Source: it.Source, SourceID: it.SourceKey, TargetID: teamID}, nil
}

func decidePlayer(ctx context.Context, tx pgx.Tx, it storedItem, d Decision) (*canon.AliasUpdate, error) {
	playerID := d.TargetID
	if d.Action == ActionCreate {
		var p PlayerPayload
		if err := decodePayload(it, &p); err != nil {
			return nil, err
		}
		var err error
		if playerID, err = insertPlayer(ctx, tx, p.Player.Name, p.TeamID); err != nil {
			return nil, err
		}
	}
	if err := upsertPlayerAlias(ctx, tx, it.Source, it.SourceKey, playerID); err != nil {
		return nil, err
	}
	return &canon.AliasUpdate{Kind: canon.AliasPlayer, Source: it.Source, SourceID: it.SourceKey, TargetID: playerID}, nil
}

func decideEvent(ctx context.Context, tx pgx.Tx, it storedItem, d Decision) (*canon.AliasUpdate, error) {
	var p EventPayload
	if err := decodePayload(it, &p); err != nil {
		return nil, err
	}
	eventID, flipped, err := targetEvent(ctx, tx, p, d)
	if err != nil {
		return nil, err
	}
	if err := upsertEventLink(ctx, tx, it.Source, it.SourceKey, eventID, flipped); err != nil {
		return nil, err
	}
	return &canon.AliasUpdate{Kind: canon.AliasEvent, Source: it.Source, SourceID: it.SourceKey, TargetID: eventID, Flipped: flipped}, nil
}

func targetEvent(ctx context.Context, tx pgx.Tx, p EventPayload, d Decision) (eventID string, flipped bool, err error) {
	if d.Action == ActionLink {
		home, err := eventHomeTeam(ctx, tx, d.TargetID)
		if err != nil {
			return "", false, err
		}
		return d.TargetID, home == p.AwayTeamID, nil
	}
	if p.HomeTeamID == "" || p.AwayTeamID == "" {
		return "", false, fmt.Errorf("%w: event teams are not resolved yet", ErrInvalidDecision)
	}
	eventID, err = upsertEvent(ctx, tx, p.League, p.HomeTeamID, p.AwayTeamID, p.Event.Kickoff)
	return eventID, false, err
}

func decodePayload(it storedItem, out any) error {
	if err := json.Unmarshal(it.Payload, out); err != nil {
		return fmt.Errorf("decode payload of review %s: %w", it.ID, err)
	}
	return nil
}
