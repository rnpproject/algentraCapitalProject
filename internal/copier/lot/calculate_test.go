package lot

import "testing"

func TestCalculateFloorsToBrokerStepAndHonorsCap(t *testing.T) {
	got := Calculate(0.37, SymbolInfo{Step: 0.1, Minimum: 0.1, Maximum: 2}, Config{Mode: "rasio", Ratio: 1, MaxPerOrder: 0.3}, nil, nil)
	if got.Volume == nil || *got.Volume != 0.3 {
		t.Fatalf("Calculate() = %#v", got)
	}
}

func TestCalculateSkipsBelowMinimum(t *testing.T) {
	got := Calculate(0.04, SymbolInfo{Step: 0.01, Minimum: 0.1, Maximum: 2}, Config{Mode: "rasio", Ratio: 1}, nil, nil)
	if got.Volume != nil {
		t.Fatalf("expected skip, got %#v", got)
	}
}

func TestCalculateBalanceRatio(t *testing.T) {
	follower, master := 500.0, 1000.0
	got := Calculate(0.2, SymbolInfo{Step: 0.01, Minimum: 0.01, Maximum: 2}, Config{Mode: "saldo"}, &follower, &master)
	if got.Volume == nil || *got.Volume != 0.1 {
		t.Fatalf("Calculate() = %#v", got)
	}
}
