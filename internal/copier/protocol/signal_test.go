package protocol

import "testing"

func TestParseAcceptsOpenSignalAndRejectsInvalid(t *testing.T) {
	s, err := Parse([]byte(`{"v":1,"id":"s-1","ts":1.5,"aksi":"buka","pos_id":7,"simbol":"EURUSD","arah":"buy","lot":0.1,"sl":0,"tp":0,"komentar":""}`))
	if err != nil || s.PosID != 7 {
		t.Fatalf("Parse() = %#v, %v", s, err)
	}
	if _, err := Parse([]byte(`{"v":1,"id":"s-1","ts":1,"aksi":"buka","pos_id":7,"simbol":"EURUSD","arah":"buy","lot":0,"sl":0,"tp":0}`)); err == nil {
		t.Fatal("expected zero lot to be rejected")
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	s := Signal{Version: 1, ID: "s-2", TS: 2, Action: "tutup", PosID: 9, Comment: ""}
	b, err := Encode(s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(b)
	if err != nil || got.ID != s.ID {
		t.Fatalf("round trip = %#v, %v", got, err)
	}
	if len(b) == 0 {
		t.Fatal("empty encoding")
	}
}
