package names

import (
	_ "embed"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
)

//go:embed synonyms.yaml
var synonymsYAML []byte

type synonymFile struct {
	TeamNames      map[string]string `yaml:"teamNames"`
	TeamTokens     map[string]string `yaml:"teamTokens"`
	LastTeamTokens map[string]string `yaml:"lastTeamTokens"`
	DropTeamTokens []string          `yaml:"dropTeamTokens"`
	SquadQualifier []string          `yaml:"squadQualifiers"`
	PlayerTokens   map[string]string `yaml:"playerTokens"`
	PersonSuffixes []string          `yaml:"personSuffixes"`
	MergeParticles []string          `yaml:"mergeParticles"`
}

type normalizer struct {
	teamNames      map[string]string
	teamTokens     map[string]string
	lastTeamTokens map[string]string
	dropTeamTokens map[string]bool
	qualifiers     map[string]bool
	playerTokens   map[string]string
	personSuffixes map[string]bool
	mergeParticles map[string]bool
}

var standard, standardLoadErr = load()

func LoadError() error { return standardLoadErr }

func Team(name string) string { return standard.team(name) }

func TeamLiteral(name string) string { return standard.teamTokenString(name) }

func HaveSameSquadQualifiers(normalizedA, normalizedB string) bool {
	return slices.Equal(standard.squadQualifiers(normalizedA), standard.squadQualifiers(normalizedB))
}

func (n *normalizer) squadQualifiers(normalized string) []string {
	var found []string
	for _, token := range strings.Fields(normalized) {
		if n.qualifiers[token] && !slices.Contains(found, token) {
			found = append(found, token)
		}
	}
	slices.Sort(found)
	return found
}

func Person(raw string) PersonName { return standard.person(raw) }

func TeamsLikelySame(a, b string) bool {
	na, nb := Team(a), Team(b)
	if na == "" || nb == "" {
		return false
	}
	return na == nb || strings.HasPrefix(na, nb+" ") || strings.HasPrefix(nb, na+" ")
}

var lettersWithoutDecomposition = map[rune]string{
	'ø': "o", 'æ': "ae", 'ß': "ss", 'ł': "l", 'đ': "d", 'ı': "i", 'œ': "oe", 'þ': "th", 'ð': "d",
}

func load() (*normalizer, error) {
	var file synonymFile
	if err := yaml.Unmarshal(synonymsYAML, &file); err != nil {
		return &normalizer{}, fmt.Errorf("parse synonyms.yaml: %w", err)
	}
	n := &normalizer{
		teamTokens:     file.TeamTokens,
		lastTeamTokens: file.LastTeamTokens,
		dropTeamTokens: setOf(file.DropTeamTokens),
		qualifiers:     setOf(file.SquadQualifier),
		playerTokens:   file.PlayerTokens,
		personSuffixes: setOf(file.PersonSuffixes),
		mergeParticles: setOf(file.MergeParticles),
		teamNames:      make(map[string]string, len(file.TeamNames)),
	}
	for variant, canonical := range file.TeamNames {
		n.teamNames[n.teamTokenString(variant)] = n.teamTokenString(canonical)
	}
	return n, nil
}

func setOf(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}

func (n *normalizer) team(name string) string {
	tokenString := n.teamTokenString(name)
	if canonical, ok := n.teamNames[tokenString]; ok {
		return canonical
	}
	return tokenString
}

func (n *normalizer) teamTokenString(name string) string {
	words := splitWords(foldAccents(name))
	kept := make([]string, 0, len(words))
	for i, w := range words {
		w = n.replaceTeamToken(w, i > 0 && i == len(words)-1)
		if n.dropTeamTokens[w] || isAllDigits(w) {
			continue
		}
		kept = append(kept, w)
	}
	if len(kept) == 0 {
		return strings.Join(words, " ")
	}
	return strings.Join(kept, " ")
}

func (n *normalizer) replaceTeamToken(word string, isTrailing bool) string {
	if replacement, ok := n.lastTeamTokens[word]; ok && isTrailing {
		return replacement
	}
	if replacement, ok := n.teamTokens[word]; ok {
		return replacement
	}
	return word
}

type PersonName struct {
	tokens []string
}

func (n *normalizer) person(raw string) PersonName {
	folded := foldAccents(raw)
	if surnameFirst, given, hasComma := strings.Cut(folded, ","); hasComma {
		folded = given + " " + surnameFirst
	}
	words := splitWords(folded)
	for i, w := range words {
		if replacement, ok := n.playerTokens[w]; ok {
			words[i] = replacement
		}
	}
	return PersonName{tokens: moveTrailingInitialsFront(n.mergeNameParticles(n.dropGenerationalSuffix(words)))}
}

const minTokensBeforeSuffixDrop = 3

func (n *normalizer) dropGenerationalSuffix(words []string) []string {
	if len(words) < minTokensBeforeSuffixDrop || !n.personSuffixes[words[len(words)-1]] {
		return words
	}
	return words[:len(words)-1]
}

var quotedHandle = regexp.MustCompile(`['"\x{2018}\x{2019}\x{201C}\x{201D}]([^'"\x{2018}\x{2019}\x{201C}\x{201D}]+)['"\x{2018}\x{2019}\x{201C}\x{201D}]`)

func Handle(raw string) string {
	if quoted := quotedHandle.FindStringSubmatch(raw); quoted != nil {
		raw = quoted[1]
	}
	return strings.Join(splitWords(foldAccents(raw)), "")
}

func (n *normalizer) mergeNameParticles(words []string) []string {
	merged := make([]string, 0, len(words))
	for i := 0; i < len(words); i++ {
		if n.mergeParticles[words[i]] && i+1 < len(words) {
			merged = append(merged, words[i]+words[i+1])
			i++
			continue
		}
		merged = append(merged, words[i])
	}
	return merged
}

func moveTrailingInitialsFront(words []string) []string {
	firstTrailingInitial := len(words)
	for firstTrailingInitial > 0 && IsInitial(words[firstTrailingInitial-1]) {
		firstTrailingInitial--
	}
	if firstTrailingInitial == 0 || firstTrailingInitial == len(words) {
		return words
	}
	return append(slices.Clone(words[firstTrailingInitial:]), words[:firstTrailingInitial]...)
}

func IsInitial(word string) bool { return len(word) == 1 }

func (p PersonName) Tokens() []string { return p.tokens }

func (p PersonName) Full() string { return strings.Join(p.tokens, " ") }

func (p PersonName) SortedFull() string {
	sorted := slices.Clone(p.tokens)
	slices.Sort(sorted)
	return strings.Join(sorted, " ")
}

func (p PersonName) IsMononym() bool { return len(p.tokens) == 1 }

func (p PersonName) Given() string {
	if len(p.tokens) == 0 {
		return ""
	}
	return p.tokens[0]
}

func (p PersonName) Surname() string {
	if len(p.tokens) == 0 {
		return ""
	}
	return p.tokens[len(p.tokens)-1]
}

func (p PersonName) SurnameInitialKey() string {
	if len(p.tokens) < 2 {
		return ""
	}
	return p.Surname() + " " + p.Given()[:1]
}

func (p PersonName) ReversedSurnameInitialKey() string {
	if len(p.tokens) < 2 {
		return ""
	}
	return p.Given() + " " + p.Surname()[:1]
}

func foldAccents(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if replacement, ok := lettersWithoutDecomposition[r]; ok {
			b.WriteString(replacement)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func splitWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func isAllDigits(word string) bool {
	for _, r := range word {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return word != ""
}
