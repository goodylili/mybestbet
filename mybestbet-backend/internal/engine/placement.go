package engine

import "slices"

type eventPlacement struct {
	shardByEvent  map[string]int
	eventsOnShard []int
}

func newEventPlacement(shards int) eventPlacement {
	return eventPlacement{shardByEvent: map[string]int{}, eventsOnShard: make([]int, shards)}
}

func (p *eventPlacement) shardOf(eventID string) int {
	if shard, ok := p.shardByEvent[eventID]; ok {
		return shard
	}
	shard := slices.Index(p.eventsOnShard, slices.Min(p.eventsOnShard))
	p.shardByEvent[eventID] = shard
	p.eventsOnShard[shard]++
	return shard
}
