package notify

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"mybestbet/pkg/canon"
)

const stakeBankroll = 100

func formatAlert(signal canon.Signal, sport canon.Sport, details alertContext, webBaseURL string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s</b>%s\n", headline(signal), competitionLabel(signal.LeagueCode, sport))
	fmt.Fprintf(&b, "%s vs %s · %s\n", html.EscapeString(details.Home), html.EscapeString(details.Away), details.Kickoff.UTC().Format("Mon 2 Jan 15:04 MST"))
	fmt.Fprintf(&b, "Market: %s\n", html.EscapeString(marketLabel(signal.Market, details.PlayerName)))
	stakes := arbStakes(signal)
	for i, leg := range signal.Legs {
		b.WriteString(formatLeg(leg, stakes, i))
	}
	if webBaseURL != "" {
		fmt.Fprintf(&b, "<a href=\"%s\">Open board</a>", html.EscapeString(strings.TrimRight(webBaseURL, "/")+"/event/"+signal.EventID))
	}
	return b.String()
}

func headline(signal canon.Signal) string {
	switch signal.Type {
	case canon.SignalArb:
		return "ARB " + percentLabel(signal.Margin)
	case canon.SignalEV:
		return fmt.Sprintf("+EV %s (fair %s)", percentLabel(signal.EV), percentLabel(signal.FairProb))
	case canon.SignalMove:
		return "MOVE " + percentLabel(signal.Margin)
	case canon.SignalLine:
		return "LINE gap " + strconv.FormatFloat(signal.Margin, 'f', -1, 64)
	}
	return strings.ToUpper(string(signal.Type))
}

func competitionLabel(leagueCode string, sport canon.Sport) string {
	var label string
	for _, part := range []string{leagueCode, string(sport)} {
		if part != "" {
			label += " · " + html.EscapeString(part)
		}
	}
	return label
}

var periodNames = map[string]string{
	canon.PeriodFullTime:   "full game",
	canon.PeriodFirstHalf:  "1st half",
	canon.PeriodSecondHalf: "2nd half",
	canon.PeriodQuarter1:   "1st quarter",
	canon.PeriodQuarter2:   "2nd quarter",
	canon.PeriodQuarter3:   "3rd quarter",
	canon.PeriodQuarter4:   "4th quarter",
	canon.PeriodFirstSet:   "1st set",
	canon.PeriodFirstMap:   "map 1",
}

func periodLabel(period string) string {
	if name, ok := periodNames[period]; ok {
		return name
	}
	return period
}

func percentLabel(fraction float64) string {
	return strconv.FormatFloat(fraction*100, 'f', 2, 64) + "%"
}

func marketLabel(key canon.MarketKey, playerName string) string {
	parts := []string{string(key.Code)}
	if key.Line != nil {
		parts = append(parts, strconv.FormatFloat(*key.Line, 'f', -1, 64))
	}
	parts = append(parts, "("+periodLabel(key.Period)+")")
	if playerName != "" {
		parts = append(parts, "· "+playerName)
	}
	return strings.Join(parts, " ")
}

func formatLeg(leg canon.Leg, stakes []float64, index int) string {
	selection := string(leg.Side)
	if leg.Label != "" {
		selection = strings.TrimSpace(selection + " " + leg.Label)
	}
	line := fmt.Sprintf("• %s @ %s %s", html.EscapeString(selection), html.EscapeString(leg.Source), priceLabel(leg.Price))
	if index < len(stakes) {
		line += fmt.Sprintf(" → stake %.2f", stakes[index])
	}
	if leg.DeepLink != "" {
		line += fmt.Sprintf(" <a href=\"%s\">bet</a>", html.EscapeString(leg.DeepLink))
	}
	return line + "\n"
}

func priceLabel(price canon.Price) string {
	switch price.Form {
	case canon.FormDecimal:
		return strconv.FormatFloat(price.Raw, 'f', 2, 64)
	case canon.FormShare:
		return strconv.FormatFloat(price.Raw, 'f', 3, 64) + decimalEquivalent(price.Prob)
	case canon.FormCents:
		return strconv.FormatFloat(price.Raw, 'f', 0, 64) + "¢" + decimalEquivalent(price.Prob)
	case canon.FormNaira:
		return "₦" + strconv.FormatFloat(price.Raw, 'f', 0, 64) + decimalEquivalent(price.Prob)
	case canon.FormLineOnly:
		return "line"
	}
	return strconv.FormatFloat(price.Raw, 'f', -1, 64)
}

func decimalEquivalent(prob float64) string {
	if prob <= 0 {
		return ""
	}
	return " (" + strconv.FormatFloat(1/prob, 'f', 2, 64) + ")"
}

func arbStakes(signal canon.Signal) []float64 {
	if signal.Type != canon.SignalArb {
		return nil
	}
	if stakes, ok := engineStakes(signal.Legs); ok {
		return stakes
	}
	total := 0.0
	for _, leg := range signal.Legs {
		if leg.Price.Prob <= 0 {
			return nil
		}
		total += leg.Price.Prob
	}
	stakes := make([]float64, len(signal.Legs))
	for i, leg := range signal.Legs {
		stakes[i] = stakeBankroll * leg.Price.Prob / total
	}
	return stakes
}

func engineStakes(legs []canon.Leg) ([]float64, bool) {
	stakes := make([]float64, len(legs))
	for i, leg := range legs {
		if leg.Stake <= 0 {
			return nil, false
		}
		stakes[i] = leg.Stake
	}
	return stakes, true
}
