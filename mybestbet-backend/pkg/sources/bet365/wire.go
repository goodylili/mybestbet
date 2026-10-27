package bet365

type section struct {
	kind   string
	fields map[string]string
}

type fixture struct {
	id          string
	name        string
	competition string
	startsAt    string
	groups      []*marketGroup
}

type marketGroup struct {
	id      string
	name    string
	columns []*column
}

type column struct {
	name         string
	participants []participant
}

type participant struct {
	id          string
	name        string
	odds        string
	handicap    string
	isSuspended bool
}
