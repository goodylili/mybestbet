package stats

type Field string

const (
	FieldMinutes       Field = "minutes"
	FieldStarted       Field = "started"
	FieldGoals         Field = "goals"
	FieldAssists       Field = "assists"
	FieldShots         Field = "shots"
	FieldShotsOnTarget Field = "shotsOnTarget"
	FieldFouls         Field = "fouls"
	FieldTackles       Field = "tackles"
	FieldYellow        Field = "yellow"
	FieldRed           Field = "red"
	FieldRating        Field = "rating"
)

type Reading struct {
	Source string
	Values map[Field]float64
}

type Disagreement struct {
	Field     Field
	Primary   float64
	Secondary float64
}

type Merged struct {
	Values        map[Field]float64
	Sources       map[string]string
	Disagreements []Disagreement
}

func Merge(primary, secondary Reading) Merged {
	m := Merged{Values: map[Field]float64{}, Sources: map[string]string{}}
	for field, value := range primary.Values {
		m.Values[field] = value
		m.Sources[string(field)] = primary.Source
	}
	for field, value := range secondary.Values {
		primaryValue, seen := primary.Values[field]
		if !seen {
			m.Values[field] = value
			m.Sources[string(field)] = secondary.Source
			continue
		}
		if primaryValue != value {
			m.Disagreements = append(m.Disagreements, Disagreement{Field: field, Primary: primaryValue, Secondary: value})
		}
	}
	return m
}

func (m Merged) applyTo(s *MatchStat) {
	v := m.Values
	s.Minutes, s.Goals, s.Assists = int(v[FieldMinutes]), int(v[FieldGoals]), int(v[FieldAssists])
	s.Shots, s.ShotsOnTarget = int(v[FieldShots]), int(v[FieldShotsOnTarget])
	s.Fouls, s.Tackles, s.Yellow, s.Red = int(v[FieldFouls]), int(v[FieldTackles]), int(v[FieldYellow]), int(v[FieldRed])
	s.Started = v[FieldStarted] == 1
	if rating, ok := v[FieldRating]; ok {
		s.Rating = &rating
	}
	s.ValueSources = m.Sources
}
