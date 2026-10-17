package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthAndErrorsAreJSONWithSecurityHeaders(t *testing.T) {
	handler := NewHandler(nil, ServerConfig{ServiceName: "algentra-capital"})

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("health status = %d", w.Code)
	}
	var health map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &health); err != nil || health["status"] != "ok" {
		t.Fatalf("health response = %q, %v", w.Body.String(), err)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("security headers were not set")
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if w.Code != http.StatusNotFound || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("404 response = status %d, content-type %q, body %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/health", nil))
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("405 response = status %d, content-type %q", w.Code, w.Header().Get("Content-Type"))
	}
}

func TestReadinessDoesNotExposeDatabaseErrors(t *testing.T) {
	handler := NewHandler(nil, ServerConfig{ServiceName: "algentra-capital"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if w.Code != http.StatusServiceUnavailable || w.Body.String() != "{\"checks\":{\"database\":\"unavailable\"},\"status\":\"not_ready\"}\n" {
		t.Fatalf("readiness response = %d %q", w.Code, w.Body.String())
	}
}

func TestProductionAddsHSTS(t *testing.T) {
	handler := NewHandler(nil, ServerConfig{ServiceName: "algentra-capital", CookieSecure: true})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Header().Get("Strict-Transport-Security") != "max-age=31536000; includeSubDomains" {
		t.Fatal("production HSTS header missing")
	}
}

func TestAnonymousSessionReturnsCSRFTokenWithoutPersistingCookie(t *testing.T) {
	handler := NewHandler(nil, ServerConfig{ServiceName: "algentra-capital", AppSecretKey: "test-secret-with-at-least-thirty-two-bytes"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/auth/session", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("session status = %d, body %q", w.Code, w.Body.String())
	}
	var payload struct {
		Admin     bool   `json:"admin"`
		Client    bool   `json:"client"`
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Admin || payload.Client || len(payload.CSRFToken) < 32 {
		t.Fatalf("unexpected anonymous session payload: %#v", payload)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("anonymous CSRF lookup should not create an authenticated session cookie")
	}
}
