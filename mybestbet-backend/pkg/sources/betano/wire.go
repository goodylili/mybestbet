package betano

type sportResponse struct {
	Data sportData `json:"data"`
}

type sportData struct {
	TopLeagues   []league      `json:"topLeagues"`
	RegionGroups []regionGroup `json:"regionGroups"`
}

type regionGroup struct {
	Regions []region `json:"regions"`
}

type region struct {
	Name    string   `json:"name"`
	Leagues []league `json:"leagues"`
}

type league struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	RegionName string `json:"regionName"`
	URL        string `json:"url"`
}

type leagueResponse struct {
	Data leagueData `json:"data"`
}

type leagueData struct {
	Blocks []block `json:"blocks"`
}

type block struct {
	Events []event `json:"events"`
}

type event struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	StartTime    int64         `json:"startTime"`
	URL          string        `json:"url"`
	Participants []participant `json:"participants"`
	Markets      []market      `json:"markets"`
}

type participant struct {
	Name string `json:"name"`
}

type matchOddsResponse struct {
	Data matchOddsData `json:"data"`
}

type matchOddsData struct {
	Event   *event   `json:"event"`
	Markets []market `json:"markets"`
}

type market struct {
	ID          string       `json:"id"`
	Type        string       `json:"type"`
	Selections  []selection  `json:"selections"`
	TableLayout *tableLayout `json:"tableLayout"`
}

type selection struct {
	Name        string  `json:"name"`
	Price       float64 `json:"price"`
	Handicap    float64 `json:"handicap"`
	ColumnIndex int     `json:"columnIndex"`
}

type tableLayout struct {
	Columns []tableColumn `json:"columns"`
	Rows    []tableRow    `json:"rows"`
}

type tableColumn struct {
	Name string `json:"name"`
}

type tableRow struct {
	Title           string           `json:"title"`
	GroupSelections []selectionGroup `json:"groupSelections"`
}

type selectionGroup struct {
	Selections []selection `json:"selections"`
}
