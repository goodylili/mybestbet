package flashscore

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	initialSuffix = "."
	slugSeparator = "-"
)

func fullName(shortName, slug string) string {
	surnameWords, initials := splitShortName(shortName)
	if len(surnameWords) == 0 || len(initials) == 0 {
		return shortName
	}
	given, ok := givenNameTokens(strings.Split(slug, slugSeparator), nameTokens(strings.Join(surnameWords, " ")))
	if !ok || !startsWithInitial(given[0], initials[0]) {
		return shortName
	}
	return titleWords(given) + " " + strings.Join(surnameWords, " ")
}

func splitShortName(shortName string) (surnameWords, initials []string) {
	words := strings.Fields(shortName)
	split := len(words)
	for split > 0 && strings.HasSuffix(words[split-1], initialSuffix) {
		split--
	}
	return words[:split], words[split:]
}

func nameTokens(name string) []string {
	return strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return r == ' ' || r == '-' })
}

func givenNameTokens(slugTokens, surnameTokens []string) ([]string, bool) {
	surnameLen := len(surnameTokens)
	if len(slugTokens) <= surnameLen {
		return nil, false
	}
	switch {
	case slices.Equal(slugTokens[:surnameLen], surnameTokens):
		return slugTokens[surnameLen:], true
	case slices.Equal(slugTokens[len(slugTokens)-surnameLen:], surnameTokens):
		return slugTokens[:len(slugTokens)-surnameLen], true
	}
	return nil, false
}

func startsWithInitial(givenToken, initial string) bool {
	first, _ := utf8.DecodeRuneInString(initial)
	slugFirst, _ := utf8.DecodeRuneInString(givenToken)
	return unicode.ToLower(first) == slugFirst
}

func titleWords(tokens []string) string {
	titled := make([]string, len(tokens))
	for i, token := range tokens {
		first, size := utf8.DecodeRuneInString(token)
		titled[i] = string(unicode.ToUpper(first)) + token[size:]
	}
	return strings.Join(titled, " ")
}
