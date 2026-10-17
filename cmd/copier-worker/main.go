// copier-worker supervises managed-copy follower processes on Windows.
// Managed job polling, MT5 login checks, EA startup, lifecycle and API
// reporting run through this Go process.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rnpproject/algentra-backend/internal/copier/managed"
	"github.com/rnpproject/algentra-backend/internal/textutil"
)

type config struct {
	APIBaseURL             string `json:"api_base_url"`
	WorkerAPIKeyEnv        string `json:"worker_api_key_env"`
	MasterConfig           string `json:"master_config"`
	MasterSource           string `json:"master_source"`
	TerminalTemplateDir    string `json:"terminal_template_dir"`
	RuntimeDir             string `json:"runtime_dir"`
	ExecutionMode          string `json:"execution_mode"`
	FeedAddress            string `json:"feed_address"`
	PollSeconds            int    `json:"poll_seconds"`
	HeartbeatTimeoutSecond int    `json:"heartbeat_timeout_seconds"`

	configDir, apiBase, masterConfig, templateDir, runtimeDir string
	projectRoot, apiKey, masterToken                          string
	signalDir                                                 string
	masterHealthURL                                           string
	masterSnapshotURL                                         string
}

type job struct {
	AccountID        int64   `json:"account_id"`
	Label            string  `json:"label"`
	Login            string  `json:"login"`
	Server           string  `json:"server"`
	BrokerPassword   string  `json:"broker_password"`
	FollowerSymbol   string  `json:"follower_symbol"`
	ModeLot          string  `json:"mode_lot"`
	RatioLot         float64 `json:"ratio_lot"`
	FixedLot         float64 `json:"lot_tetap"`
	MaxLotPerOrder   float64 `json:"max_lot_per_order"`
	MaxLotTotal      float64 `json:"max_lot_total"`
	MaxOpenPositions int     `json:"max_open_positions"`
}

type loginCheck struct {
	AccountID      int64  `json:"account_id"`
	Login          string `json:"login"`
	Server         string `json:"server"`
	BrokerPassword string `json:"broker_password"`
}

type child struct {
	accountID    int64
	fingerprint  string
	cmd          *exec.Cmd
	done         chan error
	log          *os.File
	heartbeat    string
	startedAt    time.Time
	executor     string
	feedToken    string
	startupINI   string
	parameterSet string
}

type worker struct {
	cfg            config
	client         *http.Client
	children       map[int64]*child
	retryAt        map[int64]time.Time
	failures       map[int64]int
	master         *exec.Cmd
	masterDone     chan error
	masterLog      *os.File
	masterRetryAt  time.Time
	stop           chan struct{}
	stopOnce       sync.Once
	loginProbePath string
	masterBinary   string
	feed           *managed.Feed
}

func main() {
	flag.Usage = func() { fmt.Fprintln(flag.CommandLine.Output(), "Usage: copier-worker [managed-worker.json]") }
	flag.Parse()
	if runtime.GOOS != "windows" {
		log.Fatal("managed copier worker hanya berjalan pada Windows")
	}
	path := "apps/copier-worker/config/managed-worker.json"
	if flag.NArg() > 1 {
		log.Fatal("copier-worker hanya menerima satu path konfigurasi")
	}
	if flag.NArg() == 1 {
		path = flag.Arg(0)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		log.Fatal(err)
	}
	w, err := newWorker(cfg)
	if err != nil {
		log.Fatal(err)
	}
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	go func() { <-interrupt; w.stopOnce.Do(func() { close(w.stop) }) }()
	if err := w.run(); err != nil {
		log.Fatal(err)
	}
}

func loadConfig(path string) (config, error) {
	var cfg config
	absolute, err := filepath.Abs(path)
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return cfg, fmt.Errorf("config worker tidak dapat dibaca: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("config worker bukan JSON valid: %w", err)
	}
	if cfg.WorkerAPIKeyEnv == "" || cfg.MasterConfig == "" || cfg.TerminalTemplateDir == "" || cfg.RuntimeDir == "" {
		return cfg, errors.New("config worker wajib memiliki worker_api_key_env, master_config, terminal_template_dir, dan runtime_dir")
	}
	if cfg.ExecutionMode == "" || cfg.ExecutionMode == "auto" {
		cfg.ExecutionMode = "mt5_ea"
	}
	if cfg.ExecutionMode != "mt5_ea" {
		return cfg, errors.New("execution_mode managed worker Go harus mt5_ea")
	}
	if cfg.ExecutionMode == "auto" {
		cfg.ExecutionMode = "mt5_ea"
	}
	if cfg.FeedAddress == "" {
		cfg.FeedAddress = "127.0.0.1:8766"
	}
	feedHost, _, err := net.SplitHostPort(cfg.FeedAddress)
	parsedFeedHost := net.ParseIP(feedHost)
	if err != nil || (feedHost != "localhost" && (parsedFeedHost == nil || !parsedFeedHost.IsLoopback())) {
		return cfg, errors.New("feed_address harus memakai alamat loopback dan port yang valid")
	}
	u, err := url.Parse(cfg.APIBaseURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && isLocal(u.Hostname()))) {
		return cfg, errors.New("api_base_url wajib HTTPS (HTTP hanya localhost atau 127.0.0.1)")
	}
	cfg.configDir = filepath.Dir(absolute)
	cfg.apiBase = strings.TrimRight(cfg.APIBaseURL, "/")
	cfg.masterConfig = resolve(cfg.configDir, cfg.MasterConfig)
	cfg.templateDir = resolve(cfg.configDir, cfg.TerminalTemplateDir)
	cfg.runtimeDir = resolve(cfg.configDir, cfg.RuntimeDir)
	cfg.PollSeconds = clamp(cfg.PollSeconds, 2, 60, 5)
	cfg.HeartbeatTimeoutSecond = clamp(cfg.HeartbeatTimeoutSecond, 15, 180, 45)
	cfg.projectRoot = findProjectRoot(absolute)
	cfg.apiKey = strings.TrimSpace(os.Getenv(cfg.WorkerAPIKeyEnv))
	cfg.masterToken = os.Getenv("MASTER_PUBLISHER_TOKEN")
	if len(cfg.apiKey) < 32 {
		return cfg, fmt.Errorf("environment variable %s wajib berisi worker key minimal 32 karakter", cfg.WorkerAPIKeyEnv)
	}
	if len(cfg.masterToken) < 32 {
		return cfg, errors.New("MASTER_PUBLISHER_TOKEN wajib tersedia minimal 32 karakter")
	}
	if _, err := os.Stat(filepath.Join(cfg.templateDir, "terminal64.exe")); err != nil {
		return cfg, errors.New("terminal_template_dir harus berisi terminal64.exe")
	}
	var master struct {
		TerminalPath  string `json:"terminal_path"`
		SignalDir     string `json:"folder_sinyal"`
		ListenAddress string `json:"listen_addr"`
	}
	masterData, err := os.ReadFile(cfg.masterConfig)
	if err != nil || json.Unmarshal(masterData, &master) != nil || master.SignalDir == "" {
		return cfg, errors.New("master_config wajib memiliki folder_sinyal")
	}
	if cfg.MasterSource == "" {
		cfg.MasterSource = "publisher_ea"
	}
	if cfg.MasterSource != "publisher_ea" && cfg.MasterSource != "telegram_signal" {
		return cfg, errors.New("master_source harus publisher_ea atau telegram_signal")
	}
	if cfg.MasterSource == "publisher_ea" && master.TerminalPath == "" {
		return cfg, errors.New("mode publisher_ea memerlukan terminal_path pada master_config")
	}
	if master.ListenAddress == "" {
		master.ListenAddress = "127.0.0.1:8765"
	}
	masterHost, _, err := net.SplitHostPort(master.ListenAddress)
	masterIP := net.ParseIP(masterHost)
	if err != nil || masterIP == nil || !masterIP.IsLoopback() {
		return cfg, errors.New("listen_addr pada master_config wajib menggunakan IP loopback")
	}
	cfg.signalDir = resolve(cfg.projectRoot, master.SignalDir)
	cfg.masterHealthURL = "http://" + master.ListenAddress + "/health"
	cfg.masterSnapshotURL = "http://" + master.ListenAddress + "/snapshot"
	return cfg, nil
}

func newWorker(cfg config) (*worker, error) {
	for _, directory := range []string{
		cfg.signalDir,
		filepath.Join(cfg.runtimeDir, "logs"),
		filepath.Join(cfg.runtimeDir, "states"),
		filepath.Join(cfg.runtimeDir, "reports"),
		filepath.Join(cfg.runtimeDir, "heartbeats"),
		filepath.Join(cfg.runtimeDir, "terminals"),
	} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, fmt.Errorf("buat direktori runtime %s: %w", directory, err)
		}
	}
	store, err := managed.OpenSnapshotStore(cfg.signalDir, filepath.Join(cfg.runtimeDir, "states", "master-snapshot.json"), 3*time.Second)
	if err != nil {
		return nil, err
	}
	feed, err := managed.NewFeed(store, filepath.Join(cfg.runtimeDir, "reports"), filepath.Join(cfg.runtimeDir, "heartbeats"), cfg.masterHealthURL)
	if err != nil {
		return nil, err
	}
	return &worker{
		cfg: cfg, client: &http.Client{Timeout: 20 * time.Second}, children: map[int64]*child{},
		retryAt: map[int64]time.Time{}, failures: map[int64]int{}, stop: make(chan struct{}),
		loginProbePath: filepath.Join(cfg.projectRoot, "algentra-mt5-login-probe.exe"),
		masterBinary:   filepath.Join(cfg.projectRoot, "algentra-copier-master.exe"),
		feed:           feed,
	}, nil
}

func (w *worker) run() error {
	if err := os.MkdirAll(w.cfg.runtimeDir, 0o700); err != nil {
		return err
	}
	lock, err := lockWorker(w.cfg.runtimeDir)
	if err != nil {
		return err
	}
	defer unlockWorker(lock)
	for _, dir := range []string{"logs", "configs", "terminals", "heartbeats", "reports", "states"} {
		if err := os.MkdirAll(filepath.Join(w.cfg.runtimeDir, dir), 0o700); err != nil {
			return err
		}
	}
	logFile, err := os.OpenFile(filepath.Join(w.cfg.runtimeDir, "logs", "managed_worker.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	listener, err := net.Listen("tcp", w.cfg.FeedAddress)
	if err != nil {
		return fmt.Errorf("listener managed MT5 gagal dibuka pada %s: %w", w.cfg.FeedAddress, err)
	}
	feedServer := &http.Server{Handler: w.feed, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	feedDone := make(chan error, 1)
	go func() { feedDone <- feedServer.Serve(listener) }()
	defer func() {
		_ = feedServer.Close()
		<-feedDone
	}()
	log.Printf("managed copier Go supervisor dimulai")
	ticker := time.NewTicker(time.Duration(w.cfg.PollSeconds) * time.Second)
	defer ticker.Stop()
	if err := w.syncOnce(); err != nil {
		log.Printf("sinkronisasi awal gagal: %v", err)
	}
	for {
		select {
		case <-w.stop:
			w.shutdown()
			return nil
		case <-ticker.C:
			if err := w.syncOnce(); err != nil {
				log.Printf("sinkronisasi worker gagal: %v", err)
			}
		}
	}
}

func (w *worker) request(endpoint string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return errors.New("payload worker tidak valid")
		}
		body = strings.NewReader(string(data))
	}
	method := http.MethodGet
	if payload != nil {
		method = http.MethodPost
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, w.cfg.apiBase+endpoint, body)
	if err != nil {
		return errors.New("URL API worker tidak valid")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Copier-Worker-Key", w.cfg.apiKey)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("API worker tidak dapat dijangkau (%T)", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("API worker gagal (HTTP %d pada %s)", resp.StatusCode, endpoint)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 4<<20))
	if err := decoder.Decode(out); err != nil {
		return errors.New("respons API worker tidak valid")
	}
	return nil
}

func (w *worker) startMaster() error {
	if w.master != nil {
		select {
		case err := <-w.masterDone:
			log.Printf("publisher master berhenti: %v", err)
			if w.masterLog != nil {
				_ = w.masterLog.Close()
			}
			w.master, w.masterDone = nil, nil
		default:
			return nil
		}
	}
	if time.Now().Before(w.masterRetryAt) {
		return nil
	}
	if _, err := os.Stat(w.masterBinary); err != nil {
		return errors.New("binary publisher master Go tidak ditemukan")
	}
	logFile, err := os.OpenFile(filepath.Join(w.cfg.runtimeDir, "logs", "master-publisher.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.Command(w.masterBinary, w.cfg.masterConfig)
	cmd.Dir = w.cfg.projectRoot
	cmd.Env = w.cleanEnvironment(true)
	prepareChild(cmd)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	w.master, w.masterLog = cmd, logFile
	w.masterDone = make(chan error, 1)
	w.masterRetryAt = time.Now().Add(5 * time.Second)
	go func() { w.masterDone <- cmd.Wait() }()
	log.Printf("publisher master Go dimulai pid=%d", cmd.Process.Pid)
	return nil
}

func (w *worker) cleanEnvironment(includeMasterToken bool) []string {
	allowed := make([]string, 0, len(os.Environ()))
	for _, pair := range os.Environ() {
		key, _, _ := strings.Cut(pair, "=")
		upper := strings.ToUpper(key)
		if key == w.cfg.WorkerAPIKeyEnv || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "PASS_") || strings.Contains(upper, "SECRET") || strings.Contains(upper, "TOKEN") || strings.Contains(upper, "API_KEY") || strings.Contains(upper, "APIKEY") {
			continue
		}
		allowed = append(allowed, pair)
	}
	if includeMasterToken {
		allowed = append(allowed, "MASTER_PUBLISHER_TOKEN="+w.cfg.masterToken)
	}
	return allowed
}

func (w *worker) startJob(j job, now time.Time) error {
	configPath, heartbeat, token, err := w.makeJobConfig(j)
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(w.cfg.runtimeDir, "logs", fmt.Sprintf("managed_%d.log", j.AccountID)), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		w.feed.Unregister(j.AccountID)
		return err
	}
	cmd := exec.Command(filepath.Join(w.cfg.runtimeDir, "terminals", fmt.Sprint(j.AccountID), "terminal64.exe"), "/portable", "/config:"+configPath)
	cmd.Dir = filepath.Dir(cmd.Path)
	cmd.Env = w.cleanEnvironment(false)
	prepareChild(cmd)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		w.feed.Unregister(j.AccountID)
		return err
	}
	c := &child{accountID: j.AccountID, fingerprint: fingerprint(j), cmd: cmd, done: make(chan error, 1), log: logFile, heartbeat: heartbeat, startedAt: now, executor: "mt5_ea", feedToken: token, startupINI: configPath}
	w.children[j.AccountID] = c
	go func() { c.done <- cmd.Wait() }()
	_ = w.report(j.AccountID, "STARTING", "")
	log.Printf("follower managed akun %d (mt5_ea) dimulai pid=%d", j.AccountID, cmd.Process.Pid)
	go w.removeStartupConfigAfterLaunch(c)
	return nil
}

func (w *worker) makeJobConfig(j job) (string, string, string, error) {
	if j.AccountID <= 0 || j.BrokerPassword == "" || j.Login == "" || j.Server == "" {
		return "", "", "", errors.New("data akun follower tidak lengkap")
	}
	token, err := newFeedToken()
	if err != nil {
		return "", "", "", err
	}
	name := fmt.Sprintf("managed_%d", j.AccountID)
	accountDir := filepath.Join(w.cfg.runtimeDir, "terminals", fmt.Sprint(j.AccountID))
	terminal := filepath.Join(accountDir, "terminal64.exe")
	if _, err := os.Stat(terminal); err != nil {
		if err := copyTree(w.cfg.templateDir, accountDir); err != nil {
			w.feed.Unregister(j.AccountID)
			return "", "", "", fmt.Errorf("terminal follower tidak dapat disalin: %w", err)
		}
	}
	if _, err := os.Stat(terminal); err != nil {
		w.feed.Unregister(j.AccountID)
		return "", "", "", errors.New("salinan terminal follower tidak lengkap")
	}
	if err := enableAutoTrading(accountDir); err != nil {
		w.feed.Unregister(j.AccountID)
		return "", "", "", fmt.Errorf("auto trading terminal gagal disiapkan: %w", err)
	}
	if err := w.prepareManagedEA(j, token, accountDir, name); err != nil {
		w.feed.Unregister(j.AccountID)
		return "", "", "", err
	}
	startupConfig := filepath.Join(accountDir, "managed-startup.ini")
	if err := writeManagedStartup(startupConfig, j, name); err != nil {
		w.feed.Unregister(j.AccountID)
		return "", "", "", err
	}
	return startupConfig, filepath.Join(w.cfg.runtimeDir, "heartbeats", "heartbeat_"+name+".txt"), token, nil
}

func (w *worker) validateLogin(check loginCheck) (bool, string) {
	accountDir := filepath.Join(w.cfg.runtimeDir, "terminals", fmt.Sprint(check.AccountID))
	terminal := filepath.Join(accountDir, "terminal64.exe")
	if _, err := os.Stat(terminal); err != nil {
		if err := copyTree(w.cfg.templateDir, accountDir); err != nil {
			if isTerminalFileBusy(err) {
				return false, terminalInUseMessage
			}
			return false, "Terminal follower tidak tersedia pada server."
		}
	}
	if _, err := os.Stat(terminal); err != nil {
		return false, "Terminal follower tidak tersedia pada server."
	}
	root := filepath.Join(w.cfg.projectRoot, "mt5", "experts", "Algentra", "MT5LoginProbe.ex5")
	target := filepath.Join(accountDir, "MQL5", "Experts", "Algentra", "MT5LoginProbe.ex5")
	if err := copyFile(root, target); err != nil {
		if isTerminalFileBusy(err) {
			return false, terminalInUseMessage
		}
		return false, "Expert MT5LoginProbe.ex5 belum dikompilasi pada server."
	}
	if err := enableAutoTrading(accountDir); err != nil {
		if isTerminalFileBusy(err) {
			return false, terminalInUseMessage
		}
		return false, "Worker tidak dapat menyiapkan izin trading terminal MT5."
	}
	payload := map[string]any{"path": terminal, "login": mustInt(check.Login), "password": check.BrokerPassword, "server": check.Server}
	input, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(context.Background(), 155*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.loginProbePath)
	cmd.Dir = w.cfg.projectRoot
	cmd.Env = w.cleanEnvironment(false)
	cmd.Stdin = strings.NewReader(string(input))
	var stdout strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return false, "Pemeriksaan MT5 melewati batas 150 detik. Terminal tidak merespons; worker utama tetap berjalan."
		}
		return false, "Worker tidak dapat memvalidasi login pada terminal MT5."
	}
	var result struct {
		Valid   *bool   `json:"valid"`
		Message *string `json:"message"`
	}
	if json.Unmarshal([]byte(stdout.String()), &result) != nil || result.Valid == nil || result.Message == nil {
		return false, "Worker tidak dapat memvalidasi login pada terminal MT5."
	}
	return *result.Valid, limitRunes(*result.Message, 240)
}

const terminalInUseMessage = "File terminal MT5 sedang dipakai proses lain. Tutup terminal MT5 untuk akun ini lalu coba simpan kredensial lagi."

func (w *worker) syncOnce() error {
	if err := w.startMaster(); err != nil {
		log.Printf("publisher master start gagal: %v", err)
	}
	var response struct {
		Jobs        []job        `json:"jobs"`
		LoginChecks []loginCheck `json:"login_checks"`
		ManagedIDs  []int64      `json:"managed_account_ids"`
	}
	if err := w.request("/api/mt5/copier/jobs", nil, &response); err != nil {
		return err
	}
	if response.Jobs == nil {
		return errors.New("API worker tidak mengembalikan daftar jobs")
	}
	if response.ManagedIDs == nil {
		return errors.New("API worker tidak mengembalikan daftar akun managed yang valid")
	}
	if w.cfg.MasterSource == "telegram_signal" {
		if err := w.bridgeTelegramMasterSnapshot(); err != nil {
			log.Printf("snapshot TelegramSignalEA belum diteruskan ke copier: %v", err)
		}
	}
	for _, id := range response.ManagedIDs {
		if id <= 0 {
			return errors.New("API worker tidak mengembalikan daftar akun managed yang valid")
		}
	}
	for _, check := range response.LoginChecks {
		if check.AccountID <= 0 {
			continue
		}
		w.stopChild(check.AccountID, false)
		valid, message := w.validateLogin(check)
		var ignored any
		if err := w.request(fmt.Sprintf("/api/mt5/copier/jobs/%d/login-check", check.AccountID), map[string]any{"valid": valid, "message": message}, &ignored); err != nil {
			log.Printf("hasil pemeriksaan login akun %d tidak terkirim: %v", check.AccountID, err)
		}
	}
	desired := make(map[int64]job)
	usedBroker := make(map[string]struct{})
	for _, j := range response.Jobs {
		if j.AccountID <= 0 {
			continue
		}
		broker := textutil.Fold(j.Server) + "\x00" + j.Login
		if _, exists := usedBroker[broker]; exists {
			_ = w.report(j.AccountID, "ERROR", "Akun broker duplikat di daftar worker.")
			continue
		}
		usedBroker[broker] = struct{}{}
		desired[j.AccountID] = j
	}
	for id := range w.children {
		if _, ok := desired[id]; !ok {
			w.stopChild(id, true)
		}
	}
	managed := make(map[int64]struct{}, len(response.ManagedIDs))
	for _, id := range response.ManagedIDs {
		managed[id] = struct{}{}
	}
	if entries, err := os.ReadDir(filepath.Join(w.cfg.runtimeDir, "terminals")); err == nil {
		for _, entry := range entries {
			id, parseErr := parsePositive(entry.Name())
			if entry.IsDir() && parseErr == nil {
				if _, ok := managed[id]; !ok {
					if err := w.removeOrphan(id); err != nil {
						log.Printf("runtime orphan akun %d tidak dapat dibersihkan: %v", id, err)
					}
				}
			}
		}
	}
	for id, j := range desired {
		current := w.children[id]
		sum := fingerprint(j)
		if current != nil && current.fingerprint != sum {
			w.stopChild(id, false)
			current = nil
		}
		if current == nil {
			if time.Now().Before(w.retryAt[id]) {
				_ = w.report(id, "ERROR", "Follower gagal mulai; worker sedang menunggu sebelum mencoba lagi.")
				continue
			}
			if err := w.startJob(j, time.Now()); err != nil {
				log.Printf("follower managed akun %d gagal disiapkan: %v", id, err)
				w.retryAt[id] = time.Now().Add(10 * time.Second)
				_ = w.report(id, "ERROR", "Persiapan follower gagal.")
				continue
			}
			current = w.children[id]
		}
		select {
		case err := <-current.done:
			delete(w.children, id)
			w.feed.Unregister(id)
			w.failures[id]++
			backoff := time.Duration(min(60, 1<<min(w.failures[id], 6))) * time.Second
			w.retryAt[id] = time.Now().Add(backoff)
			current.log.Close()
			_ = w.report(id, "ERROR", "Proses follower berhenti; worker akan mencoba lagi.")
			log.Printf("follower akun %d berhenti: %v", id, err)
			continue
		default:
		}
		info, err := os.Stat(current.heartbeat)
		heartbeatAge := time.Duration(1<<63 - 1)
		if err == nil {
			heartbeatAge = time.Since(info.ModTime())
		}
		if heartbeatAge <= time.Duration(w.cfg.HeartbeatTimeoutSecond)*time.Second {
			w.failures[id] = 0
			_ = w.report(id, "RUNNING", "")
			w.syncReport(id)
		} else if time.Since(current.startedAt) < time.Duration(w.cfg.HeartbeatTimeoutSecond)*time.Second {
			_ = w.report(id, "STARTING", "")
		} else {
			_ = w.report(id, "ERROR", "Follower belum terhubung ke terminal MT5.")
		}
	}
	return nil
}

func (w *worker) bridgeTelegramMasterSnapshot() error {
	var current struct {
		Active    bool `json:"active"`
		Positions []struct {
			Identifier int64   `json:"identifier"`
			Ticket     int64   `json:"ticket"`
			Symbol     string  `json:"symbol"`
			Type       int     `json:"type"`
			Volume     float64 `json:"volume"`
			OpenPrice  float64 `json:"price_open"`
			StopLoss   float64 `json:"sl"`
			TakeProfit float64 `json:"tp"`
			Comment    string  `json:"comment"`
		} `json:"positions"`
	}
	if err := w.request("/api/mt5/copier/master/snapshot", nil, &current); err != nil {
		return err
	}
	// A stale/disconnected master must never look like a flat account, which
	// would otherwise generate close signals for every follower position.
	if !current.Active {
		return errors.New("snapshot master tidak aktif atau kedaluwarsa")
	}
	payload, err := json.Marshal(map[string]any{"positions": current.Positions})
	if err != nil {
		return errors.New("snapshot master tidak dapat dienkode")
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, w.cfg.masterSnapshotURL, bytes.NewReader(payload))
	if err != nil {
		return errors.New("request snapshot lokal tidak valid")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+w.cfg.masterToken)
	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("publisher lokal tidak terjangkau: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("publisher lokal menolak snapshot (HTTP %d)", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return nil
}

func (w *worker) syncReport(id int64) {
	path := filepath.Join(w.cfg.runtimeDir, "reports", fmt.Sprintf("%d.json", id))
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > time.Duration(w.cfg.HeartbeatTimeoutSecond)*time.Second {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var report map[string]any
	if json.Unmarshal(data, &report) != nil {
		return
	}
	scan, _ := report["history_scan_msc"]
	delete(report, "history_scan_msc")
	var response map[string]any
	if err := w.request(fmt.Sprintf("/api/mt5/copier/jobs/%d/report", id), report, &response); err != nil {
		log.Printf("laporan akun %d tidak tersinkron: %v", id, err)
		return
	}
	cursor := map[string]any{"history_cursor": response["history_cursor"], "history_cursor_msc": response["history_cursor_msc"], "scan_msc": scan}
	data, _ = json.Marshal(cursor)
	path = filepath.Join(w.cfg.runtimeDir, "reports", fmt.Sprintf("%d.cursor.json", id))
	if writeAtomic(path, data) != nil {
		log.Printf("cursor histori akun %d tidak dapat disimpan", id)
	}
}

func (w *worker) report(id int64, status, message string) error {
	var body any = map[string]any{"status": status}
	if message != "" {
		body = map[string]any{"status": status, "message": message}
	}
	return w.request(fmt.Sprintf("/api/mt5/copier/jobs/%d/status", id), body, nil)
}

func (w *worker) stopChild(id int64, report bool) {
	c := w.children[id]
	if c == nil {
		return
	}
	delete(w.children, id)
	w.feed.Unregister(id)
	if c.startupINI != "" {
		defer os.Remove(c.startupINI)
	}
	select {
	case <-c.done:
	default:
		_ = stopChildProcess(c.cmd, c.done, 10*time.Second)
	}
	if c.log != nil {
		_ = c.log.Close()
	}
	if report {
		_ = w.report(id, "STOPPED", "")
	}
}

func (w *worker) stopMaster() {
	if w.master == nil {
		return
	}
	_ = stopChildProcess(w.master, w.masterDone, 5*time.Second)
	if w.masterLog != nil {
		_ = w.masterLog.Close()
	}
	w.master, w.masterDone = nil, nil
}

func (w *worker) shutdown() {
	for id := range w.children {
		w.stopChild(id, true)
	}
	w.stopMaster()
	log.Printf("managed copier Go supervisor berhenti")
}

func (w *worker) removeOrphan(id int64) error {
	dir := filepath.Join(w.cfg.runtimeDir, "terminals", fmt.Sprint(id))
	if err := removeWithin(filepath.Join(w.cfg.runtimeDir, "terminals"), dir); err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	for _, item := range [][2]string{
		{"configs", fmt.Sprintf("managed_%d.json", id)}, {"states", fmt.Sprintf("%d.json", id)},
		{"states", fmt.Sprintf("%d.copy-after", id)},
		{"heartbeats", fmt.Sprintf("heartbeat_managed_%d.txt", id)}, {"reports", fmt.Sprintf("%d.json", id)},
		{"reports", fmt.Sprintf("%d.cursor.json", id)}, {"logs", fmt.Sprintf("managed_%d.log", id)},
	} {
		_ = os.Remove(filepath.Join(w.cfg.runtimeDir, item[0], item[1]))
	}
	return nil
}

func fingerprint(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func resolve(base, value string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(base, value))
}
func isLocal(host string) bool {
	return strings.EqualFold(host, "localhost") || host == "127.0.0.1" || host == "::1"
}
func clamp(value, low, high, fallback int) int {
	if value == 0 {
		return fallback
	}
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
func findProjectRoot(path string) string {
	dir := filepath.Dir(path)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(path)
		}
		dir = parent
	}
}
func parsePositive(value string) (int64, error) {
	var id int64
	_, err := fmt.Sscan(value, &id)
	if err != nil || id <= 0 || fmt.Sprint(id) != value {
		return 0, errors.New("ID tidak valid")
	}
	return id, nil
}
func mustInt(value string) int64 { var n int64; _, _ = fmt.Sscan(value, &n); return n }
func limitRunes(value string, limit int) string {
	r := []rune(value)
	if len(r) > limit {
		return string(r[:limit])
	}
	return value
}
func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func signalDir(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var value struct {
		Dir string `json:"folder_sinyal"`
	}
	_ = json.Unmarshal(data, &value)
	return value.Dir
}
