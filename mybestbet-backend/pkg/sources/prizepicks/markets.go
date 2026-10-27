package prizepicks

import "mybestbet/pkg/canon"

type statMarket struct {
	code   canon.MarketCode
	period string
}

var statMarketsBySport = map[canon.Sport]map[string]statMarket{
	canon.SportFootball: {
		"goals":            {code: canon.PropGoals},
		"shots":            {code: canon.PropShots},
		"shots on target":  {code: canon.PropSOT},
		"assists":          {code: canon.PropAssists},
		"fouls":            {code: canon.PropFouls},
		"tackles":          {code: canon.PropTackles},
		"passes attempted": {code: canon.PropPasses},
		"goalie saves":     {code: canon.PropSaves},
	},
	canon.SportBasketball: {
		"points":        {code: canon.PropPoints},
		"rebounds":      {code: canon.PropRebounds},
		"assists":       {code: canon.PropBkAssists},
		"3-pt made":     {code: canon.PropThrees},
		"pts+rebs+asts": {code: canon.PropPtsRebAst},
	},
	canon.SportAmericanFootball: {
		"pass yards":      {code: canon.PropPassYards},
		"rush yards":      {code: canon.PropRushYards},
		"receiving yards": {code: canon.PropRecYards},
		"receptions":      {code: canon.PropReceptions},
		"pass tds":        {code: canon.PropPassTDs},
	},
	canon.SportTennis: {
		"aces":          {code: canon.PropAces},
		"double faults": {code: canon.PropDoubleFlt},
	},
	canon.SportEsports: {
		"map 1 kills":    {code: canon.PropMapKills, period: canon.PeriodFirstMap},
		"maps 1-2 kills": {code: canon.PropMapKills, period: canon.PeriodMaps1To2},
	},
}

var suppliedMarkets = []canon.MarketCode{
	canon.PropGoals, canon.PropShots, canon.PropSOT, canon.PropAssists, canon.PropFouls, canon.PropTackles,
	canon.PropPasses, canon.PropSaves,
	canon.PropPoints, canon.PropRebounds, canon.PropBkAssists, canon.PropThrees, canon.PropPtsRebAst,
	canon.PropPassYards, canon.PropRushYards, canon.PropRecYards, canon.PropReceptions, canon.PropPassTDs,
	canon.PropAces, canon.PropDoubleFlt, canon.PropMapKills,
}

var defaultLeaguesByCompetition = map[string]string{
	"premier league":           "EPL",
	"epl":                      "EPL",
	"la liga":                  "LALIGA",
	"laliga":                   "LALIGA",
	"serie a":                  "SERIEA",
	"bundesliga":               "BUNDES",
	"ligue 1":                  "LIGUE1",
	"eredivisie":               "ERED",
	"primeira liga":            "PRIMEIRA",
	"liga portugal":            "PRIMEIRA",
	"championship":             "CHAMP",
	"efl championship":         "CHAMP",
	"saudi pro league":         "SPL",
	"mls":                      "MLS",
	"major league soccer":      "MLS",
	"champions league":         "UCL",
	"uefa champions league":    "UCL",
	"europa league":            "UEL",
	"uefa europa league":       "UEL",
	"conference league":        "UECL",
	"uefa conference league":   "UECL",
	"liga mx":                  "LIGAMX",
	"brasileirao":              "BRA1",
	"brazil serie a":           "BRA1",
	"liga profesional":         "ARG1",
	"super lig":                "SUPERLIG",
	"scottish premiership":     "SPFL",
	"fa cup":                   "FACUP",
	"copa del rey":             "COPADELREY",
	"coppa italia":             "COPPAITALIA",
	"dfb pokal":                "DFBPOKAL",
	"world cup":                "WC",
	"fifa world cup":           "WC",
	"international friendlies": "FRIENDLY",
	"leagues cup":              "LEAGUESCUP",
	"copa libertadores":        "LIBERTADORES",
	"a-league":                 "ALEAGUE",
	"j1 league":                "J1",
	"k league 1":               "KLEAGUE",
	"atp miami":                "ATP_MIAMI",
	"atp french open":          "ATP_RG",
	"atp wimbledon":            "ATP_WIM",
	"atp australian open":      "ATP_AO",
	"atp us open":              "ATP_USO",
	"wta miami":                "WTA_MIAMI",
	"wta french open":          "WTA_RG",
	"wta wimbledon":            "WTA_WIM",
	"wta australian open":      "WTA_AO",
	"wta us open":              "WTA_USO",
	"atp challenger zadar":     "ATP_CH_ZADAR",
	"atp challenger murcia":    "ATP_CH_MURCIA",
	"atp challenger morelos":   "ATP_CH_MORELOS",
	"atp challenger asuncion":  "ATP_CH_ASUNCION",
}

var sportByCompetitionLeague = func() map[string]canon.Sport {
	m := map[string]canon.Sport{}
	for _, code := range []string{
		"WC", "FRIENDLY", "UCL", "UEL", "UECL", "UWCL", "EPL", "CHAMP", "FACUP", "EFLCUP", "LALIGA", "COPADELREY",
		"SERIEA", "COPPAITALIA", "BUNDES", "DFBPOKAL", "LIGUE1", "CDF", "ERED", "PRIMEIRA", "SUPERLIG", "SPFL", "JPL",
		"MLS", "LIGAMX", "BRA1", "BRA2", "ARG1", "SPL", "SSL", "GSL", "UPL", "RPL", "ELITESERIEN", "DSL", "J1", "J2",
		"KLEAGUE", "CSL", "ISL", "ALEAGUE", "LEAGUESCUP", "LIBERTADORES", "SUDAMERICANA", "SUPERCOPA",
	} {
		m[code] = canon.SportFootball
	}
	for _, code := range []string{
		"ATP_MIAMI", "ATP_RG", "ATP_WIM", "ATP_AO", "ATP_USO", "ATP_CH_ZADAR", "ATP_CH_MURCIA", "ATP_CH_MORELOS",
		"ATP_CH_ASUNCION", "WTA_MIAMI", "WTA_RG", "WTA_WIM", "WTA_AO", "WTA_USO",
	} {
		m[code] = canon.SportTennis
	}
	return m
}()

const (
	oddsTypeStandard = "standard"
	oddsTypeGoblin   = "goblin"
	oddsTypeDemon    = "demon"
)

var knownOddsTypes = map[string]bool{oddsTypeStandard: true, oddsTypeGoblin: true, oddsTypeDemon: true}
