package auth

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"ecs-cdt-panel/internal/config"
	"ecs-cdt-panel/internal/crypto"
)

const (
	sessionCookieName = "cdt_session"
	sessionTTL        = 7 * 24 * time.Hour
	maxFailures       = 8
	failureWindow     = 10 * time.Minute
	lockDuration      = 5 * time.Minute
)

type failRecord struct {
	count       int
	firstFailed time.Time
	lockedUntil time.Time
}

// Manager handles cookie-based login, verification, and brute-force rate limiting.
type Manager struct {
	cfg      *config.Config
	mu       sync.Mutex
	failures map[string]*failRecord
}

// NewManager creates an auth Manager.
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg:      cfg,
		failures: make(map[string]*failRecord),
	}
}

// HandleLogin verifies PANEL_USERNAME / PANEL_PASSWORD and sets an HttpOnly session cookie.
func (m *Manager) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式无效"})
		return
	}

	ip := clientIP(r)
	if waitSec, blocked := m.isBlocked(ip); blocked {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "登录尝试过于频繁，请稍后再试",
			"retry": time.Duration(waitSec * int64(time.Second)).String(),
		})
		return
	}

	userOK := crypto.VerifyUsername(strings.TrimSpace(body.Username), m.cfg.PanelUsername)
	passOK := crypto.VerifyPassword(body.Password, m.cfg.PanelPassword, m.cfg.PanelPasswordHash)
	if !userOK || !passOK {
		m.recordFailure(ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "用户名或密码错误"})
		return
	}

	m.clearFailures(ip)

	token, err := crypto.SignToken(m.cfg.SessionSecret, m.cfg.PanelUsername, sessionTTL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "生成会话失败"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   m.cfg.CookieSecure,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"username":      m.cfg.PanelUsername,
	})
}

// HandleLogout clears the session cookie.
func (m *Manager) HandleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   m.cfg.CookieSecure,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HandleMe checks current session status.
func (m *Manager) HandleMe(w http.ResponseWriter, r *http.Request) {
	username, ok := m.AuthenticateRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"username":      username,
	})
}

// AuthenticateRequest verifies the session cookie and returns the authenticated username.
func (m *Manager) AuthenticateRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return crypto.VerifyToken(m.cfg.SessionSecret, cookie.Value)
}

// RequireAuth protects API endpoints from unauthenticated callers.
func (m *Manager) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := m.AuthenticateRequest(r); !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未登录或登录已过期"})
			return
		}
		next(w, r)
	}
}

func (m *Manager) isBlocked(ip string) (int64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	rec, ok := m.failures[ip]
	if !ok {
		return 0, false
	}
	if !rec.lockedUntil.IsZero() && now.Before(rec.lockedUntil) {
		return int64(rec.lockedUntil.Sub(now).Seconds()) + 1, true
	}
	if now.Sub(rec.firstFailed) > failureWindow {
		delete(m.failures, ip)
	}
	return 0, false
}

func (m *Manager) recordFailure(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	// Bound map size in memory.
	if len(m.failures) > 1000 {
		for k, v := range m.failures {
			if now.Sub(v.firstFailed) > failureWindow && now.After(v.lockedUntil) {
				delete(m.failures, k)
			}
		}
	}
	rec, ok := m.failures[ip]
	if !ok || now.Sub(rec.firstFailed) > failureWindow {
		m.failures[ip] = &failRecord{count: 1, firstFailed: now}
		return
	}
	rec.count++
	if rec.count >= maxFailures {
		rec.lockedUntil = now.Add(lockDuration)
	}
}

func (m *Manager) clearFailures(ip string) {
	m.mu.Lock()
	delete(m.failures, ip)
	m.mu.Unlock()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
