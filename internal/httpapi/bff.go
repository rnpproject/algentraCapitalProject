package httpapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxSignedBodyBytes = 6 << 20
	maxClockSkew       = 60 * time.Second
	nonceTTL           = 120 * time.Second
	maxTrackedNonces   = 100_000
)

type signatureGuard struct {
	secret []byte
	mu     sync.Mutex
	used   map[string]time.Time
}

func BFFSignature(secret string, next http.Handler) http.Handler {
	if secret == "" {
		return next
	}
	g := &signatureGuard{secret: []byte(secret), used: make(map[string]time.Time)}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/") || r.URL.Path == "/v1/payments/midtrans/notification" {
			next.ServeHTTP(w, r)
			return
		}
		if r.ContentLength > maxSignedBodyBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "Ukuran permintaan melewati batas")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSignedBodyBytes))
		if err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "Ukuran permintaan melewati batas")
			return
		}
		timestamp := r.Header.Get("X-Algentra-Timestamp")
		nonce := r.Header.Get("X-Algentra-Nonce")
		supplied := r.Header.Get("X-Algentra-Signature")
		seconds, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil || len(nonce) < 16 || len(nonce) > 128 {
			writeError(w, http.StatusUnauthorized, "Tanda tangan BFF tidak valid")
			return
		}
		now := time.Now()
		if delta := now.Sub(time.Unix(seconds, 0)); delta < -maxClockSkew || delta > maxClockSkew {
			writeError(w, http.StatusUnauthorized, "Tanda tangan BFF kedaluwarsa atau nonce tidak valid")
			return
		}
		target := r.URL.Path
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		bodyHash := sha256.Sum256(body)
		payload := timestamp + "\n" + nonce + "\n" + strings.ToUpper(r.Method) + "\n" + target + "\n" + hex.EncodeToString(bodyHash[:])
		mac := hmac.New(sha256.New, g.secret)
		_, _ = mac.Write([]byte(payload))
		expected := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(supplied), []byte(expected)) {
			writeError(w, http.StatusUnauthorized, "Tanda tangan BFF tidak valid")
			return
		}
		if !g.claimNonce(nonce, now) {
			writeError(w, http.StatusConflict, "Permintaan BFF terdeteksi berulang")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

func (g *signatureGuard) claimNonce(nonce string, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for key, expiry := range g.used {
		if !expiry.After(now) {
			delete(g.used, key)
		}
	}
	if _, exists := g.used[nonce]; exists || len(g.used) >= maxTrackedNonces {
		return false
	}
	g.used[nonce] = now.Add(nonceTTL)
	return true
}

func writeError(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": detail})
}
