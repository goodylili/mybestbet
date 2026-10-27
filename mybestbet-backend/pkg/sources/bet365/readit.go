package bet365

import (
	"bytes"
	"strings"
)

const (
	messageSeparator     = '\b'
	topicPayloadMarker   = '\x01'
	sectionSeparator     = "|"
	fieldSeparator       = ";"
	snapshotFrameKind    = "F"
	pushTopicFramePrefix = "\x14\x15"
)

func parseSnapshotSections(payload []byte) []section {
	var sections []section
	for message := range bytes.SplitSeq(payload, []byte{messageSeparator}) {
		frame := stripPushTopic(string(message))
		parts := strings.Split(frame, sectionSeparator)
		if len(parts) == 0 || strings.TrimSpace(parts[0]) != snapshotFrameKind {
			continue
		}
		for _, raw := range parts[1:] {
			if s, ok := parseSection(raw); ok {
				sections = append(sections, s)
			}
		}
	}
	return sections
}

func stripPushTopic(message string) string {
	if message == "" || !strings.ContainsRune(pushTopicFramePrefix, rune(message[0])) {
		return message
	}
	if i := strings.IndexByte(message, topicPayloadMarker); i >= 0 {
		return message[i+1:]
	}
	return ""
}

func parseSection(raw string) (section, bool) {
	kind, rest, hasFields := strings.Cut(raw, fieldSeparator)
	if !hasFields || kind == "" {
		return section{}, false
	}
	s := section{kind: kind, fields: map[string]string{}}
	for field := range strings.SplitSeq(rest, fieldSeparator) {
		if key, value, ok := strings.Cut(field, "="); ok {
			s.fields[key] = value
		}
	}
	return s, true
}

func buildFixtures(sections []section, defaultFixtureID string) []*fixture {
	b := fixtureBuilder{defaultFixtureID: defaultFixtureID}
	for _, s := range sections {
		b.add(s)
	}
	return b.fixtures
}

type fixtureBuilder struct {
	defaultFixtureID string
	fixtures         []*fixture
	current          *fixture
	group            *marketGroup
	column           *column
}

func (b *fixtureBuilder) add(s section) {
	switch s.kind {
	case "EV":
		b.startFixture(s)
	case "MG":
		b.startGroup(s)
	case "MA":
		b.startColumn(s)
	case "PA":
		b.addParticipant(s)
	}
}

func (b *fixtureBuilder) startFixture(s section) {
	id := s.fields["FI"]
	if id == "" {
		id = b.defaultFixtureID
	}
	b.current = &fixture{id: id, name: s.fields["NA"], competition: s.fields["CT"], startsAt: s.fields["BC"]}
	b.fixtures = append(b.fixtures, b.current)
	b.group, b.column = nil, nil
}

func (b *fixtureBuilder) startGroup(s section) {
	if b.current == nil {
		b.startFixture(section{fields: map[string]string{}})
	}
	b.group = &marketGroup{id: s.fields["ID"], name: s.fields["NA"]}
	b.current.groups = append(b.current.groups, b.group)
	b.column = nil
}

func (b *fixtureBuilder) startColumn(s section) {
	if b.group == nil {
		return
	}
	b.column = &column{name: s.fields["NA"]}
	b.group.columns = append(b.group.columns, b.column)
}

func (b *fixtureBuilder) addParticipant(s section) {
	if b.group == nil {
		return
	}
	if b.column == nil {
		b.column = &column{}
		b.group.columns = append(b.group.columns, b.column)
	}
	b.column.participants = append(b.column.participants, participant{
		id:          s.fields["ID"],
		name:        s.fields["NA"],
		odds:        s.fields["OD"],
		handicap:    s.fields["HA"],
		isSuspended: s.fields["SU"] == "1",
	})
}
