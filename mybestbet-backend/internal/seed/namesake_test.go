package seed

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/names"
	"mybestbet/pkg/canon"
)

func TestUniqueNamesake(t *testing.T) {
	tests := []struct {
		name       string
		normalized string
		candidates []namedTeam
		want       string
	}{
		{name: "short established name", normalized: "wolverhampton wanderers", candidates: []namedTeam{{"wolves", "wolverhampton"}, {"bolton", "bolton wanderers"}}, want: "wolves"},
		{name: "longer established name", normalized: "mainz", candidates: []namedTeam{{"mainz", "fsv mainz"}}, want: "mainz"},
		{name: "exact name wins over a longer one", normalized: "real madrid", candidates: []namedTeam{{"rm", "real madrid"}, {"castilla", "real madrid castilla"}}, want: "rm"},
		{name: "ambiguous similar names", normalized: "borussia", candidates: []namedTeam{{"bvb", "borussia dortmund"}, {"bmg", "borussia m gladbach"}}},
		{name: "squad qualifier differs", normalized: names.Team("Arsenal Women"), candidates: []namedTeam{{"arsenal", names.Team("Arsenal")}}},
		{name: "no overlap", normalized: "stade rennais", candidates: []namedTeam{{"rennes", "rennes"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, uniqueNamesake(tt.normalized, tt.candidates))
		})
	}
}

func sofascoreTestID(t *testing.T) int64 {
	t.Helper()
	return 900_000_000 + int64(uuid.New().ID()%100_000_000)
}

func TestCupTeamAdoptsTheEstablishedDomesticNamesake(t *testing.T) {
	f := newEntityFixture(t)
	ctx := t.Context()
	domestic := f.league(t, "DOM", canon.SportFootball)
	cup := f.league(t, "CUP", canon.SportFootball)
	base := "Qwertyhampton q" + f.suffix
	established, err := f.store.UpsertTeam(ctx, Team{SofascoreID: sofascoreTestID(t), LeagueCode: domestic, Name: base})
	require.NoError(t, err)

	adopted, err := f.store.upsertSourceTeam(ctx, SourceTeam{Source: f.source, SourceID: "fm-1", LeagueCode: cup, Name: base + " Wanderers"}, adoptByIDOrAlias, createMissingTeam)
	require.NoError(t, err)
	assert.Equal(t, established, adopted)
	again, err := f.store.upsertSourceTeam(ctx, SourceTeam{Source: f.source, SourceID: "fm-1", LeagueCode: cup, Name: base + " Wanderers"}, adoptByIDOrAlias, createMissingTeam)
	require.NoError(t, err)
	assert.Equal(t, established, again)
	other, err := f.store.upsertSourceTeam(ctx, SourceTeam{Source: f.source, SourceID: "fm-2", LeagueCode: cup, Name: base}, adoptByIDOrAlias, createMissingTeam)
	require.NoError(t, err)
	assert.NotEqual(t, established, other)
}
