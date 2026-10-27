package canon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEveryMarketSpecHasALabel(t *testing.T) {
	t.Parallel()
	for _, spec := range marketSpecs {
		assert.NotEmpty(t, spec.Label, spec.Code)
	}
}

func TestOverUnderTeamAndPlayerSpecs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		code MarketCode
		kind MarketKind
	}{
		{name: "team goals", code: TeamGoals, kind: KindTeam},
		{name: "player passes", code: PropPasses, kind: KindPlayer},
		{name: "goalkeeper saves", code: PropSaves, kind: KindPlayer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec, ok := SpecOf(tt.code)
			assert.True(t, ok)
			assert.Equal(t, tt.kind, spec.Kind)
			assert.Equal(t, ShapeOverUnder, spec.Shape)
			assert.True(t, spec.IsFor(SportFootball))
			assert.True(t, spec.RegulationOnly)
		})
	}
}

func TestStatFieldOfMarket(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		code     MarketCode
		expected string
	}{
		{name: "football prop", code: PropSOT, expected: "shotsOnTarget"},
		{name: "yes no scorer reads goals", code: PropAnytime, expected: "goals"},
		{name: "basketball prop", code: PropPoints, expected: "points"},
		{name: "tennis prop", code: PropDoubleFlt, expected: "doubleFaults"},
		{name: "esports prop", code: PropMapKills, expected: "kills"},
		{name: "combined stat has no single field", code: PropPtsRebAst, expected: ""},
		{name: "match market", code: CodeMoneyline, expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec, ok := SpecOf(tt.code)
			assert.True(t, ok)
			assert.Equal(t, tt.expected, spec.StatField)
		})
	}
}
