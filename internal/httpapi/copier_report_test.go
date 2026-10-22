package httpapi

import (
	"math"
	"reflect"
	"testing"
)

func TestValidateFollowerReport(t *testing.T) {
	allow, terminal, expert := true, true, false
	payload := followerReport{
		Balance: 1200, Equity: 1210, FloatingProfit: 10, Margin: 100, FreeMargin: 1110,
		Currency: "USD", TradeMode: "demo", AllowLiveTrading: &allow,
		TerminalTradeAllowed: &terminal, ExpertTradeAllowed: &expert,
		OpenPositionIDs: []string{}, Deals: []followerDeal{{
			DealTicket: "9001", PositionID: "8001", TimeMSC: 1700000000000,
			Symbol: "XAUUSD.a", Action: "BUY", Entry: "IN", Volume: 0.1,
			Price: 2000, Profit: 0, Commission: -1, Swap: 0, Fee: 0,
		}},
	}
	if !validFollowerReport(payload) {
		t.Fatal("valid MT5 account report was rejected")
	}
	payload.Equity = math.NaN()
	if validFollowerReport(payload) {
		t.Fatal("non-finite account report was accepted")
	}
}

func TestCursorUsesNumericOrderAndPositionsAreSortedUnique(t *testing.T) {
	if got := maxDecimalString("999999999999999999", "1000000000000000000"); got != "1000000000000000000" {
		t.Fatalf("cursor = %s", got)
	}
	if got := maxDecimalString("100", "99"); got != "100" {
		t.Fatalf("cursor = %s", got)
	}
	if got := sortedUnique([]string{"3", "1", "3", "2"}); !reflect.DeepEqual(got, []string{"1", "2", "3"}) {
		t.Fatalf("positions = %#v", got)
	}
}
