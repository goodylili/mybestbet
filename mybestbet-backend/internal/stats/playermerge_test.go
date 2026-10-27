package stats

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlanPlayerMerges(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		twins         []playerTwin
		squadClubs    map[string]map[string]string
		wantMerges    []playerMerge
		wantAmbiguous []string
	}{
		{
			name:       "club match stats link the national copy to its club twin",
			twins:      []playerTwin{{duplicateID: "n1", survivorID: "c1", name: "bukayo saka", playedForClub: true}},
			wantMerges: []playerMerge{{duplicateID: "n1", survivorID: "c1", name: "bukayo saka"}},
		},
		{
			name: "fotmob squad club links the national copy to its club twin",
			twins: []playerTwin{{
				duplicateID: "n1", survivorID: "c1", name: "manuel ugarte",
				duplicateFotmobID: "77", nationalFotmobTeam: "5796", clubFotmobTeams: []string{"10260"},
			}},
			squadClubs: map[string]map[string]string{"5796": {"77": "10260"}},
			wantMerges: []playerMerge{{duplicateID: "n1", survivorID: "c1", name: "manuel ugarte"}},
		},
		{
			name: "fotmob squad listing another club is not evidence",
			twins: []playerTwin{{
				duplicateID: "n1", survivorID: "c1", name: "ali ahmed",
				duplicateFotmobID: "77", nationalFotmobTeam: "5796", clubFotmobTeams: []string{"10260"},
			}},
			squadClubs: map[string]map[string]string{"5796": {"77": "9825"}},
		},
		{
			name:  "same name without a club link is left alone",
			twins: []playerTwin{{duplicateID: "n1", survivorID: "c1", name: "joao pedro"}},
		},
		{
			name:  "conflicting source ids are never merged",
			twins: []playerTwin{{duplicateID: "n1", survivorID: "c1", name: "ben davies", playedForClub: true, hasConflictingIdent: true}},
		},
		{
			name: "a national copy linked to two club copies is ambiguous",
			twins: []playerTwin{
				{duplicateID: "n1", survivorID: "c1", name: "gabriel", playedForClub: true},
				{duplicateID: "n1", survivorID: "c2", name: "gabriel", playedForClub: true},
				{duplicateID: "n2", survivorID: "c3", name: "vitinha", playedForClub: true},
			},
			wantMerges:    []playerMerge{{duplicateID: "n2", survivorID: "c3", name: "vitinha"}},
			wantAmbiguous: []string{"gabriel"},
		},
		{
			name: "a club copy claimed by two national copies is ambiguous",
			twins: []playerTwin{
				{duplicateID: "n1", survivorID: "c1", name: "roberto fernandez", playedForClub: true},
				{duplicateID: "n2", survivorID: "c1", name: "roberto fernandez", playedForClub: true},
			},
			wantAmbiguous: []string{"roberto fernandez"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plan := planPlayerMerges(tt.twins, tt.squadClubs)
			assert.Equal(t, tt.wantMerges, plan.merges)
			assert.Equal(t, tt.wantAmbiguous, plan.ambiguous)
		})
	}
}
