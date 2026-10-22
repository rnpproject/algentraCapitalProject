package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type request struct {
	Path     string `json:"path"`
	Login    int64  `json:"login"`
	Password string `json:"password"`
	Server   string `json:"server"`
}

type result struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

type status struct {
	Ready   bool   `json:"ready"`
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
	Login   int64  `json:"login"`
	Server  string `json:"server"`
}

func main() {
	out := result{Valid: false, Message: "Worker tidak dapat memvalidasi login pada terminal MT5."}
	_ = run(&out) // Keep stdout machine-readable; callers never receive command-line noise.
	if len([]rune(out.Message)) > 240 {
		out.Message = string([]rune(out.Message)[:240])
	}
	_ = json.NewEncoder(os.Stdout).Encode(out)
}

func run(out *result) error {
	decoder := json.NewDecoder(io.LimitReader(os.Stdin, 16<<10))
	var req request
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("request harus berisi satu object JSON")
	}
	if req.Login <= 0 || req.Password == "" || strings.TrimSpace(req.Server) == "" || strings.ContainsAny(req.Server, "\r\n") || strings.ContainsAny(req.Password, "\r\n") {
		return errors.New("data akun MT5 tidak valid")
	}
	terminal, err := filepath.Abs(req.Path)
	if err != nil {
		return err
	}
	if info, err := os.Stat(terminal); err != nil || info.IsDir() || !strings.EqualFold(filepath.Ext(terminal), ".exe") {
		return errors.New("path terminal64.exe tidak valid")
	}
	ipcDir, err := os.MkdirTemp("", "algentra-mt5-login-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(ipcDir)
	ipcDir, err = filepath.Abs(ipcDir)
	if err != nil {
		return err
	}
	if err := os.Chmod(ipcDir, 0o700); err != nil {
		return err
	}
	configPath := filepath.Join(ipcDir, "mt5-login-probe.ini")
	reportName := "algentra-login-probe.json"
	reportPath := filepath.Join(filepath.Dir(terminal), "MQL5", "Files", reportName)
	_ = os.Remove(reportPath)
	defer os.Remove(reportPath)
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(configPath, []byte(makeConfig(req)), 0o600); err != nil {
		return err
	}
	defer os.Remove(configPath)
	defer os.Chmod(ipcDir, 0o700)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, terminal, "/portable", "/config:"+configPath)
	cmd.Dir = filepath.Dir(terminal)
	if err := cmd.Start(); err != nil {
		out.Message = "Worker tidak dapat memulai pemeriksaan MT5."
		return err
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	readReport := func() (bool, error) {
		data, err := os.ReadFile(reportPath)
		if err != nil {
			return false, err
		}
		var report status
		if err := json.Unmarshal(data, &report); err != nil || !report.Ready {
			return false, err
		}
		out.Valid, out.Message = report.Valid, report.Message
		if report.Login != req.Login || !strings.EqualFold(report.Server, req.Server) {
			out.Valid = false
			out.Message = "Terminal terhubung ke akun atau server yang berbeda dari data yang dimasukkan."
		}
		return true, nil
	}
	for {
		select {
		case <-ctx.Done():
			out.Message = "Pemeriksaan MT5 melewati batas 150 detik. Terminal tidak merespons; worker utama tetap berjalan."
			return ctx.Err()
		case waitErr := <-wait:
			// A terminal can exit just after the EA flushes its report. Read once
			// more before treating an early process exit as a failed login check.
			if ready, _ := readReport(); ready {
				return nil
			}
			out.Message = "Terminal MT5 berhenti sebelum laporan login tersedia. Periksa koneksi broker dan log terminal."
			if waitErr != nil {
				return waitErr
			}
			return errors.New("terminal MT5 berhenti sebelum hasil login tersedia")
		case <-ticker.C:
			ready, _ := readReport()
			if !ready {
				continue
			}
			_ = cmd.Process.Kill()
			<-wait
			return nil
		}
	}
}

func makeConfig(req request) string {
	return fmt.Sprintf("[Common]\r\nLogin=%d\r\nServer=%s\r\nPassword=%s\r\n\r\n[Experts]\r\nEnabled=1\r\nAllowLiveTrading=1\r\nAllowDllImport=0\r\nAccount=0\r\nProfile=0\r\n\r\n[StartUp]\r\nExpert=Algentra\\MT5LoginProbe\r\nSymbol=XAUUSD\r\nPeriod=M1\r\n", req.Login, req.Server, req.Password)
}
