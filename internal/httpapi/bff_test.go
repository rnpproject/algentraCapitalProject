package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func signedRequest(t *testing.T, secret, nonce, path, body string) *http.Request {
	t.Helper()
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	bodyHash := sha256.Sum256([]byte(body))
	payload := stamp + "\n" + nonce + "\nPOST\n" + path + "\n" + hex.EncodeToString(bodyHash[:])
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("X-Algentra-Timestamp", stamp)
	r.Header.Set("X-Algentra-Nonce", nonce)
	r.Header.Set("X-Algentra-Signature", hex.EncodeToString(mac.Sum(nil)))
	return r
}

func TestBFFSignatureVerifiesBodyAndRejectsReplay(t *testing.T) {
	const secret = "a-random-bff-secret-at-least-thirty-two-characters"
	var gotBody string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	})
	handler := BFFSignature(secret, next)
	request := signedRequest(t, secret, "nonce-value-at-least-16", "/v1/auth/login", `{"email":"user@example.com"}`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || gotBody != `{"email":"user@example.com"}` {
		t.Fatalf("signed request result: status=%d body=%q", response.Code, gotBody)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, signedRequest(t, secret, "nonce-value-at-least-16", "/v1/auth/login", `{"email":"user@example.com"}`))
	if response.Code != http.StatusConflict {
		t.Fatalf("replayed nonce status = %d, want %d", response.Code, http.StatusConflict)
	}
}

func TestBFFSignatureDoesNotGateLegacyOrMidtransWebhook(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, path := range []string{"/api/auth/session", "/v1/payments/midtrans/notification"} {
		r := httptest.NewRequest(http.MethodPost, path, nil)
		w := httptest.NewRecorder()
		BFFSignature("secret", next).ServeHTTP(w, r)
		if w.Code != http.StatusNoContent {
			t.Fatalf("%s returned %d", path, w.Code)
		}
	}
}
