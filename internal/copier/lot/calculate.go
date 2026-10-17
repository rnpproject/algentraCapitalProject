// Package lot contains safe volume sizing helpers.
package lot

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
)

type SymbolInfo struct{ Step, Minimum, Maximum float64 }
type Config struct {
	Mode                                     string
	Fixed, Ratio, MaxPerOrder, MasterBalance float64
}
type Result struct {
	Volume *float64
	Reason string
}

func Calculate(master float64, symbol SymbolInfo, cfg Config, followerBalance, masterBalance *float64) Result {
	bad := func(s string) Result { return Result{Reason: s} }
	if !finite(master) || !finite(symbol.Step) || !finite(symbol.Minimum) || !finite(symbol.Maximum) || master <= 0 || symbol.Step <= 0 || symbol.Minimum <= 0 || symbol.Maximum < symbol.Minimum {
		return bad("parameter lot atau volume simbol tidak valid")
	}
	var raw float64
	switch cfg.Mode {
	case "", "rasio":
		ratio := cfg.Ratio
		if cfg.Mode == "" && ratio == 0 {
			ratio = 1
		}
		raw = master * ratio
	case "tetap":
		raw = cfg.Fixed
	case "saldo":
		mb := masterBalance
		if mb == nil && cfg.MasterBalance > 0 {
			v := cfg.MasterBalance
			mb = &v
		}
		if followerBalance == nil || mb == nil || !finite(*followerBalance) || !finite(*mb) || *mb <= 0 {
			return bad("saldo follower/master tidak tersedia untuk mode saldo")
		}
		raw = master * (*followerBalance / *mb)
	default:
		return bad(fmt.Sprintf("mode_lot tidak dikenal: %s", cfg.Mode))
	}
	if !finite(raw) || raw <= 0 {
		return bad("hasil lot tidak valid")
	}
	cap := cfg.MaxPerOrder
	if cap == 0 {
		cap = symbol.Maximum
	}
	if !finite(cap) || cap <= 0 {
		return bad("max_lot_per_order harus lebih besar dari 0")
	}
	decimal := func(value float64) *big.Rat {
		r, ok := new(big.Rat).SetString(strconv.FormatFloat(value, 'f', -1, 64))
		if !ok {
			return nil
		}
		return r
	}
	stepDecimal, rawDecimal := decimal(symbol.Step), decimal(raw)
	maximumDecimal, capDecimal := decimal(symbol.Maximum), decimal(cap)
	if stepDecimal == nil || rawDecimal == nil || maximumDecimal == nil || capDecimal == nil {
		return bad("parameter volume_step tidak valid")
	}
	bounded := rawDecimal
	if maximumDecimal.Cmp(bounded) < 0 {
		bounded = maximumDecimal
	}
	if capDecimal.Cmp(bounded) < 0 {
		bounded = capDecimal
	}
	quotient := new(big.Rat).Quo(bounded, stepDecimal)
	units := new(big.Int).Quo(quotient.Num(), quotient.Denom())
	stepUnits := new(big.Rat).Mul(new(big.Rat).SetInt(units), stepDecimal)
	volume, _ := stepUnits.Float64()
	if volume+symbol.Step*1e-9 < symbol.Minimum {
		return bad(fmt.Sprintf("hasil lot %g di bawah volume_min %g", volume, symbol.Minimum))
	}
	return Result{Volume: &volume, Reason: "ok"}
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
