package sources_test

import (
	"testing"

	"mybestbet/pkg/sources"
)

func TestSlug(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		sep  string
		want string
	}{
		{name: "spaces", in: "Manchester United", sep: "-", want: "manchester-united"},
		{name: "accents and punctuation", in: "Atlético Madrid & Co.", sep: "_", want: "atletico_madrid_co"},
		{name: "digits kept", in: "FC Köln 1948", sep: "-", want: "fc-koln-1948"},
		{name: "non latin dropped", in: "Спартак", sep: "-", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sources.Slug(tt.in, tt.sep); got != tt.want {
				t.Errorf("Slug(%q, %q) = %q, want %q", tt.in, tt.sep, got, tt.want)
			}
		})
	}
}

func TestEventRefLeagueTitle(t *testing.T) {
	t.Parallel()
	if got := (sources.EventRef{League: "EPL", Country: "England", LeagueName: "Premier League"}).LeagueTitle(); got != "England Premier League" {
		t.Errorf("LeagueTitle() = %q", got)
	}
	if got := (sources.EventRef{League: "EPL"}).LeagueTitle(); got != "EPL" {
		t.Errorf("LeagueTitle() without names = %q", got)
	}
}

func TestOrigin(t *testing.T) {
	t.Parallel()
	if got := sources.Origin("https://www.betpawa.ng/api/sportsbook/v4"); got != "https://www.betpawa.ng" {
		t.Errorf("Origin() = %q", got)
	}
	if got := sources.Origin("not a url"); got != "" {
		t.Errorf("Origin(invalid) = %q", got)
	}
}
