package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestCSRFTokensAndAdminSession(t *testing.T) {
	state := &sessionState{Values: make(map[string]any)}
	first, err := state.CSRFToken()
	if err != nil || len(first) < 32 {
		t.Fatalf("CSRF token = %q, %v", first, err)
	}
	second, err := state.CSRFToken()
	if err != nil || first != second {
		t.Fatal("CSRF token should remain stable for the session")
	}
	state.SetAdmin("admin")
	if state.Values["admin_username"] != "admin" {
		t.Fatal("admin identity missing from session")
	}
	csrf, err := state.CSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/auth/logout", nil)
	r.Header.Set("X-CSRF-Token", csrf)
	if !state.RequireCSRF(r) {
		t.Fatal("correct CSRF token rejected")
	}
	r.Header.Set("X-CSRF-Token", "incorrect")
	if state.RequireCSRF(r) {
		t.Fatal("incorrect CSRF token accepted")
	}
}

func TestSessionIdentity(t *testing.T) {
	if kind, id := sessionIdentity(map[string]any{"admin_username": "root"}); kind != "admin" || id != "root" {
		t.Fatalf("admin identity = %q, %q", kind, id)
	}
	if kind, id := sessionIdentity(map[string]any{"client_user_id": float64(42)}); kind != "client" || id != "42" {
		t.Fatalf("client identity = %q, %q", kind, id)
	}
	if kind, _ := sessionIdentity(map[string]any{}); kind != "" {
		t.Fatalf("anonymous identity = %q", kind)
	}
}
