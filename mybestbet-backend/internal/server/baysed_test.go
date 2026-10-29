package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func shotsGroup(line float64, cells ...func(string) canon.BoardCell) marketGroup {
	key := canon.MarketKey{EventID: "e1", Code: canon.PropShots, Line: &line, PlayerID: "p1"}
	group := marketGroup{key: key, keyString: key.String()}
	for _, cell := range cells {
		group.cells = append(group.cells, cell(group.keyString))
	}
	return group
}

func priced(source string, side canon.Side, odds float64) func(string) canon.BoardCell {
	return func(key string) canon.BoardCell { return decimalCell(key, source, side, "", odds) }
}

func TestCompareBayseFlagsLinesBeatingEveryBook(t *testing.T) {
	t.Parallel()
	group := shotsGroup(1.5,
		priced(baysedSource, canon.SideOver, 2.6),
		priced(baysedSource, canon.SideUnder, 1.5),
		priced("sportybet", canon.SideOver, 1.9),
		priced("sportybet", canon.SideUnder, 1.9),
		priced("betway", canon.SideOver, 2.0),
		priced("betway", canon.SideUnder, 1.8),
	)
	event := EventInfo{ID: "e1", Home: Team{Name: "Arsenal"}, Away: Team{Name: "Fleetwood"}, Kickoff: time.Now()}
	market, ok := compareBayse(event, group, []marketGroup{group}, 0.05)
	require.True(t, ok)
	require.Len(t, market.Sides, 2)
	assert.Equal(t, []string{"betway", "sportybet"}, market.Books)
	assert.Equal(t, "Arsenal", market.Home)

	over := market.Sides[0]
	assert.Equal(t, "over", over.Side)
	require.NotNil(t, over.Best)
	assert.Equal(t, "betway", over.Best.Source)
	require.NotNil(t, over.Fair)
	assert.InDelta(t, (0.5+(0.5/(0.5+1/1.8)))/2, *over.Fair, 1e-9)
	assert.Equal(t, []string{flagBeatsBooks, flagOffConsensus}, over.Flags)

	under := market.Sides[1]
	assert.Equal(t, []string{flagOffConsensus}, under.Flags)
	require.NotNil(t, under.Gap)
	assert.Less(t, *under.Gap, 0.0)
	assert.ElementsMatch(t, []string{flagBeatsBooks, flagOffConsensus}, market.Flags)
}

func TestCompareBayseFlagsEasierLinePayingMoreThanAHarderOne(t *testing.T) {
	t.Parallel()
	bayse := shotsGroup(0.5, priced(baysedSource, canon.SideOver, 1.73), priced(baysedSource, canon.SideUnder, 1.73))
	low := shotsGroup(1.5, priced("sportybet", canon.SideOver, 1.4), priced("sportybet", canon.SideUnder, 2.8))
	high := shotsGroup(2.5, priced("sportybet", canon.SideOver, 2.4), priced("sportybet", canon.SideUnder, 1.5))
	family := []marketGroup{bayse, low, high}
	market, ok := compareBayse(EventInfo{ID: "e1"}, bayse, family, 0.05)
	require.True(t, ok)
	over := market.Sides[0]
	assert.Contains(t, over.Flags, flagBeatsHarder)
	require.NotNil(t, over.Harder)
	assert.Equal(t, "sportybet", over.Harder.Source)
	assert.InDelta(t, 1.5, *over.HarderLine, 0)
	assert.Nil(t, over.Best)
	assert.Equal(t, fairPoisson, over.FairModel)
	assert.Contains(t, over.Flags, flagOffConsensus)
	assert.Len(t, market.Ladder, 3)
	assert.InDelta(t, 0.5, *market.Ladder[0].Line, 0)
	assert.Equal(t, []string{"sportybet"}, market.Books)
}

func TestCompareBayseInterpolatesFairBetweenBookLines(t *testing.T) {
	t.Parallel()
	bayse := shotsGroup(2.5, priced(baysedSource, canon.SideOver, 2.0))
	low := shotsGroup(1.5, priced("betway", canon.SideOver, 1.25), priced("betway", canon.SideUnder, 5))
	high := shotsGroup(3.5, priced("betway", canon.SideOver, 5), priced("betway", canon.SideUnder, 1.25))
	market, ok := compareBayse(EventInfo{ID: "e1"}, bayse, []marketGroup{bayse, low, high}, 0.05)
	require.True(t, ok)
	over := market.Sides[0]
	require.NotNil(t, over.Fair)
	assert.InDelta(t, 0.5, *over.Fair, 1e-9)
	assert.Equal(t, fairInterpolated, over.FairModel)
	assert.Empty(t, over.Flags)
}

func TestCompareBayseSkipsMarketsBayseDoesNotPrice(t *testing.T) {
	t.Parallel()
	suspended := priced(baysedSource, canon.SideOver, 2.6)
	group := shotsGroup(1.5, func(key string) canon.BoardCell {
		cell := suspended(key)
		cell.Price.Suspended = true
		return cell
	}, priced("betway", canon.SideOver, 2))
	_, ok := compareBayse(EventInfo{ID: "e1"}, group, []marketGroup{group}, 0.05)
	assert.False(t, ok)
}

func TestCompareBayseFlagsALineAwayFromTheBooksBalancedLine(t *testing.T) {
	t.Parallel()
	bayse := shotsGroup(3.5, priced(baysedSource, canon.SideOver, 1.9), priced(baysedSource, canon.SideUnder, 1.9))
	books := shotsGroup(2.5,
		priced("betway", canon.SideOver, 1.9),
		priced("betway", canon.SideUnder, 1.9),
		priced("sportybet", canon.SideOver, 1.95),
		priced("sportybet", canon.SideUnder, 1.85),
	)
	market, ok := compareBayse(EventInfo{ID: "e1"}, bayse, []marketGroup{bayse, books}, 0.05)
	require.True(t, ok)
	require.NotNil(t, market.Balanced)
	assert.InDelta(t, 2.5, *market.Balanced, 0)
	under := market.Sides[1]
	assert.Equal(t, "under", under.Side)
	assert.Equal(t, fairPoisson, under.FairModel)
	require.NotNil(t, under.Fair)
	assert.Greater(t, *under.Fair, 0.65)
	assert.Contains(t, under.Flags, flagOffConsensus)
	assert.Less(t, *market.Sides[0].Fair, 0.35)
}

func TestPoissonTailMatchesTheFittedAnchor(t *testing.T) {
	t.Parallel()
	over, ok := poissonOver([]ladderPoint{{line: 2.5, over: 0.5}}, 2.5)
	require.True(t, ok)
	assert.InDelta(t, 0.5, over, 1e-6)
	_, ok = poissonOver(nil, 2.5)
	assert.False(t, ok)
}

func TestCompareBayseIgnoresPushLinesWhenLookingForHarderLines(t *testing.T) {
	t.Parallel()
	bayse := shotsGroup(1.5, priced(baysedSource, canon.SideOver, 1.2))
	quarter := shotsGroup(1.75, priced("betway", canon.SideOver, 1.15), priced("betway", canon.SideUnder, 5))
	whole := shotsGroup(2, priced("betway", canon.SideOver, 1.18), priced("betway", canon.SideUnder, 4.5))
	market, ok := compareBayse(EventInfo{ID: "e1"}, bayse, []marketGroup{bayse, quarter, whole}, 0.05)
	require.True(t, ok)
	assert.NotContains(t, market.Sides[0].Flags, flagBeatsHarder)
	assert.Nil(t, market.Balanced)
}
