// Command copier-launcher supervises the standalone MT5 master and follower
// terminals. Go publishes the master snapshot and serves the local feed; the
// MT5 follower EA consumes that feed and executes the configured copy rules.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/rnpproject/algentra-backend/internal/copier/managed"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var backoff = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second}

type launcherConfig struct {
	MasterConfig     string   `json:"master_config"`
	Followers        []string `json:"followers"`
	CheckSeconds     int      `json:"check_interval_seconds"`
	HeartbeatTimeout int      `json:"heartbeat_timeout_seconds"`
	MaxRestarts      int      `json:"max_restarts_per_10_minutes"`
	RuntimeDir       string   `json:"runtime_dir"`
	FeedAddress      string   `json:"feed_address"`
}
type masterConfig struct {
	TerminalPath  string `json:"terminal_path"`
	SignalDir     string `json:"folder_sinyal"`
	ListenAddress string `json:"listen_addr"`
}
type followerConfig struct {
	Name              string            `json:"nama"`
	TerminalPath      string            `json:"terminal_path"`
	Login             json.Number       `json:"login"`
	PasswordEnv       string            `json:"password_env"`
	Server            string            `json:"server"`
	Portable          bool              `json:"portable"`
	SignalDir         string            `json:"folder_sinyal"`
	SymbolMap         map[string]string `json:"map_simbol"`
	LotMode           string            `json:"mode_lot"`
	FixedLot          float64           `json:"lot_tetap"`
	LotRatio          *float64          `json:"rasio_lot"`
	MasterBalance     float64           `json:"saldo_master"`
	PollingMS         int               `json:"polling_ms"`
	Magic             int64             `json:"magic"`
	Slippage          int               `json:"deviation_poin"`
	SymbolSuffix      string            `json:"symbol_suffix"`
	MaxEntryDeviation float64           `json:"max_entry_deviation_percent"`
	MaxSignalAge      *float64          `json:"max_umur_sinyal_buka_detik"`
	MaxLotPerOrder    float64           `json:"max_lot_per_order"`
	MaxPositions      int               `json:"max_posisi_terbuka"`
	MaxLotTotal       float64           `json:"max_lot_total"`
	DryRun            *bool             `json:"dry_run"`
	Reverse           bool              `json:"arah_terbalik"`
	Active            *bool             `json:"aktif"`
	HeartbeatFolder   string            `json:"heartbeat_folder"`
}
type child struct {
	name          string
	command       []string
	dir           string
	env           []string
	heartbeat     string
	cmd           *exec.Cmd
	started       time.Time
	next          time.Time
	restarts      []time.Time
	permanent     bool
	exit          chan error
	failureReason string
	startupPath   string
	startupData   []byte
}
type supervisor struct {
	root            string
	config          launcherConfig
	children        []*child
	mu              sync.Mutex
	stopping        bool
	feed            *managed.Feed
	feedServer      *http.Server
	feedDone        chan error
	runtimeDir      string
	masterHealthURL string
}

func main() {
	configArg := flag.String("config", "config/launcher.json", "path konfigurasi launcher")
	flag.Parse()
	if err := run(*configArg); err != nil {
		log.Printf("launcher berhenti: %v", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	abs, err := filepath.Abs(configPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("baca config launcher: %w", err)
	}
	var cfg launcherConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("config launcher tidak valid: %w", err)
	}
	if cfg.MasterConfig == "" || cfg.Followers == nil {
		return errors.New("config launcher wajib berisi master_config dan daftar followers")
	}
	root := findRoot(filepath.Dir(abs))
	if cfg.CheckSeconds < 1 {
		cfg.CheckSeconds = 5
	}
	if cfg.HeartbeatTimeout < 5 {
		cfg.HeartbeatTimeout = 30
	}
	if cfg.MaxRestarts < 1 {
		cfg.MaxRestarts = 10
	}
	s := &supervisor{root: root, config: cfg}
	if err := s.configure(filepath.Dir(abs)); err != nil {
		return err
	}
	if err := s.startFeed(); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s.startAll()
	ticker := time.NewTicker(time.Duration(cfg.CheckSeconds) * time.Second)
	defer ticker.Stop()
	status := time.NewTicker(time.Minute)
	defer status.Stop()
	for {
		select {
		case <-ctx.Done():
			s.stopAll()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.feedServer.Shutdown(shutdownCtx)
			cancel()
			return nil
		case err := <-s.feedDone:
			s.stopAll()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("feed follower berhenti: %w", err)
			}
			return errors.New("feed follower berhenti tanpa permintaan shutdown")
		case <-ticker.C:
			s.tick()
		case <-status.C:
			s.logStatus()
		}
	}
}

func findRoot(from string) string {
	for dir := from; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return from
		}
	}
}
func resolve(base, value string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(base, filepath.FromSlash(value))
}
func readObject(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s: JSON tidak valid", path)
	}
	return nil
}

func (s *supervisor) configure(configDir string) error {
	masterPath := resolve(configDir, s.config.MasterConfig)
	var master masterConfig
	if err := readObject(masterPath, &master); err != nil {
		return fmt.Errorf("baca config master: %w", err)
	}
	if master.TerminalPath == "" || master.SignalDir == "" {
		return errors.New("config master wajib memiliki terminal_path dan folder_sinyal")
	}
	if master.ListenAddress == "" {
		master.ListenAddress = "127.0.0.1:8765"
	}
	host, _, err := net.SplitHostPort(master.ListenAddress)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("listen_addr pada master_config wajib menggunakan IP loopback")
	}
	s.masterHealthURL = "http://" + master.ListenAddress + "/health"
	s.runtimeDir = s.config.RuntimeDir
	if s.runtimeDir == "" {
		s.runtimeDir = filepath.Join(configDir, "runtime-standalone")
	} else {
		s.runtimeDir = resolve(configDir, s.runtimeDir)
	}
	if s.config.FeedAddress == "" {
		s.config.FeedAddress = "127.0.0.1:8766"
	}
	feedHost, _, feedErr := net.SplitHostPort(s.config.FeedAddress)
	if feedErr != nil || net.ParseIP(feedHost) == nil || !net.ParseIP(feedHost).IsLoopback() {
		return errors.New("feed_address wajib menggunakan IP loopback")
	}
	signalDirectory := resolve(s.root, master.SignalDir)
	store, err := managed.OpenSnapshotStore(signalDirectory, filepath.Join(s.runtimeDir, "states", "master-snapshot.json"), 100*365*24*time.Hour)
	if err != nil {
		return fmt.Errorf("inisialisasi snapshot Go: %w", err)
	}
	s.feed, err = managed.NewFeed(store, filepath.Join(s.runtimeDir, "reports"), filepath.Join(s.runtimeDir, "heartbeats"), s.masterHealthURL)
	if err != nil {
		return err
	}
	token := os.Getenv("MASTER_PUBLISHER_TOKEN")
	if len(token) < 32 {
		return errors.New("MASTER_PUBLISHER_TOKEN wajib tersedia minimal 32 karakter")
	}
	publisher := filepath.Join(s.root, "algentra-copier-master.exe")
	if runtime.GOOS != "windows" {
		publisher = filepath.Join(s.root, "algentra-copier-master")
	}
	if _, err := os.Stat(publisher); err != nil {
		return fmt.Errorf("binary publisher master Go tidak ditemukan: %s", publisher)
	}
	env := filteredEnv("MASTER_PUBLISHER_TOKEN", token)
	s.children = append(s.children, &child{name: "master", command: []string{publisher, masterPath}, dir: s.root, env: env})
	masterTerminal := resolve(s.root, master.TerminalPath)
	terminalPaths := map[string]string{normalized(masterTerminal): "master"}
	logins := map[string]string{}
	names := map[string]bool{"master": true}
	for _, item := range s.config.Followers {
		path := resolve(configDir, item)
		var f followerConfig
		if err := readObject(path, &f); err != nil {
			return fmt.Errorf("baca config follower %s: %w", path, err)
		}
		if f.Active != nil && !*f.Active {
			continue
		}
		if strings.TrimSpace(f.Name) == "" || f.TerminalPath == "" || f.Login == "" || f.PasswordEnv == "" || f.Server == "" || f.SignalDir == "" {
			return fmt.Errorf("config follower %s wajib memiliki nama, terminal_path, login, password_env, server, dan folder_sinyal", path)
		}
		if names[f.Name] {
			return fmt.Errorf("nama proses follower duplikat: %s", f.Name)
		}
		names[f.Name] = true
		f.TerminalPath = resolve(s.root, f.TerminalPath)
		term := normalized(f.TerminalPath)
		if owner, ok := terminalPaths[term]; ok {
			return fmt.Errorf("terminal_path dipakai oleh %s dan %s", owner, f.TerminalPath)
		}
		terminalPaths[term] = f.Name
		login := f.Login.String()
		if owner, ok := logins[login]; ok {
			return fmt.Errorf("akun follower login %s dipakai oleh %s dan %s", login, owner, f.Name)
		}
		logins[login] = f.Name
		accountID, parseErr := strconv.ParseInt(login, 10, 64)
		if parseErr != nil || accountID <= 0 {
			return fmt.Errorf("config follower %s: login harus angka positif", path)
		}
		if f.SignalDir != "" && normalized(resolve(s.root, f.SignalDir)) != normalized(signalDirectory) {
			return fmt.Errorf("folder_sinyal follower %s harus sama dengan publisher master", f.Name)
		}
		if password := os.Getenv(f.PasswordEnv); password == "" {
			return fmt.Errorf("environment variable password %s belum diatur untuk follower %s", f.PasswordEnv, f.Name)
		}
		if f.LotMode == "" {
			f.LotMode = "rasio"
		}
		if f.LotRatio == nil {
			value := 1.0
			f.LotRatio = &value
		}
		if f.PollingMS <= 0 {
			f.PollingMS = 20
		}
		if f.PollingMS < 20 {
			f.PollingMS = 20
		}
		if f.PollingMS > 60000 {
			f.PollingMS = 60000
		}
		if f.Magic <= 0 {
			f.Magic = 880001
		}
		if f.Slippage <= 0 {
			f.Slippage = 20
		}
		if f.MaxSignalAge == nil {
			value := 3.0
			f.MaxSignalAge = &value
		}
		if !finite(f.MaxLotPerOrder) || !finite(f.MaxLotTotal) || !finite(f.MaxEntryDeviation) || !finite(*f.MaxSignalAge) || f.MaxEntryDeviation < 0 || *f.MaxSignalAge < 0 {
			return fmt.Errorf("batas risiko dan usia sinyal follower %s harus berupa angka terbatas dan tidak negatif", f.Name)
		}
		if f.MaxLotPerOrder <= 0 || f.MaxLotTotal <= 0 || f.MaxPositions <= 0 {
			return fmt.Errorf("follower %s wajib memiliki batas max_lot_per_order, max_lot_total, dan max_posisi_terbuka positif", f.Name)
		}
		if f.LotMode != "rasio" && f.LotMode != "tetap" && f.LotMode != "saldo" {
			return fmt.Errorf("mode_lot follower %s tidak valid", f.Name)
		}
		if f.LotMode == "rasio" && (!finite(*f.LotRatio) || *f.LotRatio <= 0) || f.LotMode == "tetap" && (!finite(f.FixedLot) || f.FixedLot <= 0) || f.LotMode == "saldo" && (!finite(f.MasterBalance) || f.MasterBalance <= 0) {
			return fmt.Errorf("konfigurasi lot follower %s tidak valid", f.Name)
		}
		if strings.ContainsAny(f.SymbolSuffix, "\r\n|,=") {
			return fmt.Errorf("symbol_suffix follower %s tidak valid", f.Name)
		}
		if f.DryRun == nil {
			value := true
			f.DryRun = &value
		}
		terminalDir := filepath.Dir(f.TerminalPath)
		if _, err := os.Stat(f.TerminalPath); err != nil {
			return fmt.Errorf("terminal follower tidak ditemukan untuk %s: %w", f.Name, err)
		}
		if len(f.SymbolMap) > 0 {
			for source, target := range f.SymbolMap {
				if strings.TrimSpace(source) == "" || strings.TrimSpace(target) == "" || strings.ContainsAny(source+target, "\r\n,=|") {
					return fmt.Errorf("map_simbol follower %s tidak valid", f.Name)
				}
			}
		}
		expertBinary := filepath.Join(s.root, "mt5", "experts", "MT5FollowerCopyEA.ex5")
		if _, err := os.Stat(expertBinary); err != nil {
			return fmt.Errorf("binary MT5FollowerCopyEA.ex5 belum tersedia; compile mt5/experts/MT5FollowerCopyEA.mq5: %w", err)
		}
		expertTarget := filepath.Join(terminalDir, "MQL5", "Experts", "MT5FollowerCopyEA.ex5")
		expertData, err := os.ReadFile(expertBinary)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(expertTarget), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(expertTarget, expertData, 0o600); err != nil {
			return err
		}
		token, err := newToken()
		if err != nil {
			return err
		}
		if err := s.feed.Register(accountID, login, f.Server, token, time.Now().Unix()); err != nil {
			return err
		}
		preset, err := s.makePreset(f, token, terminalDir)
		if err != nil {
			return err
		}
		startup := filepath.Join(terminalDir, "algentra-standalone-startup.ini")
		startupData, err := s.startupData(f, filepath.Base(preset))
		if err != nil {
			return err
		}
		heartbeatDir := f.HeartbeatFolder
		if heartbeatDir == "" {
			heartbeatDir = s.root
		} else if !filepath.IsAbs(heartbeatDir) {
			heartbeatDir = resolve(s.root, heartbeatDir)
		}
		heartbeatPath := filepath.Join(heartbeatDir, "heartbeat_"+f.Name+".txt")
		s.feed.SetHeartbeatPath(accountID, heartbeatPath)
		args := []string{"/config:" + startup}
		if f.Portable {
			args = append(args, "/portable")
		}
		followerEnv := filteredAllEnv(f.PasswordEnv)
		s.children = append(s.children, &child{name: f.Name, command: append([]string{f.TerminalPath}, args...), dir: terminalDir, env: followerEnv, heartbeat: heartbeatPath, startupPath: startup, startupData: startupData})
	}
	if len(s.children) == 0 {
		return errors.New("tidak ada proses yang dapat dijalankan")
	}
	return nil
}
func normalized(path string) string {
	abs, _ := filepath.Abs(path)
	return strings.ToLower(filepath.Clean(abs))
}
func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func (s *supervisor) startFeed() error {
	if err := os.MkdirAll(s.runtimeDir, 0o700); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", s.config.FeedAddress)
	if err != nil {
		return fmt.Errorf("feed Go tidak dapat listen pada %s: %w", s.config.FeedAddress, err)
	}
	s.feedServer = &http.Server{Handler: s.feed, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	s.feedDone = make(chan error, 1)
	go func() {
		s.feedDone <- s.feedServer.Serve(listener)
	}()
	log.Printf("feed standalone Go aktif pada %s", s.config.FeedAddress)
	return nil
}

func (s *supervisor) makePreset(f followerConfig, token, terminalDir string) (string, error) {
	values := []string{"ServerURL=http://" + s.config.FeedAddress, "AccountToken=" + token, fmt.Sprintf("PollIntervalMs=%d", f.PollingMS), fmt.Sprintf("MagicNumber=%d", f.Magic), "VolumeMultiplier=1.0", fmt.Sprintf("MaxEntryDeviationPercent=%.6f", f.MaxEntryDeviation), fmt.Sprintf("MaxSignalAgeSeconds=%.3f", *f.MaxSignalAge), fmt.Sprintf("MaxSlippagePoints=%d", f.Slippage), "ManagedMode=true", "DryRun=" + strconv.FormatBool(*f.DryRun), "LotMode=" + f.LotMode, "SymbolSuffix=" + f.SymbolSuffix,
		fmt.Sprintf("FixedLot=%.8f", f.FixedLot), fmt.Sprintf("RatioLot=%.8f", *f.LotRatio), fmt.Sprintf("MasterBalance=%.8f", f.MasterBalance), fmt.Sprintf("MaxLotPerOrder=%.8f", f.MaxLotPerOrder), fmt.Sprintf("MaxLotTotal=%.8f", f.MaxLotTotal), fmt.Sprintf("MaxOpenPositions=%d", f.MaxPositions), "ReverseDirection=" + strconv.FormatBool(f.Reverse)}
	if len(f.SymbolMap) > 0 {
		keys := make([]string, 0, len(f.SymbolMap))
		for k := range f.SymbolMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, k+"="+f.SymbolMap[k])
		}
		values = append(values, "SymbolMapCsv="+strings.Join(pairs, ","))
	}
	path := filepath.Join(terminalDir, "MQL5", "Presets", "algentra-standalone.set")
	if err := writeAtomic(path, []byte(strings.Join(values, "\r\n")+"\r\n")); err != nil {
		return "", err
	}
	return path, nil
}

func (s *supervisor) startupData(f followerConfig, presetName string) ([]byte, error) {
	password := os.Getenv(f.PasswordEnv)
	for _, value := range []string{f.Server, password} {
		if strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("server atau password follower %s mengandung baris baru", f.Name)
		}
	}
	content := fmt.Sprintf("[Common]\r\nLogin=%s\r\nServer=%s\r\nPassword=%s\r\nKeepPrivate=1\r\nNewsEnable=0\r\n\r\n[Experts]\r\nEnabled=1\r\nAllowLiveTrading=1\r\nAllowDllImport=0\r\nAccount=0\r\nProfile=0\r\n\r\n[StartUp]\r\nExpert=MT5FollowerCopyEA\r\nExpertParameters=%s\r\nSymbol=EURUSD\r\nPeriod=M1\r\n", f.Login.String(), f.Server, password, presetName)
	return []byte(content), nil
}

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func filteredAllEnv(allowedPasswordKey string) []string {
	out := make([]string, 0)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		upper := strings.ToUpper(key)
		if strings.EqualFold(key, allowedPasswordKey) || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "PASS_") || strings.Contains(upper, "SECRET") || strings.Contains(upper, "TOKEN") || strings.Contains(upper, "API_KEY") || strings.Contains(upper, "APIKEY") {
			continue
		}
		out = append(out, item)
	}
	return out
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
func filteredEnv(extraKey, extraValue string) []string {
	out := make([]string, 0)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		up := strings.ToUpper(key)
		if strings.Contains(up, "PASSWORD") || strings.Contains(up, "PASS_") || strings.Contains(up, "SECRET") || strings.Contains(up, "TOKEN") || strings.Contains(up, "API_KEY") || strings.Contains(up, "APIKEY") {
			continue
		}
		if strings.EqualFold(key, "PATH") || strings.EqualFold(key, "SYSTEMROOT") || strings.EqualFold(key, "WINDIR") || strings.EqualFold(key, "TEMP") || strings.EqualFold(key, "TMP") {
			out = append(out, item)
		}
	}
	return append(out, extraKey+"="+extraValue)
}

func (s *supervisor) startAll() {
	for _, c := range s.children {
		s.start(c)
	}
}
func (s *supervisor) start(c *child) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping || c.permanent || c.cmd != nil {
		return
	}
	cmd := exec.Command(c.command[0], c.command[1:]...)
	cmd.Dir = c.dir
	cmd.Env = c.env
	if c.startupPath != "" {
		if err := writeAtomic(c.startupPath, c.startupData); err != nil {
			log.Printf("startup config %s gagal ditulis: %v", c.name, err)
			c.next = time.Now().Add(time.Second)
			return
		}
		if c.heartbeat != "" {
			_ = os.Remove(c.heartbeat)
		}
	}
	logPath := filepath.Join(s.root, "apps", "copier-worker", "logs", c.name+".log")
	if c.name == "master" {
		logPath = filepath.Join(s.root, "apps", "copier-worker", "logs", "master-publisher.log")
	}
	_ = os.MkdirAll(filepath.Dir(logPath), 0o700)
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("buka log %s gagal: %v", c.name, err)
		c.next = time.Now().Add(time.Second)
		return
	}
	cmd.Stdout = io.MultiWriter(os.Stdout, file)
	cmd.Stderr = io.MultiWriter(os.Stderr, file)
	if err = cmd.Start(); err != nil {
		file.Close()
		log.Printf("proses %s gagal mulai: %v", c.name, err)
		c.next = time.Now().Add(time.Second)
		return
	}
	c.cmd = cmd
	c.started = time.Now()
	c.exit = make(chan error, 1)
	go func() {
		err := cmd.Wait()
		_ = file.Close()
		c.exit <- err
		log.Printf("proses %s berhenti: %v", c.name, err)
	}()
	if c.startupPath != "" {
		go func(path string) {
			timer := time.NewTimer(3 * time.Second)
			defer timer.Stop()
			<-timer.C
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				log.Printf("startup config %s tidak dapat dibersihkan: %v", c.name, err)
			}
		}(c.startupPath)
	}
	log.Printf("proses %s dimulai pid=%d", c.name, cmd.Process.Pid)
}
func (s *supervisor) tick() {
	now := time.Now()
	for _, c := range s.children {
		s.mu.Lock()
		running := c.cmd != nil
		next := c.next
		exit := c.exit
		s.mu.Unlock()
		if running {
			select {
			case err := <-exit:
				s.mu.Lock()
				c.cmd = nil
				c.exit = nil
				reason := c.failureReason
				c.failureReason = ""
				s.mu.Unlock()
				if reason == "" {
					reason = "exit code 0"
					if err != nil {
						reason = err.Error()
					}
				}
				s.schedule(c, reason, now)
				continue
			default:
			}
		}
		if !running {
			if !c.permanent && !now.Before(next) {
				s.start(c)
			}
			continue
		}
		if c.heartbeat != "" && now.Sub(c.started) > time.Duration(s.config.HeartbeatTimeout)*time.Second {
			info, err := os.Stat(c.heartbeat)
			if err != nil || time.Since(info.ModTime()) > time.Duration(s.config.HeartbeatTimeout)*time.Second {
				s.mu.Lock()
				cmd := c.cmd
				if c.failureReason == "" {
					c.failureReason = "heartbeat lebih tua dari batas"
				}
				s.mu.Unlock()
				if cmd != nil {
					_ = cmd.Process.Kill()
				}
			}
		}
	}
}
func (s *supervisor) schedule(c *child, reason string, now time.Time) {
	cutoff := now.Add(-10 * time.Minute)
	kept := c.restarts[:0]
	for _, t := range c.restarts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	c.restarts = kept
	if len(c.restarts) >= s.config.MaxRestarts {
		c.permanent = true
		log.Printf("proses %s gagal permanen setelah %d restart/10 menit (%s)", c.name, len(c.restarts), reason)
		return
	}
	c.restarts = append(c.restarts, now)
	wait := backoff[min(len(c.restarts)-1, len(backoff)-1)]
	c.next = now.Add(wait)
	log.Printf("proses %s akan dimulai ulang dalam %s: %s", c.name, wait, reason)
}
func (s *supervisor) stopAll() {
	s.mu.Lock()
	s.stopping = true
	s.mu.Unlock()
	for _, c := range s.children {
		s.mu.Lock()
		cmd := c.cmd
		s.mu.Unlock()
		if cmd != nil {
			_ = cmd.Process.Kill()
		}
	}
}
func (s *supervisor) logStatus() {
	parts := make([]string, 0, len(s.children))
	for _, c := range s.children {
		s.mu.Lock()
		running := c.cmd != nil
		s.mu.Unlock()
		state := "menunggu restart"
		if running {
			state = "berjalan"
		}
		if c.permanent {
			state = "gagal permanen"
		}
		parts = append(parts, c.name+"="+state)
	}
	log.Printf("status proses: %s", strings.Join(parts, ", "))
}
