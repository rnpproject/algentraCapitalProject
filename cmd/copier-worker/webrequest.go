package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var webRequestGUIApplyMu sync.Mutex

func (w *worker) configureManagedWebRequest(c *child, address string) {
	if c == nil || c.cmd == nil || c.cmd.Process == nil {
		return
	}
	url := "http://" + address
	if err := w.applyManagedWebRequest(c, url); err != nil {
		log.Printf("WebRequest allow-list akun %d belum berhasil: %v", c.accountID, err)
	}
}

func (w *worker) applyManagedWebRequest(c *child, url string) error {
	// MT5's Options dialog is the supported way to grant WebRequest access.
	// Some Windows VM builds keep this permission in machine-bound data, so
	// provisioning the terminal template or common.ini alone is not sufficient.
	// Apply after startup and serialize desktop input across this worker's MT5s.
	select {
	case <-time.After(time.Second):
	case <-c.stopped:
		return fmt.Errorf("terminal follower berhenti sebelum pengaturan WebRequest")
	}

	webRequestGUIApplyMu.Lock()
	defer webRequestGUIApplyMu.Unlock()

	assetDir := filepath.Join(w.cfg.projectRoot, "apps", "copier-worker", "assets", "autoit")
	interpreter := filepath.Join(assetDir, "AutoIt3_x64.exe")
	script := filepath.Join(assetDir, "set_webrequest_allowlist.au3")
	if _, err := os.Stat(interpreter); err != nil {
		return fmt.Errorf("interpreter AutoIt tidak ditemukan di %s", interpreter)
	}
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("script konfigurasi WebRequest tidak ditemukan di %s", script)
	}
	logPath := filepath.Join(w.cfg.runtimeDir, "logs", fmt.Sprintf("webrequest_%d.log", c.accountID))
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		select {
		case <-c.stopped:
			return fmt.Errorf("terminal follower sudah berhenti")
		default:
		}
		_ = os.Remove(logPath)
		ctx, cancel := context.WithTimeout(context.Background(), 23*time.Second)
		cmd := exec.CommandContext(ctx, interpreter, script, fmt.Sprint(c.cmd.Process.Pid), url, logPath)
		cmd.Dir = filepath.Dir(c.cmd.Path)
		cmd.Env = w.cleanEnvironment(false)
		output, err := cmd.CombinedOutput()
		cancel()
		logText, _ := os.ReadFile(logPath)
		if err == nil && strings.Contains(string(logText), "RESULT=OK") {
			log.Printf("WebRequest allow-list akun %d berhasil disiapkan untuk %s (percobaan %d)", c.accountID, url, attempt)
			return nil
		}
		lastErr = fmt.Errorf("percobaan %d gagal; output=%s; log=%s", attempt,
			strings.TrimSpace(string(output)), strings.TrimSpace(string(logText)))
		log.Printf("WebRequest allow-list akun %d: %v", c.accountID, lastErr)
		if strings.Contains(string(logText), "confirm_control_focused_but_dialog_still_open") ||
			strings.Contains(string(logText), "confirm_button_not_found_dialog_left_open") {
			// The script deliberately leaves Options open after a failed confirm.
			// Do not send another Ctrl+O into that modal dialog on the next retry.
			return lastErr
		}
		if attempt < 3 {
			select {
			case <-time.After(2 * time.Second):
			case <-c.stopped:
				return fmt.Errorf("terminal follower berhenti sebelum pengaturan WebRequest")
			}
		}
	}
	return lastErr
}
