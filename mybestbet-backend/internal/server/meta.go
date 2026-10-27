package server

import (
	"cmp"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang/v2"
)

const (
	telegramBaseURL     = "https://t.me/"
	vcsRevisionKey      = "vcs.revision"
	shortRevisionLength = 12
	unknownVersion      = "unknown"
	countryCodeLength   = 2
)

type countryDB = maxminddb.Reader

var (
	countryHeaders = []string{"CF-IPCountry", "X-Vercel-IP-Country", "CloudFront-Viewer-Country"}
	unknownCountry = map[string]bool{"XX": true, "ZZ": true, "T1": true}
)

var buildVersion = sync.OnceValue(readBuildVersion)

func (s *Server) getMeta(w http.ResponseWriter, r *http.Request) {
	writeCachedJSON(w, r, Meta{
		Version:        buildVersion(),
		TelegramBotURL: telegramBotURL(s.cfg.Telegram.BotUsername),
		Auth:           MetaAuth{Google: s.auth != nil && s.auth.GoogleEnabled()},
		Country:        s.requestCountry(r),
	}, metaMaxAge)
}

func openCountryDB(path string, log *slog.Logger) *countryDB {
	if path == "" {
		return nil
	}
	reader, err := maxminddb.Open(path)
	if err != nil {
		log.Warn("geoip database unavailable", slog.String("path", path), slog.Any("error", err))
		return nil
	}
	return reader
}

func (s *Server) requestCountry(r *http.Request) string {
	trustProxy := s.auth != nil && s.auth.Config().TrustProxy
	if trustProxy {
		for _, header := range countryHeaders {
			if code := countryCode(r.Header.Get(header)); code != "" {
				return code
			}
		}
	}
	return lookupCountry(s.countries, clientIP(r, trustProxy))
}

func lookupCountry(reader *countryDB, ip string) string {
	if reader == nil {
		return ""
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	var iso string
	if err := reader.Lookup(addr.Unmap()).DecodePath(&iso, "country", "iso_code"); err != nil {
		return ""
	}
	return countryCode(iso)
}

func countryCode(raw string) string {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if len(code) != countryCodeLength || unknownCountry[code] {
		return ""
	}
	for _, letter := range code {
		if letter < 'A' || letter > 'Z' {
			return ""
		}
	}
	return code
}

func telegramBotURL(botUsername string) string {
	username := strings.TrimPrefix(strings.TrimSpace(botUsername), "@")
	if username == "" {
		return ""
	}
	return telegramBaseURL + url.PathEscape(username)
}

func readBuildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return unknownVersion
	}
	for _, setting := range info.Settings {
		if setting.Key == vcsRevisionKey {
			return setting.Value[:min(shortRevisionLength, len(setting.Value))]
		}
	}
	return cmp.Or(info.Main.Version, unknownVersion)
}
