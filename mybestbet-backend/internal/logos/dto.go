package logos

import "net/url"

type Kind string

const (
	KindTeam   Kind = "team"
	KindLeague Kind = "league"
	KindSource Kind = "source"
)

const versionLength = 12

func ParseKind(raw string) (Kind, bool) {
	switch kind := Kind(raw); kind {
	case KindTeam, KindLeague, KindSource:
		return kind, true
	}
	return "", false
}

type Logo struct {
	Kind        Kind
	EntityID    string
	ContentType string
	SHA256      string
	Bytes       []byte
}

func (l Logo) Version() string { return shortVersion(l.SHA256) }

func (l Logo) IsSVG() bool { return l.ContentType == contentTypeSVG }

func shortVersion(sha string) string {
	if len(sha) > versionLength {
		return sha[:versionLength]
	}
	return sha
}

func URL(kind Kind, entityID, version string) string {
	if version == "" {
		return ""
	}
	return "/v1/logos/" + string(kind) + "/" + url.PathEscape(entityID) + "?v=" + url.QueryEscape(version)
}
