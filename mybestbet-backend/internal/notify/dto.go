package notify

import "time"

const (
	defaultMinArbPercent = 0.5
	defaultMinEVPercent  = 3
)

type Subscription struct {
	ChatID        int64
	MinArbPercent float64
	MinEVPercent  float64
	Leagues       []string
	Sports        []string
	Props         bool
	MovePercent   *float64
	MutedUntil    *time.Time
}

func defaultSubscription(chatID int64) Subscription {
	return Subscription{ChatID: chatID, MinArbPercent: defaultMinArbPercent, MinEVPercent: defaultMinEVPercent, Leagues: []string{}, Sports: []string{}}
}

type telegramChat struct {
	ID int64 `json:"id"`
}

type telegramMessage struct {
	Chat telegramChat `json:"chat"`
	Text string       `json:"text"`
}

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message,omitempty"`
}

type telegramResponse[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	Description string `json:"description"`
}

type getUpdatesRequest struct {
	Offset         int64    `json:"offset"`
	Timeout        int      `json:"timeout"`
	AllowedUpdates []string `json:"allowed_updates"`
}

type sendMessageRequest struct {
	ChatID                int64  `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
}

type alertContext struct {
	Home       string
	Away       string
	Kickoff    time.Time
	PlayerName string
}
