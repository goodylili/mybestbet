package ingest

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/cespare/xxhash/v2"

	"mybestbet/pkg/canon"
)

const identitySeparator = "|"

type hashedMarket struct {
	market   canon.RawMarket
	identity string
	hash     uint64
}

type marketHashes struct {
	mu      sync.Mutex
	byEvent map[string]map[string]uint64
}

func newMarketHashes() *marketHashes { return &marketHashes{byEvent: map[string]map[string]uint64{}} }

func (h *marketHashes) changed(markets []canon.RawMarket) []hashedMarket {
	h.mu.Lock()
	defer h.mu.Unlock()
	changed := make([]hashedMarket, 0, len(markets))
	for _, m := range markets {
		hm := hashedMarket{market: m, identity: marketIdentity(m), hash: contentHash(m)}
		if previous, seen := h.byEvent[m.EventSourceID][hm.identity]; seen && previous == hm.hash {
			continue
		}
		changed = append(changed, hm)
	}
	return changed
}

func (h *marketHashes) commit(markets []hashedMarket) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, hm := range markets {
		event := hm.market.EventSourceID
		if h.byEvent[event] == nil {
			h.byEvent[event] = map[string]uint64{}
		}
		h.byEvent[event][hm.identity] = hm.hash
	}
}

func (h *marketHashes) forgetMissing(polled []canon.RawMarket) {
	h.mu.Lock()
	defer h.mu.Unlock()
	present := map[string]map[string]bool{}
	for _, m := range polled {
		if present[m.EventSourceID] == nil {
			present[m.EventSourceID] = map[string]bool{}
		}
		present[m.EventSourceID][marketIdentity(m)] = true
	}
	for event, identities := range present {
		for identity := range h.byEvent[event] {
			if !identities[identity] {
				delete(h.byEvent[event], identity)
			}
		}
	}
}

func (h *marketHashes) retainEvents(eventIDs []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	keep := make(map[string]bool, len(eventIDs))
	for _, id := range eventIDs {
		keep[id] = true
	}
	for id := range h.byEvent {
		if !keep[id] {
			delete(h.byEvent, id)
		}
	}
}

func marketIdentity(m canon.RawMarket) string {
	line := ""
	if m.Line != nil {
		line = strconv.FormatFloat(*m.Line, 'f', -1, 64)
	}
	player := ""
	if m.Player != nil {
		player = m.Player.SourceID + "/" + m.Player.Name
	}
	return strings.Join([]string{string(m.Code), line, player, m.Period, m.SourceMarketID}, identitySeparator)
}

func contentHash(m canon.RawMarket) uint64 {
	d := xxhash.New()
	writeBool(d, m.Rules.RegulationOnly)
	writeBool(d, m.Rules.VoidIfNoStart)
	_, _ = d.WriteString(m.Rules.Resolver)
	for _, s := range m.Selections {
		_, _ = d.WriteString(string(s.Side))
		_, _ = d.WriteString(identitySeparator + s.Label + identitySeparator)
		_, _ = d.WriteString(s.DeepLink)
		writeFloat(d, s.Price.Raw)
		_, _ = d.WriteString(string(s.Price.Form))
		writeBool(d, s.Price.Suspended)
		writeOptionalFloat(d, s.Price.Bid)
		writeOptionalFloat(d, s.Price.Ask)
		writeOptionalFloat(d, s.Price.Liquidity)
	}
	return d.Sum64()
}

func writeFloat(d *xxhash.Digest, f float64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(f))
	_, _ = d.Write(buf[:])
}

func writeOptionalFloat(d *xxhash.Digest, f *float64) {
	writeBool(d, f != nil)
	if f != nil {
		writeFloat(d, *f)
	}
}

func writeBool(d *xxhash.Digest, b bool) {
	if b {
		_, _ = d.Write([]byte{1})
		return
	}
	_, _ = d.Write([]byte{0})
}
