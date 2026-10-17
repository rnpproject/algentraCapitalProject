// Package state reads and writes the JSON state shared by copier processes.
package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// State deliberately retains arbitrary JSON values in fields whose contents
// are owned by the follower, preserving compatibility with existing state files.
type State struct {
	Positions   map[string]any             `json:"positions"`
	SignalDate  any                        `json:"signal_date"`
	Offset      int64                      `json:"offset"`
	Processed   []any                      `json:"processed"`
	NetResidual map[string]any             `json:"netting_residuals,omitempty"`
	Extra       map[string]json.RawMessage `json:"-"`
}

func Empty() State {
	return State{Positions: map[string]any{}, SignalDate: nil, Offset: 0, Processed: []any{}, Extra: map[string]json.RawMessage{}}
}

func Load(path string) (State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Empty(), nil
	}
	if err != nil {
		return State{}, fmt.Errorf("state %s tidak dapat dibaca: %w", path, err)
	}
	var result map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&result); err != nil {
		return State{}, fmt.Errorf("state %s tidak dapat dibaca: %w", path, err)
	}
	if result == nil {
		return State{}, fmt.Errorf("state %s harus berupa object JSON", path)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return State{}, fmt.Errorf("state %s berisi data setelah object JSON", path)
		}
		return State{}, fmt.Errorf("state %s tidak dapat dibaca: %w", path, err)
	}
	loaded := Empty()
	if value, ok := result["positions"]; ok {
		if err := json.Unmarshal(value, &loaded.Positions); err != nil || loaded.Positions == nil {
			return State{}, fmt.Errorf("format state %s tidak valid: positions harus object", path)
		}
	}
	if value, ok := result["signal_date"]; ok {
		if err := json.Unmarshal(value, &loaded.SignalDate); err != nil {
			return State{}, fmt.Errorf("format state %s tidak valid: signal_date", path)
		}
	}
	if value, ok := result["offset"]; ok {
		if err := json.Unmarshal(value, &loaded.Offset); err != nil || loaded.Offset < 0 {
			return State{}, fmt.Errorf("format state %s tidak valid: offset", path)
		}
	}
	if value, ok := result["processed"]; ok {
		if err := json.Unmarshal(value, &loaded.Processed); err != nil || loaded.Processed == nil {
			return State{}, fmt.Errorf("format state %s tidak valid: processed harus array", path)
		}
	}
	if value, ok := result["netting_residuals"]; ok {
		if err := json.Unmarshal(value, &loaded.NetResidual); err != nil || loaded.NetResidual == nil {
			return State{}, fmt.Errorf("format state %s tidak valid: netting_residuals harus object", path)
		}
	}
	known := map[string]struct{}{"positions": {}, "signal_date": {}, "offset": {}, "processed": {}, "netting_residuals": {}}
	for key, value := range result {
		if _, ok := known[key]; !ok {
			loaded.Extra[key] = append(json.RawMessage(nil), value...)
		}
	}
	return loaded, nil
}

func Save(path string, value State) error {
	if value.Positions == nil || value.Processed == nil || value.Offset < 0 {
		return errors.New("format state tidak valid")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("state tidak dapat dienkode: %w", err)
	}
	if len(value.Extra) > 0 {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return fmt.Errorf("state tidak dapat dienkode: %w", err)
		}
		for key, raw := range value.Extra {
			if _, reserved := fields[key]; !reserved {
				fields[key] = append(json.RawMessage(nil), raw...)
			}
		}
		data, err = json.Marshal(fields)
		if err != nil {
			return fmt.Errorf("state tidak dapat dienkode: %w", err)
		}
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("folder state tidak dapat dibuat: %w", err)
	}
	file, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("file sementara state tidak dapat dibuat: %w", err)
	}
	temp := file.Name()
	defer os.Remove(temp)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("state tidak dapat ditulis: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("state tidak dapat disinkronkan: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("file state tidak dapat ditutup: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		return fmt.Errorf("state tidak dapat diganti secara atomik: %w", err)
	}
	return nil
}
