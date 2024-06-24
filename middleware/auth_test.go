package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthorize_ValidToken(t *testing.T) {
	validTokens := map[string]struct{}{
		"token1": {},
		"token2": {},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authorizeHandler := Authorize(handler, validTokens)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Auth-Token", "token1")
	rec := httptest.NewRecorder()

	authorizeHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestAuthorize_InvalidToken(t *testing.T) {
	validTokens := map[string]struct{}{
		"token1": {},
		"token2": {},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	})

	authorizeHandler := Authorize(handler, validTokens)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Auth-Token", "invalid-token")
	rec := httptest.NewRecorder()

	authorizeHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Expected status code %d, got %d", http.StatusForbidden, rec.Code)
	}
}
