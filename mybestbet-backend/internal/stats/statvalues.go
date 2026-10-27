package stats

import "mybestbet/pkg/canon"

var footballCardColumns = map[string]statReading{
	"yellow": count(func(m MatchStat) int { return m.Yellow }),
	"red":    count(func(m MatchStat) int { return m.Red }),
}

func (m MatchStat) StatValues() map[string]float64 {
	values := make(map[string]float64, len(m.Stats)+len(statRules))
	for key, value := range m.Stats {
		values[string(key)] = value
	}
	if m.Sport == canon.SportFootball {
		for field, read := range footballCardColumns {
			values[field], _ = read(m)
		}
	}
	for code, rule := range statRules {
		spec, _ := canon.SpecOf(code)
		if spec.StatField == "" || !spec.IsFor(m.Sport) {
			continue
		}
		if value, ok := rule.read(m); ok {
			values[spec.StatField] = value
		}
	}
	return values
}
