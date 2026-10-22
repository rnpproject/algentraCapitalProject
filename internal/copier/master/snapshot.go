// Package master converts terminal position snapshots into copier signals.
package master

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/rnpproject/algentra-backend/internal/copier/protocol"
	"github.com/rnpproject/algentra-backend/internal/textutil"
)

type Position struct {
	Identifier int64   `json:"identifier"`
	Ticket     int64   `json:"ticket"`
	Magic      int64   `json:"magic"`
	Symbol     string  `json:"symbol"`
	Type       int     `json:"type"`
	Volume     float64 `json:"volume"`
	OpenPrice  float64 `json:"price_open"`
	StopLoss   float64 `json:"sl"`
	TakeProfit float64 `json:"tp"`
	Comment    string  `json:"comment"`
}

type Filter struct {
	Magic       map[int64]struct{}
	Symbols     map[string]struct{}
	IgnoreWords []string
	BuyType     int
}

type Snapshot map[int64]Position

type IDGenerator struct {
	lastMillis int64
	counter    uint64
}

func (g *IDGenerator) Next(now time.Time) string {
	millis := now.UnixMilli()
	if millis > g.lastMillis {
		g.lastMillis, g.counter = millis, 0
	} else {
		g.counter++
	}
	return strconv.FormatInt(g.lastMillis, 10) + "-" + strconv.FormatUint(g.counter, 10)
}

// Stamp supplies the time and monotonic process-local IDs required by the
// shared JSONL signal protocol.
func Stamp(events []protocol.Signal, ids *IDGenerator, now time.Time) []protocol.Signal {
	for i := range events {
		events[i].ID = ids.Next(now)
		events[i].TS = float64(now.UnixNano()) / float64(time.Second)
	}
	return events
}

// BuildSnapshot applies the same optional magic, symbol, and comment filters
// used by the former MT5 publisher. Identifier falls back to ticket.
func BuildSnapshot(positions []Position, filter Filter) (Snapshot, error) {
	out := make(Snapshot, len(positions))
	for _, p := range positions {
		if len(filter.Magic) > 0 {
			if _, ok := filter.Magic[p.Magic]; !ok {
				continue
			}
		}
		if len(filter.Symbols) > 0 {
			if _, ok := filter.Symbols[p.Symbol]; !ok {
				continue
			}
		}
		comment := textutil.Fold(p.Comment)
		ignored := false
		for _, word := range filter.IgnoreWords {
			if strings.Contains(comment, textutil.Fold(word)) {
				ignored = true
				break
			}
		}
		if ignored {
			continue
		}
		id := p.Identifier
		if id == 0 {
			id = p.Ticket
		}
		if id <= 0 || strings.TrimSpace(p.Symbol) == "" || !finite(p.Volume) || !finite(p.OpenPrice) || !finite(p.StopLoss) || !finite(p.TakeProfit) {
			return nil, fmt.Errorf("posisi MT5 %d berisi nilai tidak valid", id)
		}
		p.Identifier = id
		if _, exists := out[id]; exists {
			return nil, fmt.Errorf("identifier posisi MT5 duplikat: %d", id)
		}
		out[id] = p
	}
	return out, nil
}

// Diff emits open, stop updates, partial closes, and full closes. A nil
// position list is represented by the caller as no update and must not be
// passed here.
func Diff(previous, current Snapshot, buyType int) []protocol.Signal {
	var events []protocol.Signal
	for id, now := range current {
		old, exists := previous[id]
		if !exists {
			side := "sell"
			if now.Type == buyType {
				side = "buy"
			}
			symbol := now.Symbol
			lot, price, sl, tp := now.Volume, now.OpenPrice, now.StopLoss, now.TakeProfit
			events = append(events, protocol.Signal{Version: 1, Action: "buka", PosID: id, Symbol: &symbol, Side: &side, Lot: &lot, Price: &price, SL: &sl, TP: &tp, Comment: now.Comment})
			continue
		}
		if now.StopLoss != old.StopLoss || now.TakeProfit != old.TakeProfit {
			sl, tp := now.StopLoss, now.TakeProfit
			events = append(events, protocol.Signal{Version: 1, Action: "ubah", PosID: id, SL: &sl, TP: &tp})
		}
		if now.Volume < old.Volume-1e-10 {
			closed := old.Volume - now.Volume
			left := now.Volume
			events = append(events, protocol.Signal{Version: 1, Action: "kurang", PosID: id, Lot: &closed, LotLeft: &left})
		}
	}
	for id := range previous {
		if _, exists := current[id]; !exists {
			events = append(events, protocol.Signal{Version: 1, Action: "tutup", PosID: id})
		}
	}
	return events
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
