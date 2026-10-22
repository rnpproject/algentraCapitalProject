// Package managed replays the master's daily signal log into a snapshot that
// a managed MT5 follower can consume without importing the Python runtime.
package managed

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/rnpproject/algentra-backend/internal/copier/protocol"
	"github.com/rnpproject/algentra-backend/internal/copier/signalio"
)

type Position struct {
	Ticket     string  `json:"ticket"`
	Symbol     string  `json:"symbol"`
	Action     string  `json:"action"`
	Lots       float64 `json:"lots"`
	EntryPrice float64 `json:"entry_price"`
	StopLoss   float64 `json:"sl"`
	TakeProfit float64 `json:"tp"`
	OpenedAt   float64 `json:"opened_at"`
}

type snapshotState struct {
	SignalDate string              `json:"signal_date"`
	Offset     int64               `json:"offset"`
	Positions  map[string]Position `json:"positions"`
	Processed  map[string]bool     `json:"processed"`
}

type SnapshotStore struct {
	mu        sync.Mutex
	folder    string
	statePath string
	maxAge    time.Duration
	state     snapshotState
}

func OpenSnapshotStore(folder, statePath string, maxAge time.Duration) (*SnapshotStore, error) {
	if folder == "" || statePath == "" || maxAge <= 0 {
		return nil, errors.New("folder sinyal, path state, dan usia sinyal maksimum wajib diisi")
	}
	store := &SnapshotStore{folder: folder, statePath: statePath, maxAge: maxAge}
	store.state = snapshotState{Positions: map[string]Position{}, Processed: map[string]bool{}}
	data, err := os.ReadFile(statePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("state snapshot tidak dapat dibaca: %w", err)
	}
	if err == nil {
		if err := json.Unmarshal(data, &store.state); err != nil || store.state.Positions == nil || store.state.Processed == nil {
			return nil, errors.New("format state snapshot managed tidak valid")
		}
		if store.state.Offset < 0 {
			return nil, errors.New("offset state snapshot managed tidak valid")
		}
	}
	return store, nil
}

// Sync consumes complete signal lines, applies the same age gate to new
// entries as the follower, and persists offsets and the effective master
// position set together before returning it to an EA adapter.
func (s *SnapshotStore) Sync(now time.Time) ([]Position, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.SignalDate == "" {
		s.state.SignalDate = now.Local().Format("2006-01-02")
		s.state.Offset = 0
	}
	activeDate, err := time.ParseInLocation("2006-01-02", s.state.SignalDate, time.Local)
	if err != nil {
		return nil, errors.New("tanggal state snapshot managed tidak valid")
	}
	path := signalio.Path(s.folder, activeDate)
	read, err := signalio.ReadNew(path, s.state.Offset)
	if err != nil {
		return nil, err
	}
	for _, record := range read.Records {
		s.apply(record.Signal, now)
		s.state.Offset = record.EndOffset
	}
	if len(read.Records) == 0 {
		s.state.Offset = read.Offset
	} else if read.Offset > s.state.Offset {
		s.state.Offset = read.Offset
	}
	today := now.Local()
	todayAtMidnight := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	if activeDate.Before(todayAtMidnight) && !read.HasPartialLine {
		info, statErr := os.Stat(path)
		if errors.Is(statErr, os.ErrNotExist) || (statErr == nil && s.state.Offset >= info.Size()) {
			s.state.SignalDate = activeDate.AddDate(0, 0, 1).Format("2006-01-02")
			s.state.Offset = 0
		}
	}
	if err := s.save(); err != nil {
		return nil, err
	}
	positions := make([]Position, 0, len(s.state.Positions))
	for _, position := range s.state.Positions {
		positions = append(positions, position)
	}
	sort.Slice(positions, func(i, j int) bool {
		a, _ := strconv.ParseInt(positions[i].Ticket, 10, 64)
		b, _ := strconv.ParseInt(positions[j].Ticket, 10, 64)
		if a != b {
			return a < b
		}
		return positions[i].Ticket < positions[j].Ticket
	})
	return positions, nil
}

func (s *SnapshotStore) apply(signal protocol.Signal, now time.Time) {
	if s.state.Processed[signal.ID] {
		return
	}
	key := strconv.FormatInt(signal.PosID, 10)
	switch signal.Action {
	case "buka":
		if signal.Lot == nil || signal.Symbol == nil || signal.Side == nil || now.Sub(time.UnixMilli(int64(signal.TS*1000))) > s.maxAge {
			break
		}
		action := "BUY"
		if *signal.Side == "sell" {
			action = "SELL"
		}
		s.state.Positions[key] = Position{
			Ticket: key, Symbol: *signal.Symbol, Action: action, Lots: *signal.Lot,
			EntryPrice: number(signal.Price), StopLoss: number(signal.SL), TakeProfit: number(signal.TP), OpenedAt: signal.TS,
		}
	case "ubah":
		if position, ok := s.state.Positions[key]; ok {
			position.StopLoss, position.TakeProfit = number(signal.SL), number(signal.TP)
			s.state.Positions[key] = position
		}
	case "kurang":
		if position, ok := s.state.Positions[key]; ok && signal.LotLeft != nil {
			if *signal.LotLeft <= 0 {
				delete(s.state.Positions, key)
			} else {
				position.Lots = *signal.LotLeft
				s.state.Positions[key] = position
			}
		}
	case "tutup":
		delete(s.state.Positions, key)
	}
	s.state.Processed[signal.ID] = true
}

func (s *SnapshotStore) save() error {
	data, err := json.Marshal(s.state)
	if err != nil {
		return fmt.Errorf("state snapshot managed tidak dapat dienkode: %w", err)
	}
	dir := filepath.Dir(s.statePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, filepath.Base(s.statePath)+".*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, s.statePath); err != nil {
		return fmt.Errorf("state snapshot managed tidak dapat diganti: %w", err)
	}
	return nil
}

func number(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}
