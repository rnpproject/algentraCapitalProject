package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// managedBridgeRequest is the local, FILE_COMMON transport envelope used by
// managed EA instances. Authentication is still checked by managed.Feed.
type managedBridgeRequest struct {
	ID     uint64 `json:"id"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Token  string `json:"token"`
	Login  string `json:"login"`
	Server string `json:"server"`
	Body   string `json:"body"`
}

func (w *worker) serveManagedFileBridge(c *child) {
	commonDir := os.Getenv("APPDATA")
	if commonDir == "" {
		var err error
		commonDir, err = os.UserConfigDir()
		if err != nil {
			log.Printf("managed file bridge akun %d: common folder tidak ditemukan: %v", c.accountID, err)
			return
		}
	}
	commonDir = filepath.Join(commonDir, "MetaQuotes", "Terminal", "Common", "Files")
	if err := os.MkdirAll(commonDir, 0o700); err != nil {
		log.Printf("managed file bridge akun %d: folder common tidak dapat dibuat: %v", c.accountID, err)
		return
	}
	base := filepath.Join(commonDir, "algentra-managed-"+strconv.FormatInt(c.accountID, 10))
	requestPath, responsePath := base+".req", base+".res"
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopped:
			_ = os.Remove(requestPath)
			_ = os.Remove(responsePath)
			return
		case <-ticker.C:
			data, err := os.ReadFile(requestPath)
			if err != nil {
				continue
			}
			// Claim before handling so repeated polls cannot process one EA call twice.
			claimed := requestPath + ".processing"
			if err := os.Rename(requestPath, claimed); err != nil {
				continue
			}
			data, err = os.ReadFile(claimed)
			_ = os.Remove(claimed)
			if err != nil {
				continue
			}
			var request managedBridgeRequest
			if err := json.Unmarshal(data, &request); err != nil {
				log.Printf("managed file bridge akun %d: request JSON tidak valid: %v", c.accountID, err)
				continue
			}
			if request.ID == 0 || (request.Method != http.MethodGet && request.Method != http.MethodPost) ||
				(request.Path != "/api/mt5/follower/positions" && request.Path != "/api/mt5/follower/report") {
				_ = writeBridgeResponse(responsePath, request.ID, http.StatusBadRequest, []byte("invalid bridge request"))
				continue
			}
			body := strings.NewReader(request.Body)
			httpRequest, err := http.NewRequest(request.Method, "http://managed.local"+request.Path, body)
			if err != nil {
				_ = writeBridgeResponse(responsePath, request.ID, http.StatusBadRequest, []byte("invalid bridge request"))
				continue
			}
			httpRequest.Header.Set("X-Account-Token", request.Token)
			httpRequest.Header.Set("X-MT5-Login", request.Login)
			httpRequest.Header.Set("X-MT5-Server", request.Server)
			httpRequest.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			w.feed.ServeHTTP(recorder, httpRequest)
			result := recorder.Result()
			responseBody, readErr := io.ReadAll(io.LimitReader(result.Body, 16<<20))
			_ = result.Body.Close()
			if readErr != nil {
				responseBody = []byte("bridge response unavailable")
				result.StatusCode = http.StatusInternalServerError
			}
			if err := writeBridgeResponse(responsePath, request.ID, result.StatusCode, responseBody); err != nil {
				log.Printf("managed file bridge akun %d: gagal menulis respons: %v", c.accountID, err)
			}
		}
	}
}

func writeBridgeResponse(path string, id uint64, status int, body []byte) error {
	temp := fmt.Sprintf("%s.%d.tmp", path, id)
	content := make([]byte, 0, 32+len(body))
	content = append(content, []byte(fmt.Sprintf("%d %d\n", id, status))...)
	content = append(content, body...)
	if err := os.WriteFile(temp, content, 0o600); err != nil {
		return err
	}
	// The EA permits exactly one in-flight request per account and removes its
	// previous response before publishing a new request.
	_ = os.Remove(path)
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}
