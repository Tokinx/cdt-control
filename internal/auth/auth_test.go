package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"ecs-cdt-panel/internal/config"
)

func TestLoginAndRateLimit(t *testing.T) {
	cfg := &config.Config{
		PanelUsername: "admin",
		PanelPassword: "super-secret-password",
		SessionSecret: "0123456789abcdef0123456789abcdef",
	}
	mgr := NewManager(cfg)

	// Wrong password
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"wrong"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	mgr.HandleLogin(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	// Correct login
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"super-secret-password"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	mgr.HandleLogin(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookieName {
		t.Fatalf("expected session cookie, got %+v", cookies)
	}

	// Protected endpoint check
	protected := mgr.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	authReq := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	authReq.AddCookie(cookies[0])
	authRec := httptest.NewRecorder()
	protected(authRec, authReq)
	if authRec.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid cookie, got %d", authRec.Code)
	}

	// Brute-force lockout from another IP
	for i := 0; i < maxFailures; i++ {
		badReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"bad"}`))
		badReq.RemoteAddr = "10.0.0.5:9999"
		badRec := httptest.NewRecorder()
		mgr.HandleLogin(badRec, badReq)
		if badRec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d expected 401, got %d", i, badRec.Code)
		}
	}
	blockedReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"super-secret-password"}`))
	blockedReq.RemoteAddr = "10.0.0.5:9999"
	blockedRec := httptest.NewRecorder()
	mgr.HandleLogin(blockedRec, blockedReq)
	if blockedRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 rate limit, got %d", blockedRec.Code)
	}
}
