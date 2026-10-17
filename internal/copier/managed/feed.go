package managed

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type followerAccount struct {
	ID        int64
	Login     string
	Server    string
	Token     string
	CopyAfter int64
}

// Feed implements the local HTTP contract already used by MT5FollowerCopyEA.
// It binds to loopback and accepts credentials only for registered managed
// accounts, keeping order execution inside the MT5 terminal.
type Feed struct {
	Store           *SnapshotStore
	ReportsDir      string
	HeartbeatDir    string
	MasterHealthURL string
	mu              sync.RWMutex
	accounts        map[string]followerAccount
	heartbeatPaths  map[int64]string
	healthMu        sync.Mutex
	healthAt        time.Time
	healthActive    bool
	healthClient    *http.Client
}

func NewFeed(store *SnapshotStore, reportsDir, heartbeatDir, masterHealthURL string) (*Feed, error) {
	if store == nil || reportsDir == "" || heartbeatDir == "" || masterHealthURL == "" {
		return nil, errors.New("snapshot store, folder laporan, folder heartbeat, dan URL publisher master wajib tersedia")
	}
	return &Feed{
		Store: store, ReportsDir: reportsDir, HeartbeatDir: heartbeatDir, MasterHealthURL: masterHealthURL,
		accounts: map[string]followerAccount{}, heartbeatPaths: map[int64]string{}, healthClient: &http.Client{Timeout: time.Second},
	}, nil
}

// SetHeartbeatPath lets standalone followers preserve their configured local
// heartbeat location. Managed workers leave the shared runtime default intact.
func (f *Feed) SetHeartbeatPath(id int64, path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id > 0 && strings.TrimSpace(path) != "" {
		f.heartbeatPaths[id] = path
	}
}

func (f *Feed) Register(id int64, login, server, token string, copyAfter int64) error {
	if id <= 0 || strings.TrimSpace(login) == "" || strings.TrimSpace(server) == "" || len(token) < 32 {
		return errors.New("identitas managed follower tidak valid")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.accounts[token]; exists {
		return errors.New("token managed follower duplikat")
	}
	if copyAfter < 0 {
		return errors.New("waktu mulai managed follower tidak valid")
	}
	f.accounts[token] = followerAccount{ID: id, Login: login, Server: server, Token: token, CopyAfter: copyAfter}
	return nil
}

func (f *Feed) Unregister(id int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.heartbeatPaths, id)
	for token, account := range f.accounts {
		if account.ID == id {
			delete(f.accounts, token)
		}
	}
}

func (f *Feed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/mt5/follower/positions" && r.URL.Path != "/api/mt5/follower/report" {
		http.NotFound(w, r)
		return
	}
	token := r.Header.Get("X-Account-Token")
	f.mu.RLock()
	account, ok := f.accounts[token]
	f.mu.RUnlock()
	if !ok || r.Header.Get("X-MT5-Login") != account.Login || !strings.EqualFold(r.Header.Get("X-MT5-Server"), account.Server) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case r.URL.Path == "/api/mt5/follower/positions" && r.Method == http.MethodGet:
		if !f.masterActive() {
			cursor := f.readCursor(account.ID)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"active": false, "positions": []Position{}, "history_cursor": cursor.Ticket,
				"history_cursor_msc": cursor.TimeMSC, "copy_after": account.CopyAfter,
			})
			return
		}
		positions, err := f.Store.Sync(time.Now())
		if err != nil {
			http.Error(w, "signal snapshot unavailable", http.StatusServiceUnavailable)
			return
		}
		cursor := f.readCursor(account.ID)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"active": true, "positions": positions, "history_cursor": cursor.Ticket, "history_cursor_msc": cursor.TimeMSC,
			"copy_after": account.CopyAfter,
		})
	case r.URL.Path == "/api/mt5/follower/report" && r.Method == http.MethodPost:
		if err := f.writeReport(account.ID, r.Body); err != nil {
			http.Error(w, "report unavailable", http.StatusBadRequest)
			return
		}
		cursor := f.readCursor(account.ID)
		_ = json.NewEncoder(w).Encode(map[string]any{"history_cursor": cursor.Ticket, "history_cursor_msc": cursor.TimeMSC})
	default:
		w.Header().Set("Allow", allowedMethod(r.URL.Path))
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (f *Feed) masterActive() bool {
	f.healthMu.Lock()
	defer f.healthMu.Unlock()
	if time.Since(f.healthAt) < time.Second {
		return f.healthActive
	}
	f.healthAt = time.Now()
	request, err := http.NewRequest(http.MethodGet, f.MasterHealthURL, nil)
	if err != nil {
		f.healthActive = false
		return false
	}
	response, err := f.healthClient.Do(request)
	if err != nil {
		f.healthActive = false
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		f.healthActive = false
		return false
	}
	var result struct {
		Active       bool   `json:"active"`
		LastSnapshot string `json:"last_snapshot"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 8<<10)).Decode(&result) != nil || !result.Active {
		f.healthActive = false
		return false
	}
	lastSnapshot, err := time.Parse(time.RFC3339Nano, result.LastSnapshot)
	f.healthActive = err == nil && time.Since(lastSnapshot) >= -time.Second && time.Since(lastSnapshot) <= 15*time.Second
	return f.healthActive
}

type historyCursor struct {
	Ticket  string `json:"history_cursor"`
	TimeMSC int64  `json:"history_cursor_msc"`
}

func (f *Feed) readCursor(id int64) historyCursor {
	value := historyCursor{Ticket: "0"}
	data, err := os.ReadFile(filepath.Join(f.ReportsDir, fmt.Sprintf("%d.cursor.json", id)))
	if err != nil {
		return value
	}
	if json.Unmarshal(data, &value) != nil {
		return historyCursor{Ticket: "0"}
	}
	if value.Ticket == "" {
		value.Ticket = "0"
	}
	if value.TimeMSC < 0 {
		value.TimeMSC = 0
	}
	return value
}

func (f *Feed) writeReport(id int64, body io.Reader) error {
	var report map[string]any
	decoder := json.NewDecoder(io.LimitReader(body, 2<<20))
	decoder.UseNumber()
	if err := decoder.Decode(&report); err != nil || report == nil {
		return errors.New("laporan managed follower tidak valid")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("laporan managed follower berisi data tambahan")
	}
	data, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("laporan managed follower tidak dapat dienkode: %w", err)
	}
	if err := os.MkdirAll(f.ReportsDir, 0o700); err != nil {
		return err
	}
	target := filepath.Join(f.ReportsDir, fmt.Sprintf("%d.json", id))
	temporary := target + ".feed.tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	f.mu.RLock()
	heartbeat := f.heartbeatPaths[id]
	f.mu.RUnlock()
	if heartbeat == "" {
		heartbeat = filepath.Join(f.HeartbeatDir, fmt.Sprintf("heartbeat_managed_%d.txt", id))
	}
	if err := os.MkdirAll(f.HeartbeatDir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(heartbeat, []byte(time.Now().Format(time.RFC3339Nano)), 0o600); err != nil {
		return err
	}
	return nil
}

func allowedMethod(path string) string {
	if path == "/api/mt5/follower/positions" {
		return http.MethodGet
	}
	return http.MethodPost
}
