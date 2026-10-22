// Package symbols handles broker symbol aliases and terminal availability.
package symbols

import (
	"fmt"
)

func Map(masterSymbol string, aliases map[string]string) string {
	if symbol, ok := aliases[masterSymbol]; ok {
		return symbol
	}
	return masterSymbol
}

// Terminal defines the subset of an MT5 adapter needed for symbol selection.
type Terminal interface {
	SymbolInfo(symbol string) (SymbolInfo, bool)
	SymbolSelect(symbol string, selected bool) bool
	LastError() error
}

type SymbolInfo struct {
	Visible bool
}

func Select(terminal Terminal, symbol string) error {
	info, ok := terminal.SymbolInfo(symbol)
	if !ok {
		return fmt.Errorf("simbol %q tidak ditemukan di terminal follower", symbol)
	}
	if !info.Visible && !terminal.SymbolSelect(symbol, true) {
		return fmt.Errorf("gagal menampilkan simbol %q di Market Watch: %v", symbol, terminal.LastError())
	}
	return nil
}
