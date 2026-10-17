// Package signalio implements the append-only daily JSONL transport.
package signalio

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rnpproject/algentra-backend/internal/copier/protocol"
)

type Record struct {
	Signal    protocol.Signal
	EndOffset int64
}
type ReadResult struct {
	Records        []Record
	Offset         int64
	HasPartialLine bool
}

func Filename(t time.Time) string {
	return fmt.Sprintf("sinyal_%s.jsonl", t.Local().Format("20060102"))
}
func Path(folder string, t time.Time) string { return filepath.Join(folder, Filename(t)) }
func Append(folder string, s protocol.Signal, now time.Time) (string, error) {
	if err := os.MkdirAll(folder, 0750); err != nil {
		return "", err
	}
	b, err := protocol.Encode(s)
	if err != nil {
		return "", err
	}
	path := Path(folder, now)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err = f.Write(append(b, '\n')); err != nil {
		return "", err
	}
	return path, nil
}
func ReadNew(path string, offset int64) (ReadResult, error) {
	out := ReadResult{Records: []Record{}, Offset: offset}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return out, err
	}
	start := offset
	if start < 0 {
		start = 0
	}
	if start > info.Size() {
		log.Printf("file sinyal %s lebih pendek dari offset tersimpan; membaca ulang dari awal", path)
		start = 0
	}
	if _, err = f.Seek(start, 0); err != nil {
		return out, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return out, err
	}
	cursor := start
	for len(data) > 0 {
		lineEnd := bytes.IndexAny(data, "\r\n")
		if lineEnd < 0 {
			out.HasPartialLine = true
			break
		}
		lineSize := lineEnd + 1
		if data[lineEnd] == '\r' && lineSize < len(data) && data[lineSize] == '\n' {
			lineSize++
		}
		line := data[:lineSize]
		data = data[lineSize:]
		cursor += int64(len(line))
		trimmed := strings.TrimSpace(string(line))
		if trimmed != "" {
			s, parseErr := protocol.Parse([]byte(trimmed))
			if parseErr == nil {
				out.Records = append(out.Records, Record{Signal: s, EndOffset: cursor})
			} else {
				log.Printf("baris sinyal rusak di %s byte %d: %v", path, cursor-int64(len(line)), parseErr)
			}
		}
	}
	out.Offset = cursor
	return out, nil
}
