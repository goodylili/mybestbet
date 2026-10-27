package resolve

import (
	"context"
	"fmt"

	"mybestbet/pkg/canon"
)

const recordSourceLeagues = `UPDATE event_links l SET "sourceLeagueId" = v."sourceLeagueId"
FROM unnest($2::text[], $3::text[]) AS v("sourceEventId", "sourceLeagueId")
WHERE l.source = $1 AND l."sourceEventId" = v."sourceEventId" AND l."sourceLeagueId" IS DISTINCT FROM v."sourceLeagueId"`

func (r *Resolver) RecordSourceLeagues(ctx context.Context, source string, events []canon.SourceEvent) error {
	var sourceEventIDs, sourceLeagueIDs []string
	for _, ev := range events {
		if ev.SourceLeagueID != "" {
			sourceEventIDs = append(sourceEventIDs, ev.SourceID)
			sourceLeagueIDs = append(sourceLeagueIDs, ev.SourceLeagueID)
		}
	}
	if len(sourceEventIDs) == 0 {
		return nil
	}
	if _, err := r.store.pool.Exec(ctx, recordSourceLeagues, source, sourceEventIDs, sourceLeagueIDs); err != nil {
		return fmt.Errorf("record %s source leagues: %w", source, err)
	}
	return nil
}
