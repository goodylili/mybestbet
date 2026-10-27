package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"mybestbet/internal/logos"
)

const (
	maxLogoEntityIDLength    = 128
	immutableLogoCache       = "public, max-age=86400, immutable"
	revalidatedLogoCache     = "public, no-cache"
	svgContentSecurityPolicy = "default-src 'none'; style-src 'unsafe-inline'"
)

func (s *Server) getLogo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	kind, ok := logos.ParseKind(r.PathValue("kind"))
	if !ok {
		s.writeFailure(w, r, invalidInput("kind", "must be team, league or source"))
		return
	}
	id := r.PathValue("id")
	if id == "" || len(id) > maxLogoEntityIDLength {
		s.writeFailure(w, r, invalidInput("id", "must be 1-128 characters"))
		return
	}
	logo, err := s.logos.Get(r.Context(), kind, id)
	if errors.Is(err, logos.ErrNotFound) {
		err = ErrNotFound
	}
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	etag := `"` + logo.SHA256 + `"`
	header := w.Header()
	header.Set("ETag", etag)
	header.Set("Cache-Control", revalidatedLogoCache)
	if r.URL.Query().Get("v") == logo.Version() {
		header.Set("Cache-Control", immutableLogoCache)
	}
	if logo.IsSVG() {
		header.Set("Content-Security-Policy", svgContentSecurityPolicy)
	}
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	header.Set("Content-Type", logo.ContentType)
	header.Set("Content-Length", strconv.Itoa(len(logo.Bytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(logo.Bytes)
}

func etagMatches(ifNoneMatch, etag string) bool {
	etag = strings.TrimPrefix(etag, "W/")
	for candidate := range strings.SplitSeq(ifNoneMatch, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == etag || candidate == "*" {
			return true
		}
	}
	return false
}

func (s *Server) logoURLs(ctx context.Context, kind logos.Kind, ids []string) (map[string]string, error) {
	versions, err := s.logos.Versions(ctx, kind, ids)
	if err != nil {
		return nil, err
	}
	urls := make(map[string]string, len(versions))
	for id, version := range versions {
		urls[id] = logos.URL(kind, id, version)
	}
	return urls, nil
}

func (s *Server) attachTeamLogos(ctx context.Context, teams []*Team) error {
	ids := make([]string, len(teams))
	for i, t := range teams {
		ids[i] = t.ID
	}
	urls, err := s.logoURLs(ctx, logos.KindTeam, ids)
	if err != nil {
		return err
	}
	for _, t := range teams {
		t.LogoURL = urls[t.ID]
	}
	return nil
}

func (s *Server) attachEventLogos(ctx context.Context, events []EventInfo) error {
	teams := make([]*Team, 0, 2*len(events))
	for i := range events {
		teams = append(teams, &events[i].Home, &events[i].Away)
	}
	return s.attachTeamLogos(ctx, teams)
}

func (s *Server) attachLeagueLogos(ctx context.Context, leagues []League) error {
	codes := make([]string, len(leagues))
	for i, l := range leagues {
		codes[i] = l.Code
	}
	urls, err := s.logoURLs(ctx, logos.KindLeague, codes)
	if err != nil {
		return err
	}
	for i := range leagues {
		leagues[i].LogoURL = urls[leagues[i].Code]
	}
	return nil
}
