package canon

const (
	PeriodHalfTime  = "HT"
	PeriodExtraTime = "ET"
	PeriodOvertime  = "OT"
	PeriodFinished  = "FT"
)

type EventScore struct {
	Home   int    `json:"home"`
	Away   int    `json:"away"`
	Period string `json:"period,omitempty"`
	Minute int    `json:"minute,omitempty"`
	Clock  string `json:"clock,omitempty"`
	Detail string `json:"detail,omitempty"`
}
