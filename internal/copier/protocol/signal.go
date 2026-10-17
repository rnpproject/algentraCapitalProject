// Package protocol defines the JSONL wire format shared with the MT5 EAs.
package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"
)

type Signal struct {
	Version int      `json:"v"`
	ID      string   `json:"id"`
	TS      float64  `json:"ts"`
	Action  string   `json:"aksi"`
	PosID   int64    `json:"pos_id"`
	Symbol  *string  `json:"simbol,omitempty"`
	Side    *string  `json:"arah,omitempty"`
	Lot     *float64 `json:"lot,omitempty"`
	Price   *float64 `json:"harga,omitempty"`
	SL      *float64 `json:"sl,omitempty"`
	TP      *float64 `json:"tp,omitempty"`
	Comment string   `json:"komentar"`
	LotLeft *float64 `json:"lot_sisa,omitempty"`
}

func Parse(raw []byte) (Signal, error) {
	var s Signal
	if !utf8.Valid(raw) {
		return s, errors.New("baris bukan UTF-8 yang valid")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return s, fmt.Errorf("JSON tidak valid: %w", err)
	}
	for _, key := range []string{"v", "id", "ts", "aksi", "pos_id"} {
		if _, ok := fields[key]; !ok {
			return s, fmt.Errorf("field '%s' wajib diisi", key)
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := d.Decode(&s); err != nil {
		return s, fmt.Errorf("JSON tidak valid: %w", err)
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return s, errors.New("JSON tidak valid: data tambahan setelah object")
	}
	if s.Version != 1 {
		return s, errors.New("field 'v' harus bernilai 1")
	}
	if strings.TrimSpace(s.ID) == "" {
		return s, errors.New("field 'id' harus berupa string yang tidak kosong")
	}
	if !finite(s.TS) {
		return s, errors.New("field 'ts' harus berupa angka finite")
	}
	if s.PosID <= 0 {
		return s, errors.New("field 'pos_id' harus berupa integer positif")
	}
	switch s.Action {
	case "buka":
		if s.Symbol == nil || strings.TrimSpace(*s.Symbol) == "" {
			return s, errors.New("field 'simbol' wajib untuk aksi 'buka'")
		}
		if s.Side == nil || (*s.Side != "buy" && *s.Side != "sell") {
			return s, errors.New("field 'arah' harus 'buy' atau 'sell'")
		}
		if err := positive("lot", s.Lot); err != nil {
			return s, err
		}
		if err := present("sl", s.SL); err != nil {
			return s, err
		}
		if err := present("tp", s.TP); err != nil {
			return s, err
		}
	case "ubah":
		if err := present("sl", s.SL); err != nil {
			return s, err
		}
		if err := present("tp", s.TP); err != nil {
			return s, err
		}
	case "kurang":
		if err := positive("lot", s.Lot); err != nil {
			return s, err
		}
		if s.LotLeft == nil || !finite(*s.LotLeft) || *s.LotLeft < 0 {
			return s, errors.New("lot kurang harus positif dan lot_sisa tidak boleh negatif")
		}
	case "tutup":
	default:
		return s, errors.New("field 'aksi' harus salah satu dari buka, ubah, kurang, tutup")
	}
	for name, value := range map[string]*float64{"lot": s.Lot, "harga": s.Price, "sl": s.SL, "tp": s.TP, "lot_sisa": s.LotLeft} {
		if value != nil && !finite(*value) {
			return s, fmt.Errorf("field '%s' harus berupa angka finite", name)
		}
	}
	if s.Symbol != nil && strings.TrimSpace(*s.Symbol) == "" {
		return s, errors.New("field 'simbol' harus berupa string yang tidak kosong")
	}
	return s, nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func present(name string, v *float64) error {
	if v == nil || !finite(*v) {
		return fmt.Errorf("field '%s' wajib berupa angka finite", name)
	}
	return nil
}
func positive(name string, v *float64) error {
	if err := present(name, v); err != nil {
		return err
	}
	if *v <= 0 {
		return fmt.Errorf("field '%s' harus lebih besar dari 0", name)
	}
	return nil
}

func Encode(s Signal) ([]byte, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if _, err := Parse(b); err != nil {
		return nil, err
	}
	return b, nil
}
