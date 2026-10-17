package snapshot

import (
	"sync"
	"time"
)

type Account struct {
	Balance        float64
	Equity         float64
	FloatingProfit float64
	Currency       string
	TradeMode      string
	ObservedAt     time.Time
}

type Quote struct {
	Symbol  string  `json:"symbol"`
	Bid     float64 `json:"bid"`
	Ask     float64 `json:"ask"`
	TimeMSC int64   `json:"time_msc"`
}

type Snapshot struct {
	ObservedAt       time.Time
	Account          *Account
	Positions        []Position
	MarketQuotes     []Quote
	MarketObservedAt *time.Time
}

type Position struct {
	Ticket     string   `json:"ticket"`
	Symbol     string   `json:"symbol"`
	Action     string   `json:"action"`
	Lots       float64  `json:"lots"`
	EntryPrice float64  `json:"entry_price"`
	SL         *float64 `json:"sl"`
	TP         *float64 `json:"tp"`
}

type Heartbeat struct {
	Active        bool
	LastHeartbeat time.Time
	Terminal      *string
	Version       *string
	Symbol        *string
}

type Store struct {
	mu        sync.RWMutex
	current   *Snapshot
	heartbeat *Heartbeat
}

func (s *Store) PublishHeartbeat(value Heartbeat) Heartbeat {
	s.mu.Lock()
	defer s.mu.Unlock()
	value.LastHeartbeat = value.LastHeartbeat.UTC()
	s.heartbeat = &value
	return value
}

func (s *Store) CurrentHeartbeat() (Heartbeat, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.heartbeat == nil {
		return Heartbeat{}, false
	}
	return *s.heartbeat, true
}

func (s *Store) Publish(now time.Time, account *Account, positions []Position, quotes *[]Quote) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var next Snapshot
	if s.current != nil {
		next = *s.current
		next.MarketQuotes = append([]Quote(nil), s.current.MarketQuotes...)
	}
	next.ObservedAt = now.UTC()
	if account != nil {
		copy := *account
		copy.ObservedAt = now.UTC()
		next.Account = &copy
	}
	next.Positions = append([]Position(nil), positions...)
	if quotes != nil {
		next.MarketQuotes = append([]Quote(nil), (*quotes)...)
		observed := now.UTC()
		next.MarketObservedAt = &observed
	}
	s.current = &next
}

func (s *Store) Current() (Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.current == nil {
		return Snapshot{}, false
	}
	copy := *s.current
	copy.Positions = append([]Position(nil), s.current.Positions...)
	copy.MarketQuotes = append([]Quote(nil), s.current.MarketQuotes...)
	if s.current.Account != nil {
		account := *s.current.Account
		copy.Account = &account
	}
	if s.current.MarketObservedAt != nil {
		observed := *s.current.MarketObservedAt
		copy.MarketObservedAt = &observed
	}
	return copy, true
}
