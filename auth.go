package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	sessionCookie = "sm_session"
	sessionTTL    = 24 * time.Hour

	// Brute-force protection: after maxLoginFails failed attempts from one
	// IP, further attempts are rejected for loginLockout.
	maxLoginFails = 5
	loginLockout  = 60 * time.Second
)

// ─── Sessions ─────────────────────────────────────────────────────────────────

var (
	sessions  = make(map[string]time.Time) // token → expiry
	sessionMu sync.RWMutex
)

func genToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	return hex.EncodeToString(b)
}

func createSession() string {
	token := genToken()
	now := time.Now()
	sessionMu.Lock()
	for t, exp := range sessions { // lazy sweep of expired sessions
		if now.After(exp) {
			delete(sessions, t)
		}
	}
	sessions[token] = now.Add(sessionTTL)
	sessionMu.Unlock()
	return token
}

func sessionValid(token string) bool {
	sessionMu.RLock()
	exp, ok := sessions[token]
	sessionMu.RUnlock()
	return ok && time.Now().Before(exp)
}

func deleteSession(token string) {
	sessionMu.Lock()
	delete(sessions, token)
	sessionMu.Unlock()
}

func isAuthenticated(r *http.Request) bool {
	if !getSettings().Auth.Enabled {
		return true
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	return sessionValid(c.Value)
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// ─── Login rate limiting ──────────────────────────────────────────────────────

type failRecord struct {
	count   int
	lockedU time.Time
}

type loginLimiter struct {
	mu    sync.Mutex
	fails map[string]*failRecord
}

var limiter = &loginLimiter{fails: make(map[string]*failRecord)}

// retryAfter returns how long the client must wait, or 0 if allowed.
func (l *loginLimiter) retryAfter(ip string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	rec, ok := l.fails[ip]
	if !ok {
		return 0
	}
	if d := time.Until(rec.lockedU); d > 0 {
		return d
	}
	return 0
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	rec := l.fails[ip]
	if rec == nil {
		rec = &failRecord{}
		l.fails[ip] = rec
	}
	rec.count++
	if rec.count >= maxLoginFails {
		rec.lockedU = time.Now().Add(loginLockout)
		rec.count = 0
	}
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	delete(l.fails, ip)
	l.mu.Unlock()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// authHandler: GET → auth status; POST {password} → login.
func authHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]bool{
			"authRequired":  getSettings().Auth.Enabled,
			"authenticated": isAuthenticated(r),
		})
	case http.MethodPost:
		loginHandler(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if wait := limiter.retryAfter(ip); wait > 0 {
		secs := int(wait.Seconds()) + 1
		w.Header().Set("Retry-After", fmt.Sprint(secs))
		writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
			"error": "too many attempts", "retryAfter": secs,
		})
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}

	want := getSettings().Auth.Password
	if want == "" || subtle.ConstantTimeCompare([]byte(req.Password), []byte(want)) != 1 {
		limiter.fail(ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "wrong password"})
		return
	}
	limiter.reset(ip)

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    createSession(),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		deleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
