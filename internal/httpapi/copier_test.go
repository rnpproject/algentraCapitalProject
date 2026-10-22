package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCopierHealthRequiresWorkerKeyAndDisablesCache(t *testing.T) {
	handler := NewHandler(nil, ServerConfig{ServiceName: "algentra-capital", WorkerAPIKey: "a-worker-key-long-enough-to-be-secure-123"})
	request := httptest.NewRequest(http.MethodGet, "/api/mt5/copier/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing worker key status = %d", w.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/mt5/copier/health", nil)
	request.Header.Set("X-Copier-Worker-Key", "a-worker-key-long-enough-to-be-secure-123")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("authorized health response = %d cache=%q body=%q", w.Code, w.Header().Get("Cache-Control"), w.Body.String())
	}
}

func TestCopierWorkerKeyNotConfigured(t *testing.T) {
	handler := NewHandler(nil, ServerConfig{ServiceName: "algentra-capital"})
	request := httptest.NewRequest(http.MethodGet, "/api/mt5/copier/health", nil)
	request.Header.Set("X-Copier-Worker-Key", "some-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured worker health status = %d", w.Code)
	}
}
