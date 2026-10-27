package canon

import "slices"

type Sport string

const (
	SportFootball         Sport = "football"
	SportBasketball       Sport = "basketball"
	SportTennis           Sport = "tennis"
	SportAmericanFootball Sport = "american-football"
	SportEsports          Sport = "esports"
)

var Sports = []Sport{SportFootball, SportBasketball, SportTennis, SportAmericanFootball, SportEsports}

type MarketCode string

type MarketKind string

const (
	KindMatch  MarketKind = "match"
	KindPlayer MarketKind = "player"
	KindTeam   MarketKind = "team"
)

type MarketShape string

const (
	ShapeThreeWay  MarketShape = "three_way"
	ShapeTwoWay    MarketShape = "two_way"
	ShapeHandicap  MarketShape = "handicap"
	ShapeOverUnder MarketShape = "over_under"
	ShapeYesNo     MarketShape = "yes_no"
	ShapeLabeled   MarketShape = "labeled"
	ShapeBuckets   MarketShape = "buckets"
)

type MarketSpec struct {
	Code           MarketCode
	Kind           MarketKind
	Shape          MarketShape
	Sports         []Sport
	RegulationOnly bool
	Label          string
	StatField      string
}

const (
	Code1X2     MarketCode = "1X2"
	CodeDC      MarketCode = "DC"
	CodeOU      MarketCode = "OU"
	CodeBTTS    MarketCode = "BTTS"
	CodeDNB     MarketCode = "DNB"
	CodeAH      MarketCode = "AH"
	CodeCS      MarketCode = "CS"
	CodeHTFT    MarketCode = "HTFT"
	Code1H1X2   MarketCode = "1H_1X2"
	CodeCornOU  MarketCode = "CORN_OU"
	CodeQualify MarketCode = "QUALIFY"

	CodeMoneyline  MarketCode = "ML"
	CodeSpread     MarketCode = "SPREAD"
	CodeTotal      MarketCode = "TOTAL"
	CodeSetHcp     MarketCode = "SET_HCP"
	CodeTotalGames MarketCode = "TOTAL_GAMES"
	CodeMapHcp     MarketCode = "MAP_HCP"
	CodeTotalMaps  MarketCode = "TOTAL_MAPS"

	PropAnytime MarketCode = "P_ANYTIME"
	PropFirst   MarketCode = "P_FIRST"
	PropLast    MarketCode = "P_LAST"
	PropGoals   MarketCode = "P_GOALS"
	PropShots   MarketCode = "P_SHOTS"
	PropSOT     MarketCode = "P_SOT"
	PropAssists MarketCode = "P_AST"
	PropGA      MarketCode = "P_GA"
	PropCard    MarketCode = "P_CARD"
	PropFouls   MarketCode = "P_FOULS"
	PropTackles MarketCode = "P_TACKLES"
	PropPasses  MarketCode = "P_PASSES"
	PropSaves   MarketCode = "P_SAVES"
	PropRating  MarketCode = "P_RATING"

	PropPoints     MarketCode = "BK_PTS"
	PropRebounds   MarketCode = "BK_REB"
	PropBkAssists  MarketCode = "BK_AST"
	PropThrees     MarketCode = "BK_3PM"
	PropPtsRebAst  MarketCode = "BK_PRA"
	PropPassYards  MarketCode = "NF_PASS_YDS"
	PropRushYards  MarketCode = "NF_RUSH_YDS"
	PropRecYards   MarketCode = "NF_REC_YDS"
	PropReceptions MarketCode = "NF_REC"
	PropPassTDs    MarketCode = "NF_PASS_TD"
	PropAnytimeTD  MarketCode = "NF_ANYTIME_TD"
	PropAces       MarketCode = "TN_ACES"
	PropDoubleFlt  MarketCode = "TN_DF"
	PropMapKills   MarketCode = "ES_KILLS"

	TeamGoals  MarketCode = "T_GOALS"
	TeamShots  MarketCode = "T_SHOTS"
	TeamPasses MarketCode = "T_PASSES"
	TeamCards  MarketCode = "T_CARDS"
)

var (
	football   = []Sport{SportFootball}
	basketball = []Sport{SportBasketball}
	gridiron   = []Sport{SportAmericanFootball}
	tennis     = []Sport{SportTennis}
	esports    = []Sport{SportEsports}
	usSports   = []Sport{SportBasketball, SportAmericanFootball}
	headToHead = []Sport{SportBasketball, SportAmericanFootball, SportTennis, SportEsports}
)

var marketSpecs = []MarketSpec{
	{Code1X2, KindMatch, ShapeThreeWay, football, true, "Match result", ""},
	{CodeDC, KindMatch, ShapeLabeled, football, true, "Double chance", ""},
	{CodeOU, KindMatch, ShapeOverUnder, football, true, "Total goals", ""},
	{CodeBTTS, KindMatch, ShapeYesNo, football, true, "Both teams score", ""},
	{CodeDNB, KindMatch, ShapeTwoWay, football, true, "Draw no bet", ""},
	{CodeAH, KindMatch, ShapeHandicap, football, true, "Asian handicap", ""},
	{CodeCS, KindMatch, ShapeLabeled, football, true, "Correct score", ""},
	{CodeHTFT, KindMatch, ShapeLabeled, football, true, "Half time / full time", ""},
	{Code1H1X2, KindMatch, ShapeThreeWay, football, true, "First half result", ""},
	{CodeCornOU, KindMatch, ShapeOverUnder, football, true, "Total corners", ""},
	{CodeQualify, KindMatch, ShapeTwoWay, football, false, "To qualify", ""},

	{CodeMoneyline, KindMatch, ShapeTwoWay, headToHead, false, "Match winner", ""},
	{CodeSpread, KindMatch, ShapeHandicap, usSports, false, "Point spread", ""},
	{CodeTotal, KindMatch, ShapeOverUnder, usSports, false, "Total points", ""},
	{CodeSetHcp, KindMatch, ShapeHandicap, tennis, false, "Set handicap", ""},
	{CodeTotalGames, KindMatch, ShapeOverUnder, tennis, false, "Total games", ""},
	{CodeMapHcp, KindMatch, ShapeHandicap, esports, false, "Map handicap", ""},
	{CodeTotalMaps, KindMatch, ShapeOverUnder, esports, false, "Total maps", ""},

	{PropAnytime, KindPlayer, ShapeYesNo, football, true, "Anytime scorer", "goals"},
	{PropFirst, KindPlayer, ShapeYesNo, football, true, "First scorer", "goals"},
	{PropLast, KindPlayer, ShapeYesNo, football, true, "Last scorer", "goals"},
	{PropGoals, KindPlayer, ShapeOverUnder, football, true, "Goals", "goals"},
	{PropShots, KindPlayer, ShapeOverUnder, football, true, "Shots", "shots"},
	{PropSOT, KindPlayer, ShapeOverUnder, football, true, "Shots on target", "shotsOnTarget"},
	{PropAssists, KindPlayer, ShapeOverUnder, football, true, "Assists", "assists"},
	{PropGA, KindPlayer, ShapeYesNo, football, true, "Goal or assist", ""},
	{PropCard, KindPlayer, ShapeYesNo, football, true, "To be booked", ""},
	{PropFouls, KindPlayer, ShapeOverUnder, football, true, "Fouls", "fouls"},
	{PropTackles, KindPlayer, ShapeOverUnder, football, true, "Tackles", "tackles"},
	{PropPasses, KindPlayer, ShapeOverUnder, football, true, "Passes", ""},
	{PropSaves, KindPlayer, ShapeOverUnder, football, true, "Goalkeeper saves", ""},
	{PropRating, KindPlayer, ShapeYesNo, football, true, "Sofascore rating", "rating"},

	{PropPoints, KindPlayer, ShapeOverUnder, basketball, false, "Points", "points"},
	{PropRebounds, KindPlayer, ShapeOverUnder, basketball, false, "Rebounds", "rebounds"},
	{PropBkAssists, KindPlayer, ShapeOverUnder, basketball, false, "Assists", "assists"},
	{PropThrees, KindPlayer, ShapeOverUnder, basketball, false, "Threes made", "threes"},
	{PropPtsRebAst, KindPlayer, ShapeOverUnder, basketball, false, "Points, rebounds and assists", ""},
	{PropPassYards, KindPlayer, ShapeOverUnder, gridiron, false, "Passing yards", "passYards"},
	{PropRushYards, KindPlayer, ShapeOverUnder, gridiron, false, "Rushing yards", "rushYards"},
	{PropRecYards, KindPlayer, ShapeOverUnder, gridiron, false, "Receiving yards", "recYards"},
	{PropReceptions, KindPlayer, ShapeOverUnder, gridiron, false, "Receptions", "receptions"},
	{PropPassTDs, KindPlayer, ShapeOverUnder, gridiron, false, "Passing touchdowns", "passTDs"},
	{PropAnytimeTD, KindPlayer, ShapeYesNo, gridiron, false, "Anytime touchdown", ""},
	{PropAces, KindPlayer, ShapeOverUnder, tennis, false, "Aces", "aces"},
	{PropDoubleFlt, KindPlayer, ShapeOverUnder, tennis, false, "Double faults", "doubleFaults"},
	{PropMapKills, KindPlayer, ShapeOverUnder, esports, false, "Kills", "kills"},

	{TeamGoals, KindTeam, ShapeOverUnder, football, true, "Team goals", ""},
	{TeamShots, KindTeam, ShapeBuckets, football, true, "Team shots", ""},
	{TeamPasses, KindTeam, ShapeBuckets, football, true, "Team passes", ""},
	{TeamCards, KindTeam, ShapeBuckets, football, true, "Team cards", ""},
}

var specByCode = func() map[MarketCode]MarketSpec {
	m := make(map[MarketCode]MarketSpec, len(marketSpecs))
	for _, s := range marketSpecs {
		m[s.Code] = s
	}
	return m
}()

func SpecOf(code MarketCode) (MarketSpec, bool) {
	s, ok := specByCode[code]
	return s, ok
}

func (c MarketCode) Kind() MarketKind { return specByCode[c].Kind }

func (c MarketCode) IsPlayerProp() bool { return c.Kind() == KindPlayer }

func (c MarketCode) IsTeamStat() bool { return c.Kind() == KindTeam }

func (c MarketCode) DefaultRule() SettleRule {
	return SettleRule{RegulationOnly: specByCode[c].RegulationOnly}
}

func (s MarketSpec) IsFor(sport Sport) bool { return slices.Contains(s.Sports, sport) }

func MarketsFor(sport Sport, kind MarketKind) []MarketCode {
	var codes []MarketCode
	for _, s := range marketSpecs {
		if s.Kind == kind && s.IsFor(sport) {
			codes = append(codes, s.Code)
		}
	}
	return codes
}

var (
	MatchMarkets    = []MarketCode{Code1X2, CodeDC, CodeOU, CodeBTTS, CodeDNB, CodeAH, CodeCS, CodeHTFT, Code1H1X2, CodeCornOU}
	CupMarkets      = []MarketCode{CodeQualify}
	PlayerProps     = MarketsFor(SportFootball, KindPlayer)
	TeamStatMarkets = MarketsFor(SportFootball, KindTeam)
)

func InCanonicalOrder(supplied map[MarketCode]bool) []MarketCode {
	var codes []MarketCode
	for _, s := range marketSpecs {
		if supplied[s.Code] {
			codes = append(codes, s.Code)
		}
	}
	return codes
}
