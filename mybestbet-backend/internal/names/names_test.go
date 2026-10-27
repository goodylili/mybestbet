package names

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynonymsLoad(t *testing.T) {
	require.NoError(t, LoadError())
}

func TestNormalizeTeam(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "drops club tokens", input: "Arsenal FC", expected: "arsenal"},
		{name: "strips accents", input: "Atlético Madrid", expected: "atletico madrid"},
		{name: "folds letters without decomposition", input: "København", expected: "kobenhavn"},
		{name: "drops numerals and prefixes", input: "1. FC Köln", expected: "koln"},
		{name: "applies whole-name synonym", input: "Man Utd", expected: "manchester united"},
		{name: "applies token synonym", input: "St. Pauli", expected: "saint pauli"},
		{name: "punctuation becomes spaces", input: "Brighton & Hove Albion", expected: "brighton hove albion"},
		{name: "synonym values are normalized", input: "PSG", expected: "paris saint germain"},
		{name: "word-order synonym", input: "Red Bull New York", expected: "new york red bulls"},
		{name: "club-prefix synonym", input: "CS Marítimo", expected: "maritimo"},
		{name: "full-name synonym", input: "Vitória de Guimarães", expected: "vitoria guimaraes"},
		{name: "all-stopword name keeps its words", input: "FC", expected: "fc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, Team(tt.input))
		})
	}
}

func TestNormalizePerson(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		full         string
		surnameKey   string
		reversedKey  string
		isSingleName bool
	}{
		{name: "given then surname", input: "Bruno Fernandes", full: "bruno fernandes", surnameKey: "fernandes b", reversedKey: "bruno f"},
		{name: "comma puts surname last", input: "Fernandes, Bruno", full: "bruno fernandes", surnameKey: "fernandes b", reversedKey: "bruno f"},
		{name: "trailing initial moves first", input: "Fernandes B.", full: "b fernandes", surnameKey: "fernandes b", reversedKey: "b f"},
		{name: "arabic particle merges", input: "Salem Al-Dawsari", full: "salem aldawsari", surnameKey: "aldawsari s", reversedKey: "salem a"},
		{name: "surname first arabic order", input: "Al Dawsari Salem", full: "aldawsari salem", surnameKey: "salem a", reversedKey: "aldawsari s"},
		{name: "suffix synonym", input: "Vinícius Jr.", full: "vinicius junior", surnameKey: "junior v", reversedKey: "vinicius j"},
		{name: "mononym", input: "Pepê", full: "pepe", isSingleName: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Person(tt.input)
			assert.Equal(t, tt.full, p.Full())
			assert.Equal(t, tt.surnameKey, p.SurnameInitialKey())
			assert.Equal(t, tt.reversedKey, p.ReversedSurnameInitialKey())
			assert.Equal(t, tt.isSingleName, p.IsMononym())
		})
	}
}

func TestTeamsLikelySame(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		expected bool
	}{
		{"short form prefix", "Tottenham", "Tottenham Hotspur", true},
		{"identical after normalization", "Everton FC", "Everton", true},
		{"different clubs", "Manchester City", "Manchester United", false},
		{"partial token is not a prefix", "Man", "Manchester City", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, TeamsLikelySame(tt.a, tt.b))
		})
	}
}

func TestMultiSportNormalization(t *testing.T) {
	assert.Equal(t, Team("NAVI"), Team("Natus Vincere"))
	assert.Equal(t, Team("Team Spirit"), Team("Spirit"))
	assert.Equal(t, Team("G2 Esports"), Team("G2"))
	assert.Equal(t, "michigan state", Team("Michigan St."))
	assert.Equal(t, "saint john s", Team("St. John's"))
	assert.Equal(t, "arsenal women", Team("Arsenal (W)"))
	assert.Equal(t, "patrick mahomes", Person("Patrick Mahomes II").Full())
	assert.Equal(t, "vinicius junior", Person("Vinicius Jr.").Full())
	assert.Equal(t, "marvin harrison", Person("Marvin Harrison Jr.").Full())
}

func TestSquadQualifiers(t *testing.T) {
	assert.True(t, HaveSameSquadQualifiers(Team("Arsenal W"), Team("Arsenal Women")))
	assert.False(t, HaveSameSquadQualifiers(Team("Arsenal"), Team("Arsenal Women")))
	assert.False(t, HaveSameSquadQualifiers(Team("Barcelona"), Team("Barcelona B")))
}

func TestHandle(t *testing.T) {
	tests := []struct{ input, expected string }{
		{"ZywOo", "zywoo"},
		{"Mathieu 'ZywOo' Herbaut", "zywoo"},
		{"Danil “donk” Kryshkovets", "donk"},
		{"m0NESY", "m0nesy"},
		{"Na'Vi", "navi"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, Handle(tt.input), tt.input)
	}
}
