package resolve

import (
	"fmt"
	"os"
	"slices"
	"time"

	"gopkg.in/yaml.v3"

	"mybestbet/pkg/canon"
)

type MatchingConfig struct {
	KickoffTolerance      time.Duration                 `yaml:"kickoffTolerance"`
	SportKickoffTolerance map[canon.Sport]time.Duration `yaml:"sportKickoffTolerance"`
	CollegeLeagues        []string                      `yaml:"collegeLeagues"`
}

var DefaultMatchingConfig = MatchingConfig{
	KickoffTolerance:      15 * time.Minute,
	SportKickoffTolerance: map[canon.Sport]time.Duration{canon.SportTennis: 12 * time.Hour},
	CollegeLeagues:        []string{"NCAAF", "NCAAB"},
}

func (m MatchingConfig) IsCollegeLeague(league string) bool {
	return slices.Contains(m.CollegeLeagues, league)
}

func (m MatchingConfig) KickoffToleranceFor(sport canon.Sport) time.Duration {
	if tolerance, ok := m.SportKickoffTolerance[sport]; ok && tolerance > 0 {
		return tolerance
	}
	if m.KickoffTolerance > 0 {
		return m.KickoffTolerance
	}
	return DefaultMatchingConfig.KickoffTolerance
}

func LoadMatchingConfig(configPath string) (MatchingConfig, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return MatchingConfig{}, fmt.Errorf("read %s: %w", configPath, err)
	}
	var file struct {
		Resolve *MatchingConfig `yaml:"resolve"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return MatchingConfig{}, fmt.Errorf("parse resolve block of %s: %w", configPath, err)
	}
	if file.Resolve == nil {
		return DefaultMatchingConfig, nil
	}
	return *file.Resolve, nil
}
