// Package follower owns copier position mappings and pre-trade risk decisions.
// The terminal adapter can be an MT5 bridge without moving these rules back
// into a scripting runtime.
package follower

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/rnpproject/algentra-backend/internal/copier/lot"
	"github.com/rnpproject/algentra-backend/internal/copier/protocol"
)

type Position struct {
	Ticket        any     `json:"ticket"`
	VolumeInitial float64 `json:"volume_initial"`
	VolumeCurrent float64 `json:"volume_current"`
	MasterInitial float64 `json:"master_initial"`
	Side          string  `json:"side"`
	Symbol        string  `json:"symbol"`
	StopLoss      float64 `json:"sl,omitempty"`
	TakeProfit    float64 `json:"tp,omitempty"`
}

type Limits struct {
	MaxPositions int
	MaxLots      float64
}

type Engine struct {
	Positions     map[int64]Position
	ActualVolumes []float64
	actualKnown   bool
	Netting       bool
	DryRun        bool
	Limits        Limits
}

func New(limits Limits, netting, dryRun bool, initial map[int64]Position) (*Engine, error) {
	if limits.MaxPositions <= 0 || !finite(limits.MaxLots) || limits.MaxLots <= 0 {
		return nil, errors.New("batas max_posisi_terbuka dan max_lot_total harus lebih besar dari 0")
	}
	positions := make(map[int64]Position, len(initial))
	for id, position := range initial {
		if id <= 0 || !validPosition(position) {
			return nil, fmt.Errorf("state posisi master %d tidak valid", id)
		}
		positions[id] = position
	}
	return &Engine{Positions: positions, Netting: netting, DryRun: dryRun, Limits: limits}, nil
}

// CanOpen performs the pre-trade risk check. Hedging checks require a fresh
// snapshot from SetActualPositions, matching the old worker's fail-closed
// behavior when MT5 cannot report open positions.
func (e *Engine) CanOpen(signal protocol.Signal, symbol, side string, volume float64) string {
	if signal.Action != "buka" || signal.Lot == nil || signal.PosID <= 0 || symbol == "" || (side != "buy" && side != "sell") || !finite(volume) || volume <= 0 {
		return "gagal_parameter_buka"
	}
	if _, exists := e.Positions[signal.PosID]; exists {
		return "lewati_sudah_terpetakan"
	}
	position := Position{VolumeInitial: volume, VolumeCurrent: volume, MasterInitial: *signal.Lot, Side: side, Symbol: symbol, StopLoss: value(signal.SL), TakeProfit: value(signal.TP)}
	proposed := clone(e.Positions)
	proposed[signal.PosID] = position
	if e.Netting {
		if reason := CheckNettingLimits(proposed, e.Limits); reason != "" {
			return "lewati_batas_pengaman"
		}
	} else {
		if !e.actualKnown {
			return "gagal_positions_get"
		}
		if reason := CheckHedgingLimits(e.ActualVolumes, e.Positions, volume, e.Limits, e.DryRun); reason != "" {
			return "lewati_batas_pengaman"
		}
	}
	return ""
}

// Open commits a master mapping after the terminal adapter has confirmed the
// follower execution (or immediately in dry-run mode).
func (e *Engine) Open(signal protocol.Signal, symbol, side string, volume float64) (string, error) {
	if signal.Action != "buka" || signal.Lot == nil || signal.PosID <= 0 || symbol == "" || (side != "buy" && side != "sell") || !finite(volume) || volume <= 0 {
		return "", errors.New("parameter posisi buka tidak valid")
	}
	if _, exists := e.Positions[signal.PosID]; exists {
		return "lewati_sudah_terpetakan", nil
	}
	proposed := clone(e.Positions)
	proposed[signal.PosID] = Position{VolumeInitial: volume, VolumeCurrent: volume, MasterInitial: *signal.Lot, Side: side, Symbol: symbol, StopLoss: value(signal.SL), TakeProfit: value(signal.TP)}
	e.Positions = proposed
	if e.DryRun {
		return "sukses_dry_run", nil
	}
	return "sukses", nil
}

// SetActualPositions refreshes the terminal's current gross open volumes
// before evaluating a hedging entry. Failure to fetch positions should be
// handled by the adapter as a failed refresh, not as an empty snapshot.
func (e *Engine) SetActualPositions(volumes []float64) error {
	copyOf := append([]float64(nil), volumes...)
	for _, volume := range copyOf {
		if !finite(volume) || volume < 0 {
			return errors.New("volume posisi MT5 tidak valid")
		}
	}
	e.ActualVolumes = copyOf
	e.actualKnown = true
	return nil
}

func (e *Engine) Modify(signal protocol.Signal) string {
	position, ok := e.Positions[signal.PosID]
	if !ok {
		return "lewati_tidak_terpetakan"
	}
	position.StopLoss, position.TakeProfit = value(signal.SL), value(signal.TP)
	e.Positions[signal.PosID] = position
	if e.DryRun {
		return "sukses_dry_run"
	}
	if e.Netting {
		return "sukses_virtual_stop"
	}
	return "sukses"
}

// PlanPartialClose calculates the volume and resulting mapping without
// mutating state. A live adapter commits only after the broker confirms it.
func (e *Engine) PlanPartialClose(signal protocol.Signal, info lot.SymbolInfo) (float64, float64, string) {
	position, ok := e.Positions[signal.PosID]
	if !ok {
		return 0, position.VolumeCurrent, "lewati_tidak_terpetakan"
	}
	if signal.LotLeft == nil || position.MasterInitial <= 0 || info.Step <= 0 || info.Minimum <= 0 {
		return 0, position.VolumeCurrent, "gagal_volume_partial_close"
	}
	target := position.VolumeInitial * *signal.LotLeft / position.MasterInitial
	var closeVolume float64
	if target < info.Minimum {
		closeVolume = position.VolumeCurrent
	} else {
		closeVolume = floorStep(math.Max(0, position.VolumeCurrent-target), info.Step)
		if closeVolume < info.Minimum {
			return 0, position.VolumeCurrent, "lewati_di_bawah_minimum"
		}
		if position.VolumeCurrent-closeVolume < info.Minimum {
			closeVolume = position.VolumeCurrent
		}
	}
	remaining := math.Max(0, position.VolumeCurrent-closeVolume)
	if e.Netting && remaining > 1e-10 && remaining < info.Minimum-1e-10 {
		return 0, position.VolumeCurrent, "lewati_di_bawah_minimum"
	}
	return closeVolume, remaining, "sukses"
}

func (e *Engine) CommitPartialClose(posID int64, remaining float64) error {
	position, ok := e.Positions[posID]
	if !ok {
		return errors.New("posisi master tidak terpetakan")
	}
	if !finite(remaining) || remaining < 0 || remaining > position.VolumeCurrent+1e-10 {
		return errors.New("sisa volume partial close tidak valid")
	}
	if remaining <= 1e-10 {
		delete(e.Positions, posID)
		return nil
	}
	position.VolumeCurrent = remaining
	e.Positions[posID] = position
	return nil
}

func (e *Engine) PlanClose(signal protocol.Signal) (float64, string) {
	position, ok := e.Positions[signal.PosID]
	if !ok {
		return 0, "lewati_tidak_terpetakan"
	}
	return position.VolumeCurrent, "sukses"
}

func (e *Engine) CommitClose(posID int64) { delete(e.Positions, posID) }

func CheckVirtualLimits(positions map[int64]Position, limits Limits) string {
	if limits.MaxPositions <= 0 || !finite(limits.MaxLots) || limits.MaxLots <= 0 {
		return "batas max_posisi_terbuka dan max_lot_total wajib dikonfigurasi"
	}
	if len(positions) > limits.MaxPositions {
		return fmt.Sprintf("batas max_posisi_terbuka=%d akan terlampaui", limits.MaxPositions)
	}
	var gross float64
	for _, position := range positions {
		gross += position.VolumeCurrent
	}
	if gross > limits.MaxLots+1e-10 {
		return fmt.Sprintf("batas max_lot_total=%g akan terlampaui", limits.MaxLots)
	}
	return ""
}

func CheckHedgingLimits(actual []float64, virtual map[int64]Position, proposed float64, limits Limits, dryRun bool) string {
	if limits.MaxPositions <= 0 || !finite(limits.MaxLots) || limits.MaxLots <= 0 {
		return "batas max_posisi_terbuka dan max_lot_total wajib dikonfigurasi"
	}
	count, gross := len(actual), 0.0
	for _, volume := range actual {
		if !finite(volume) || volume < 0 {
			return "volume posisi MT5 tidak valid"
		}
		gross += volume
	}
	if dryRun {
		count += len(virtual)
		for _, position := range virtual {
			gross += position.VolumeCurrent
		}
	}
	if count >= limits.MaxPositions {
		return fmt.Sprintf("batas max_posisi_terbuka=%d tercapai", limits.MaxPositions)
	}
	if gross+proposed > limits.MaxLots+1e-10 {
		return fmt.Sprintf("batas max_lot_total=%g akan terlampaui", limits.MaxLots)
	}
	return ""
}

func SignedExposure(positions map[int64]Position, symbol string) float64 {
	var total float64
	for _, position := range positions {
		if position.Symbol != symbol {
			continue
		}
		if position.Side == "buy" {
			total += position.VolumeCurrent
		} else {
			total -= position.VolumeCurrent
		}
	}
	return total
}

func CheckNettingLimits(positions map[int64]Position, limits Limits) string {
	if limits.MaxPositions <= 0 || !finite(limits.MaxLots) || limits.MaxLots <= 0 {
		return "batas max_posisi_terbuka dan max_lot_total wajib dikonfigurasi"
	}
	symbols := make(map[string]struct{})
	gross := 0.0
	for _, position := range positions {
		gross += position.VolumeCurrent
		if position.VolumeCurrent > 1e-10 {
			symbols[position.Symbol] = struct{}{}
		}
	}
	if len(symbols) > limits.MaxPositions {
		return fmt.Sprintf("batas max_posisi_terbuka=%d akan terlampaui", limits.MaxPositions)
	}
	if gross > limits.MaxLots+1e-10 {
		return fmt.Sprintf("batas max_lot_total=%g akan terlampaui", limits.MaxLots)
	}
	return ""
}

func ApplyReverse(side string, reverse bool) string {
	if reverse && side == "buy" {
		return "sell"
	}
	if reverse && side == "sell" {
		return "buy"
	}
	return side
}

func validPosition(position Position) bool {
	return position.Symbol != "" && (position.Side == "buy" || position.Side == "sell") && finite(position.VolumeInitial) && position.VolumeInitial > 0 && finite(position.VolumeCurrent) && position.VolumeCurrent >= 0 && finite(position.MasterInitial) && position.MasterInitial > 0
}

func clone(source map[int64]Position) map[int64]Position {
	result := make(map[int64]Position, len(source)+1)
	for id, position := range source {
		result[id] = position
	}
	return result
}

func value(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func floorStep(value, step float64) float64 {
	units := math.Floor(value/step + 1e-10)
	return math.Round(units*step*1e8) / 1e8
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func NormalizeSymbol(masterSymbol string, mapping map[string]string) string {
	if value, ok := mapping[masterSymbol]; ok && strings.TrimSpace(value) != "" {
		return value
	}
	return masterSymbol
}
