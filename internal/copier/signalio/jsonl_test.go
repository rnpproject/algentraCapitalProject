package signalio

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rnpproject/algentra-backend/internal/copier/protocol"
)

func TestReadNewLeavesTrailingPartialLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signals.jsonl")
	first := `{"v":1,"id":"a","ts":1,"aksi":"tutup","pos_id":3,"komentar":""}` + "\n"
	if err := os.WriteFile(path, []byte(first+`{"v":1`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadNew(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 1 || !got.HasPartialLine || got.Offset != int64(len(first)) {
		t.Fatalf("ReadNew() = %#v", got)
	}
}

func TestAppendCreatesDailyJSONL(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	path, err := Append(dir, protocol.Signal{Version: 1, ID: "x", TS: 1, Action: "tutup", PosID: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "sinyal_20260102.jsonl" {
		t.Fatalf("unexpected file path %s", path)
	}
	got, err := ReadNew(path, 0)
	if err != nil || len(got.Records) != 1 {
		t.Fatalf("ReadNew() = %#v, %v", got, err)
	}
}
