package prizepicks

import (
	"fmt"

	json "github.com/goccy/go-json"
)

const (
	resourceTypePlayer = "new_player"
	resourceTypeGame   = "game"
)

type board struct {
	projections []projectionResource
	players     map[string]playerAttributes
	games       map[string]gameAttributes
}

func decodeBoard(body []byte) (board, error) {
	var doc projectionsDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return board{}, fmt.Errorf("decode projections: %w", err)
	}
	b := board{projections: doc.Data, players: map[string]playerAttributes{}, games: map[string]gameAttributes{}}
	for _, res := range doc.Included {
		if err := b.index(res); err != nil {
			return board{}, err
		}
	}
	return b, nil
}

func (b *board) index(res includedResource) error {
	switch res.Type {
	case resourceTypePlayer:
		var attrs playerAttributes
		if err := json.Unmarshal(res.Attributes, &attrs); err != nil {
			return fmt.Errorf("decode player %s: %w", res.ID, err)
		}
		b.players[res.ID] = attrs
	case resourceTypeGame:
		var attrs gameAttributes
		if err := json.Unmarshal(res.Attributes, &attrs); err != nil {
			return fmt.Errorf("decode game %s: %w", res.ID, err)
		}
		b.games[res.ID] = attrs
	}
	return nil
}
