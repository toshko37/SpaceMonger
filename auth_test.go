package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func setAuth(enabled bool, password string) {
	setSettings(Settings{Auth: AuthSettings{Enabled: enabled, Password: password}})
	limiter.reset("192.0.2.1")
}

func postLogin(password, ip string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/auth", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip + ":12345"
	w := httptest.NewRecorder()
	authHandler(w, req)
	return w
}

func sessionCookieFrom(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			return c
		}
	}
	return nil
}

func TestAuthHandler_wrongPassword(t *testing.T) {
	setAuth(true, "secret")
	if w := postLogin("wrong", "192.0.2.1"); w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthHandler_correctPasswordSetsCookie(t *testing.T) {
	setAuth(true, "secret")
	w := postLogin("secret", "192.0.2.1")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	c := sessionCookieFrom(w)
	if c == nil || !c.HttpOnly {
		t.Fatalf("expected HttpOnly session cookie, got %+v", c)
	}
	if !sessionValid(c.Value) {
		t.Error("session should be valid after login")
	}
}

func TestAuthHandler_emptyConfiguredPasswordNeverMatches(t *testing.T) {
	setAuth(true, "")
	if w := postLogin("", "192.0.2.1"); w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty password, got %d", w.Code)
	}
}

func TestAuthHandler_rateLimitAfterFailures(t *testing.T) {
	setAuth(true, "secret")
	ip := "192.0.2.1"
	for i := 0; i < maxLoginFails; i++ {
		if w := postLogin("wrong", ip); w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i, w.Code)
		}
	}
	w := postLogin("secret", ip) // correct password but locked out
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 after %d failures, got %d", maxLoginFails, w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
	// Other IPs are unaffected.
	if w2 := postLogin("secret", "192.0.2.2"); w2.Code != http.StatusOK {
		t.Errorf("other IP should not be limited, got %d", w2.Code)
	}
	limiter.reset(ip)
}

func TestAuthHandler_statusGET(t *testing.T) {
	setAuth(true, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/auth", nil)
	w := httptest.NewRecorder()
	authHandler(w, req)
	var st map[string]bool
	json.Unmarshal(w.Body.Bytes(), &st)
	if !st["authRequired"] || st["authenticated"] {
		t.Errorf("unexpected status %+v", st)
	}
}

func TestLogout_invalidatesSession(t *testing.T) {
	setAuth(true, "secret")
	c := sessionCookieFrom(postLogin("secret", "192.0.2.1"))
	req := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	req.AddCookie(c)
	w := httptest.NewRecorder()
	logoutHandler(w, req)
	if sessionValid(c.Value) {
		t.Error("session should be invalid after logout")
	}
}

func TestSession_expires(t *testing.T) {
	token := createSession()
	sessionMu.Lock()
	sessions[token] = time.Now().Add(-time.Second)
	sessionMu.Unlock()
	if sessionValid(token) {
		t.Error("expired session should be invalid")
	}
}

func TestAuthMiddleware_allowsWhenDisabled(t *testing.T) {
	setAuth(false, "")
	called := false
	h := authMiddleware(func(w http.ResponseWriter, r *http.Request) { called = true })
	h(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/data", nil))
	if !called {
		t.Error("expected handler to be called when auth disabled")
	}
}

func TestAuthMiddleware_blocks401WhenEnabled(t *testing.T) {
	setAuth(true, "x")
	h := authMiddleware(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodGet, "/api/data", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_allowsValidCookie(t *testing.T) {
	setAuth(true, "secret")
	c := sessionCookieFrom(postLogin("secret", "192.0.2.1"))
	h := authMiddleware(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.AddCookie(c)
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid cookie, got %d", w.Code)
	}
}
