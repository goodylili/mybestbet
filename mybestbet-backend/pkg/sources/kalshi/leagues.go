package kalshi

import (
	"maps"

	"mybestbet/pkg/canon"
)

type league struct {
	prefix      string
	family      family
	competition string
}

func (l league) gameSeries() string { return l.prefix + l.family.gameSuffix }

func soccerLeague(prefix string) league { return league{prefix: prefix, family: soccerFamily} }

func tennisLeague(prefix, competition string) league {
	return league{prefix: prefix, family: tennisFamily, competition: competition}
}

const (
	atpPrefix           = "KXATP"
	wtaPrefix           = "KXWTA"
	atpChallengerPrefix = "KXATPCHALLENGER"
)

var defaultLeagues = map[string]league{
	"WC":           soccerLeague("KXWC"),
	"FRIENDLY":     soccerLeague("KXINTLFRIENDLY"),
	"UCL":          soccerLeague("KXUCL"),
	"UEL":          soccerLeague("KXUEL"),
	"UECL":         soccerLeague("KXUECL"),
	"UWCL":         soccerLeague("KXUCLW"),
	"EPL":          soccerLeague("KXEPL"),
	"CHAMP":        soccerLeague("KXEFLCHAMPIONSHIP"),
	"FACUP":        soccerLeague("KXFACUP"),
	"EFLCUP":       soccerLeague("KXEFLCUP"),
	"LALIGA":       soccerLeague("KXLALIGA"),
	"COPADELREY":   soccerLeague("KXCOPADELREY"),
	"SERIEA":       soccerLeague("KXSERIEA"),
	"COPPAITALIA":  soccerLeague("KXCOPPAITALIA"),
	"BUNDES":       soccerLeague("KXBUNDESLIGA"),
	"DFBPOKAL":     soccerLeague("KXDFBPOKAL"),
	"LIGUE1":       soccerLeague("KXLIGUE1"),
	"CDF":          soccerLeague("KXCOUPEDEFRANCE"),
	"ERED":         soccerLeague("KXEREDIVISIE"),
	"PRIMEIRA":     soccerLeague("KXLIGAPORTUGAL"),
	"SUPERLIG":     soccerLeague("KXSUPERLIG"),
	"SPFL":         soccerLeague("KXSCOTTISHPREM"),
	"JPL":          soccerLeague("KXBELGIANPL"),
	"MLS":          soccerLeague("KXMLS"),
	"LIGAMX":       soccerLeague("KXLIGAMX"),
	"BRA1":         soccerLeague("KXBRASILEIRO"),
	"BRA2":         soccerLeague("KXBRASILEIROB"),
	"ARG1":         soccerLeague("KXARGPREMDIV"),
	"SPL":          soccerLeague("KXSAUDIPL"),
	"SSL":          soccerLeague("KXSWISSLEAGUE"),
	"GSL":          soccerLeague("KXSLGREECE"),
	"ELITESERIEN":  soccerLeague("KXELITESERIEN"),
	"DSL":          soccerLeague("KXDENSUPERLIGA"),
	"J1":           soccerLeague("KXJLEAGUE"),
	"J2":           soccerLeague("KXJ2LEAGUE"),
	"KLEAGUE":      soccerLeague("KXKLEAGUE"),
	"CSL":          soccerLeague("KXCHNSL"),
	"ISL":          soccerLeague("KXINDSL"),
	"ALEAGUE":      soccerLeague("KXALEAGUE"),
	"LEAGUESCUP":   soccerLeague("KXLEAGUESCUP"),
	"LIBERTADORES": soccerLeague("KXCONMEBOLLIB"),
	"SUDAMERICANA": soccerLeague("KXCONMEBOLSUD"),
	"SUPERCOPA":    soccerLeague("KXESPSUPERCUP"),

	"NBA":        {prefix: "KXNBA", family: nbaFamily},
	"WNBA":       {prefix: "KXWNBA", family: nbaFamily},
	"NCAAB":      {prefix: "KXNCAAMB", family: nbaFamily},
	"EUROLEAGUE": {prefix: "KXEUROLEAGUE", family: basketballCoreFamily},
	"EUROCUP":    {prefix: "KXEUROCUP", family: basketballCoreFamily},
	"ACB":        {prefix: "KXACB", family: basketballCoreFamily},
	"NBL":        {prefix: "KXNBL", family: basketballCoreFamily},

	"NFL":   {prefix: "KXNFL", family: gridironFamily},
	"NCAAF": {prefix: "KXNCAAF", family: gridironFamily},

	"ATP_MIAMI":       tennisLeague(atpPrefix, "ATP Miami"),
	"ATP_RG":          tennisLeague(atpPrefix, "French Open Men Singles"),
	"ATP_WIM":         tennisLeague(atpPrefix, "Wimbledon Men Singles"),
	"ATP_AO":          tennisLeague(atpPrefix, "Australian Open Men Singles"),
	"ATP_USO":         tennisLeague(atpPrefix, "US Open Men Singles"),
	"ATP_CH_ZADAR":    tennisLeague(atpChallengerPrefix, "ATP Challenger Zadar"),
	"ATP_CH_MURCIA":   tennisLeague(atpChallengerPrefix, "ATP Challenger Murcia"),
	"ATP_CH_MORELOS":  tennisLeague(atpChallengerPrefix, "ATP Challenger Morelos"),
	"ATP_CH_ASUNCION": tennisLeague(atpChallengerPrefix, "ATP Challenger Asuncion"),
	"WTA_MIAMI":       tennisLeague(wtaPrefix, "WTA Miami"),
	"WTA_RG":          tennisLeague(wtaPrefix, "French Open Women Singles"),
	"WTA_WIM":         tennisLeague(wtaPrefix, "Wimbledon Women Singles"),
	"WTA_AO":          tennisLeague(wtaPrefix, "Australian Open Women Singles"),
	"WTA_USO":         tennisLeague(wtaPrefix, "US Open Women Singles"),

	"CS2":      {prefix: "KXCS2", family: esportsFamily},
	"DOTA2":    {prefix: "KXDOTA2", family: esportsFamily},
	"VALORANT": {prefix: "KXVALORANT", family: esportsFamily},
	"LOL":      {prefix: "KXLOL", family: esportsFamily},
}

func leaguesWithOverrides(overrides map[string]string) map[string]league {
	leagues := maps.Clone(defaultLeagues)
	for code, value := range overrides {
		l, known := leagues[code]
		if !known {
			continue
		}
		if l.family.sport == canon.SportTennis {
			l.competition = value
		} else {
			l.prefix = value
		}
		leagues[code] = l
	}
	return leagues
}

func leaguesByGameSeries(leagues map[string]league) map[string]league {
	byGameSeries := make(map[string]league, len(leagues))
	for _, l := range leagues {
		byGameSeries[l.gameSeries()] = l
	}
	return byGameSeries
}

func marketsOffered(leagues map[string]league) []canon.MarketCode {
	offered := map[canon.MarketCode]bool{}
	for _, l := range leagues {
		for _, s := range l.family.series {
			offered[s.code] = true
		}
	}
	return canon.InCanonicalOrder(offered)
}
