package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/rnpproject/algentra-backend/internal/copier/master"
	"github.com/rnpproject/algentra-backend/internal/copier/signalio"
)

type config struct {
	SignalDirectory string   `json:"folder_sinyal"`
	FilterMagic     []int64  `json:"filter_magic"`
	FilterSymbols   []string `json:"filter_simbol"`
	IgnoreComments  []string `json:"abaikan_komentar_mengandung"`
	ListenAddress   string   `json:"listen_addr"`
}

type snapshotRequest struct {
	Positions *[]master.Position `json:"positions"`
}

type publisher struct {
	mu           sync.Mutex
	filter       master.Filter
	previous     master.Snapshot
	initialized  bool
	lastSnapshot time.Time
	ids          master.IDGenerator
	output       string
}

func main() {
	flag.Usage = func() { fmt.Fprintln(flag.CommandLine.Output(), "Usage: copier-master [config.json]") }
	flag.Parse()
	path := "apps/copier-worker/config/master.json"
	if flag.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "copier-master hanya menerima satu path konfigurasi")
		os.Exit(2)
	}
	if flag.NArg() == 1 {
		path = flag.Arg(0)
	}
	if err := run(path); err != nil {
		log.Fatal(err)
	}
}

func run(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("baca konfigurasi master: %w", err)
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("konfigurasi master bukan JSON valid: %w", err)
	}
	if strings.TrimSpace(cfg.SignalDirectory) == "" {
		return errors.New("folder_sinyal wajib diisi")
	}
	if cfg.ListenAddress == "" {
		cfg.ListenAddress = "127.0.0.1:8765"
	}
	host, _, err := net.SplitHostPort(cfg.ListenAddress)
	if err != nil {
		return errors.New("listen_addr harus berupa alamat host:port")
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return errors.New("copier-master hanya menerima koneksi loopback")
	}
	key := os.Getenv("MASTER_PUBLISHER_TOKEN")
	if len(key) < 32 {
		return errors.New("MASTER_PUBLISHER_TOKEN wajib berisi secret minimal 32 karakter")
	}
	magic := make(map[int64]struct{}, len(cfg.FilterMagic))
	for _, value := range cfg.FilterMagic {
		magic[value] = struct{}{}
	}
	symbols := make(map[string]struct{}, len(cfg.FilterSymbols))
	for _, value := range cfg.FilterSymbols {
		symbols[value] = struct{}{}
	}
	p := &publisher{filter: master.Filter{Magic: magic, Symbols: symbols, IgnoreWords: cfg.IgnoreComments}, output: cfg.SignalDirectory}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		p.mu.Lock()
		active := p.initialized && time.Since(p.lastSnapshot) <= 15*time.Second
		lastSnapshot := p.lastSnapshot
		p.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "active": active, "last_snapshot": lastSnapshot.UTC().Format(time.RFC3339Nano)})
	})
	mux.HandleFunc("POST /snapshot", func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if len(provided) != len(key) || subtle.ConstantTimeCompare([]byte(provided), []byte(key)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"detail": "Unauthorized"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		var payload snapshotRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&payload); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"detail": "Snapshot tidak valid"})
			return
		}
		if payload.Positions == nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"detail": "Snapshot wajib berisi array positions"})
			return
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"detail": "Snapshot harus satu object JSON"})
			return
		}
		if err := p.accept(*payload.Positions); err != nil {
			log.Printf("gagal memproses snapshot master: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": "Snapshot gagal disimpan"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "accepted", "positions": len(*payload.Positions)})
	})
	server := &http.Server{Addr: cfg.ListenAddress, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("publisher master Go aktif pada %s; sinyal: %s", cfg.ListenAddress, cfg.SignalDirectory)
	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		return fmt.Errorf("publisher tidak dapat membuka listener: %w", err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	defer signal.Stop(stop)
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}
}

func (p *publisher) accept(positions []master.Position) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	current, err := master.BuildSnapshot(positions, p.filter)
	if err != nil {
		return err
	}
	if !p.initialized {
		p.previous, p.initialized = current, true
		p.lastSnapshot = time.Now()
		log.Printf("baseline master berisi %d posisi", len(current))
		return nil
	}
	events := master.Stamp(master.Diff(p.previous, current, 0), &p.ids, time.Now())
	for _, event := range events {
		if _, err := signalio.Append(p.output, event, time.Now()); err != nil {
			return err
		}
		log.Printf("sinyal %s aksi=%s diterbitkan", event.ID, event.Action)
	}
	p.previous = current
	p.lastSnapshot = time.Now()
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
