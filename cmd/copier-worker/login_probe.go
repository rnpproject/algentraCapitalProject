package main

// Login probe: membuka terminal MT5 follower dengan konfigurasi startup yang
// ditulis langsung oleh worker, lalu menunggu laporan dari Expert
// MT5LoginProbe dan membaca log terminal untuk alasan penolakan broker.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

const (
	probeTimeout    = 150 * time.Second
	probeStatusFile = "algentra-login-probe.json"
)

type probeStatus struct {
	Ready   bool   `json:"ready"`
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
	Login   int64  `json:"login"`
	Server  string `json:"server"`
}

func (w *worker) validateLogin(check loginCheck) (bool, string) {
	valid, message := w.probeLogin(check)
	return valid, limitRunes(message, 240)
}

func (w *worker) probeLogin(check loginCheck) (bool, string) {
	login, err := strconv.ParseInt(strings.TrimSpace(check.Login), 10, 64)
	if err != nil || login <= 0 {
		return false, "Nomor login MT5 harus berupa angka."
	}
	server := strings.TrimSpace(check.Server)
	if server == "" || check.BrokerPassword == "" || strings.ContainsAny(server+check.BrokerPassword, "\r\n\x00") {
		return false, "Server atau password broker kosong atau mengandung karakter yang tidak valid."
	}
	accountDir := filepath.Join(w.cfg.runtimeDir, "terminals", fmt.Sprint(check.AccountID))
	terminal := filepath.Join(accountDir, "terminal64.exe")

	// Terminal sisa dari pemeriksaan sebelumnya (timeout) masih mengunci file,
	// folder data, dan port MCP 22346. Tutup dulu sebelum memulai yang baru.
	w.killTerminalsIn(accountDir)

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
	source := filepath.Join(w.cfg.projectRoot, "mt5", "experts", "Algentra", "MT5LoginProbe.ex5")
	target := filepath.Join(accountDir, "MQL5", "Experts", "Algentra", "MT5LoginProbe.ex5")
	if err := copyFile(source, target); err != nil {
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

	filesDir := filepath.Join(accountDir, "MQL5", "Files")
	statusPath := filepath.Join(filesDir, probeStatusFile)
	if err := os.MkdirAll(filesDir, 0o700); err != nil {
		return false, "Worker tidak dapat menyiapkan folder status terminal MT5."
	}
	if err := os.Remove(statusPath); err != nil && !os.IsNotExist(err) {
		return false, terminalInUseMessage
	}

	probeDir, err := os.MkdirTemp("", "algentra-mt5-login-")
	if err != nil {
		return false, "Worker tidak dapat menyiapkan folder sementara pemeriksaan login."
	}
	defer os.RemoveAll(probeDir)
	_ = os.Chmod(probeDir, 0o700)
	iniPath := filepath.Join(probeDir, "login-probe.ini")
	ini := strings.Join([]string{
		"[Common]",
		"Login=" + strconv.FormatInt(login, 10),
		"Password=" + check.BrokerPassword,
		"Server=" + server,
		"KeepPrivate=1",
		"NewsEnable=0",
		"ProxyEnable=0",
		"",
		"[Experts]",
		"AllowLiveTrading=1",
		"AllowDllImport=0",
		"Enabled=1",
		"Account=0",
		"Profile=0",
		"",
		"[StartUp]",
		`Expert=Algentra\MT5LoginProbe`,
		"Symbol=EURUSD",
		"Period=M1",
		"",
	}, "\r\n")
	if err := os.WriteFile(iniPath, []byte(ini), 0o600); err != nil {
		return false, "Worker tidak dapat menulis konfigurasi startup terminal MT5."
	}
	// File ini berisi password; hapus secepat mungkin setelah terminal membacanya.
	removeINI := time.AfterFunc(20*time.Second, func() { _ = os.Remove(iniPath) })
	defer removeINI.Stop()
	defer os.Remove(iniPath)

	offsets := logOffsets(accountDir)
	cmd := exec.Command(terminal, "/portable", "/config:"+iniPath)
	cmd.Dir = accountDir
	cmd.Env = w.cleanEnvironment(false)
	prepareChild(cmd)
	if err := cmd.Start(); err != nil {
		return false, "Terminal MT5 tidak dapat dijalankan oleh worker."
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	exited := false
	defer func() {
		if !exited {
			_ = stopChildProcess(cmd, done, 10*time.Second)
		}
		w.killTerminalsIn(accountDir)
	}()

	deadline := time.NewTimer(probeTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	var last probeStatus
	seen := false
	for n := 0; ; n++ {
		select {
		case <-w.stop:
			return false, "Pemeriksaan dibatalkan karena worker dihentikan."
		case waitErr := <-done:
			exited = true
			if status, ok := readProbeStatus(statusPath); ok && status.Ready {
				return validateProbeResult(status, login, server)
			}
			authorized, reason := scanAuthLog(accountDir, offsets, login)
			if reason != "" {
				message, _ := authFailureMessage(reason)
				return false, message
			}
			if authorized {
				return false, "Login broker berhasil, tetapi Expert MT5LoginProbe tidak mengirim laporan status. Periksa kompilasi expert dan izin Algo Trading."
			}
			if waitErr != nil {
				return false, "Terminal MT5 berhenti sebelum laporan login diterima. Periksa log terminal dan konfigurasi startup."
			}
			return false, "Terminal MT5 berhenti sebelum login selesai."
		case <-deadline.C:
			authorized, reason := scanAuthLog(accountDir, offsets, login)
			switch {
			case authorized && !seen:
				return false, "Login broker berhasil, tetapi Expert MT5LoginProbe tidak melapor. Periksa Auto Trading dan kompilasi expert."
			case reason != "":
				message, _ := authFailureMessage(reason)
				return false, message
			case seen && last.Message != "" && !strings.Contains(strings.ToLower(last.Message), "menunggu koneksi broker"):
				return false, last.Message
			default:
				return false, "Terminal MT5 terbuka tetapi tidak pernah mencoba login ke broker dalam 150 detik. Periksa nama server dan konfigurasi startup terminal."
			}
		case <-tick.C:
		}
		if status, ok := readProbeStatus(statusPath); ok {
			last, seen = status, true
			if status.Ready {
				return validateProbeResult(status, login, server)
			}
		}
		if n%3 == 2 {
			if _, reason := scanAuthLog(accountDir, offsets, login); reason != "" {
				if message, definitive := authFailureMessage(reason); definitive {
					return false, message
				}
			}
		}
	}
}

func validateProbeResult(status probeStatus, login int64, expectedServer string) (bool, string) {
	if status.Login != login {
		return false, "Terminal login ke akun lain dari yang didaftarkan."
	}
	if !strings.EqualFold(strings.TrimSpace(status.Server), strings.TrimSpace(expectedServer)) {
		return false, fmt.Sprintf("Server broker tidak cocok (diminta: %s; terminal: %s).", expectedServer, status.Server)
	}
	return status.Valid, status.Message
}

func readProbeStatus(path string) (probeStatus, bool) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return probeStatus{}, false
	}
	var status probeStatus
	if json.Unmarshal(data, &status) != nil {
		return probeStatus{}, false // file sedang ditulis EA; coba lagi
	}
	return status, true
}

func logOffsets(dir string) map[string]int64 {
	offsets := map[string]int64{}
	files, _ := filepath.Glob(filepath.Join(dir, "logs", "*.log"))
	for _, f := range files {
		if info, err := os.Stat(f); err == nil {
			offsets[f] = info.Size()
		}
	}
	return offsets
}

// scanAuthLog membaca log terminal (UTF-16LE) yang ditulis setelah probe mulai
// dan mengembalikan hasil otorisasi terakhir untuk nomor login tersebut.
func scanAuthLog(dir string, offsets map[string]int64, login int64) (authorized bool, failure string) {
	files, _ := filepath.Glob(filepath.Join(dir, "logs", "*.log"))
	needle := "'" + strconv.FormatInt(login, 10) + "'"
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		start := offsets[f]
		if start < 0 || start > int64(len(data)) {
			start = 0
		}
		data = data[start&^1:]
		units := make([]uint16, 0, len(data)/2)
		for i := 0; i+1 < len(data); i += 2 {
			units = append(units, uint16(data[i])|uint16(data[i+1])<<8)
		}
		for _, line := range strings.Split(string(utf16.Decode(units)), "\n") {
			if !strings.Contains(line, needle) {
				continue
			}
			lower := strings.ToLower(line)
			if strings.Contains(lower, "authorized on") {
				authorized, failure = true, ""
			} else if i := strings.LastIndex(lower, "authorization on"); i >= 0 {
				if j := strings.LastIndex(lower, "failed ("); j > i {
					authorized = false
					failure = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(lower[j+len("failed ("):]), ")"))
				}
			}
		}
	}
	return authorized, failure
}

func authFailureMessage(reason string) (string, bool) {
	r := strings.ToLower(reason)
	switch {
	case strings.Contains(r, "invalid account"):
		return "Login ditolak broker (Invalid account). Periksa nomor login, password, dan nama server.", true
	case strings.Contains(r, "account disabled"):
		return "Akun dinonaktifkan oleh broker.", true
	case strings.Contains(r, "no connection"):
		return "Terminal tidak dapat terhubung ke server broker (No connection). Periksa nama server dan akses internet keluar dari server Windows.", false
	default:
		return "Broker menolak login: " + reason + ".", false
	}
}

// killTerminalsIn menghentikan terminal64.exe yang berjalan dari folder akun
// tertentu saja (terminal akun lain dan master tidak tersentuh).
func (w *worker) killTerminalsIn(dir string) {
	if runtime.GOOS != "windows" {
		return
	}
	script := `$dir = $env:ALGENTRA_TERMINAL_DIR.TrimEnd('\') + '\'
Get-CimInstance Win32_Process -Filter "Name='terminal64.exe'" | Where-Object { $_.ExecutablePath -and $_.ExecutablePath.StartsWith($dir, [System.StringComparison]::OrdinalIgnoreCase) } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }`
	units := utf16.Encode([]rune(script))
	raw := make([]byte, 0, len(units)*2)
	for _, u := range units {
		raw = append(raw, byte(u), byte(u>>8))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(raw))
	cmd.Env = append(w.cleanEnvironment(false), "ALGENTRA_TERMINAL_DIR="+filepath.Clean(dir))
	_ = cmd.Run()
	time.Sleep(time.Second) // beri waktu Windows melepas handle file
}
