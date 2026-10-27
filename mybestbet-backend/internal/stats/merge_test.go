package stats

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergePrimaryWinsAndSecondaryFillsGaps(t *testing.T) {
	t.Parallel()
	primary := Reading{Source: "sofascore", Values: map[Field]float64{FieldMinutes: 90, FieldShots: 3, FieldRating: 7.4}}
	secondary := Reading{Source: "fotmob", Values: map[Field]float64{FieldMinutes: 90, FieldShots: 2, FieldShotsOnTarget: 1}}

	merged := Merge(primary, secondary)

	assert.Equal(t, map[Field]float64{FieldMinutes: 90, FieldShots: 3, FieldRating: 7.4, FieldShotsOnTarget: 1}, merged.Values)
	assert.Equal(t, map[string]string{"minutes": "sofascore", "shots": "sofascore", "rating": "sofascore", "shotsOnTarget": "fotmob"}, merged.Sources)
	assert.Equal(t, []Disagreement{{Field: FieldShots, Primary: 3, Secondary: 2}}, merged.Disagreements)
}

func TestMergedApplyToLeavesRatingNilWhenAbsent(t *testing.T) {
	t.Parallel()
	var s MatchStat
	Merge(Reading{Source: "fotmob", Values: map[Field]float64{FieldMinutes: 60, FieldStarted: 1, FieldGoals: 1}}, Reading{}).applyTo(&s)
	assert.Equal(t, 60, s.Minutes)
	assert.True(t, s.Started)
	assert.Equal(t, 1, s.Goals)
	assert.Nil(t, s.Rating)
	assert.Equal(t, "fotmob", s.ValueSources["goals"])
}
