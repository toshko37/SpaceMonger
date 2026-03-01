package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSettings_missingFile(t *testing.T) {
	_, err := loadSettings("/nonexistent/path/settings.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadSettings_valid(t *testing.T) {
	f, err := os.CreateTemp("", "settings*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	input := Settings{
		Port: 9999,
		Bind: "127.0.0.1",
		Auth: AuthSettings{Enabled: true, Password: "abc123"},
	}
	json.NewEncoder(f).Encode(input)
	f.Close()

	s, err := loadSettings(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Port != 9999 {
		t.Errorf("expected port 9999, got %d", s.Port)
	}
	if s.Auth.Password != "abc123" {
		t.Errorf("expected password abc123, got %s", s.Auth.Password)
	}
}

func TestGenPassword_length(t *testing.T) {
	p := genPassword()
	if len(p) != 6 {
		t.Errorf("expected 6 chars, got %d: %q", len(p), p)
	}
}

func TestGenPassword_alphanumeric(t *testing.T) {
	for i := 0; i < 100; i++ {
		p := genPassword()
		for _, c := range p {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
				t.Errorf("non-alphanumeric char %c in password %s", c, p)
			}
		}
	}
}

func TestScanner_singleFile(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello world") // 11 bytes
	os.WriteFile(filepath.Join(dir, "test.txt"), content, 0644)

	s := &Scanner{}
	s.Scan(dir)

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.root == nil {
		t.Fatal("expected root node, got nil")
	}
	if len(s.root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(s.root.Children))
	}
	if s.root.Children[0].Size != 11 {
		t.Errorf("expected size 11, got %d", s.root.Children[0].Size)
	}
	if s.files != 1 {
		t.Errorf("expected files=1, got %d", s.files)
	}
}

func TestScanner_dirSizeIsSum(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), make([]byte, 100), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), make([]byte, 200), 0644)

	s := &Scanner{}
	s.Scan(dir)

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.root.Size != 300 {
		t.Errorf("expected total size 300, got %d", s.root.Size)
	}
	if s.files != 2 {
		t.Errorf("expected files=2, got %d", s.files)
	}
}

func TestScanner_nested(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	os.Mkdir(sub, 0755)
	os.WriteFile(filepath.Join(sub, "file.txt"), make([]byte, 500), 0644)

	s := &Scanner{}
	s.Scan(dir)

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.root.Size != 500 {
		t.Errorf("expected total size 500, got %d", s.root.Size)
	}
	if s.dirs < 1 {
		t.Errorf("expected dirs >= 1, got %d", s.dirs)
	}
}

func TestParseMounts_realFSOnly(t *testing.T) {
	content := `sysfs /sys sysfs rw,nosuid 0 0
proc /proc proc rw,nosuid 0 0
/dev/sda1 / ext4 rw,relatime 0 0
/dev/sda2 /home ext4 rw,relatime 0 0
tmpfs /tmp tmpfs rw,nosuid 0 0
/dev/sdb1 /data xfs rw,relatime 0 0
`
	f, _ := os.CreateTemp("", "mounts*")
	f.WriteString(content)
	f.Close()
	defer os.Remove(f.Name())

	mounts, err := parseMountsFile(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mounts) != 3 { // ext4 x2 + xfs x1, skip sysfs/proc/tmpfs
		t.Errorf("expected 3 real mounts, got %d: %+v", len(mounts), mounts)
	}
	paths := map[string]bool{}
	for _, m := range mounts {
		paths[m.Path] = true
	}
	for _, want := range []string{"/", "/home", "/data"} {
		if !paths[want] {
			t.Errorf("missing mount path %s", want)
		}
	}
}

func TestParseMounts_filtersPseudo(t *testing.T) {
	content := `devpts /dev/pts devpts rw 0 0
tmpfs /run/user/1000 tmpfs rw 0 0
cgroup2 /sys/fs/cgroup cgroup2 rw 0 0
`
	f, _ := os.CreateTemp("", "mounts*")
	f.WriteString(content)
	f.Close()
	defer os.Remove(f.Name())

	mounts, err := parseMountsFile(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mounts) != 0 {
		t.Errorf("expected 0 real mounts, got %d", len(mounts))
	}
}

func TestAuthHandler_wrongPassword(t *testing.T) {
	settingsMu.Lock()
	globalSettings = Settings{Auth: AuthSettings{Enabled: true, Password: "secret"}}
	settingsMu.Unlock()

	body := `{"password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	authHandler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthHandler_correctPassword(t *testing.T) {
	settingsMu.Lock()
	globalSettings = Settings{Auth: AuthSettings{Enabled: true, Password: "secret"}}
	settingsMu.Unlock()

	body := `{"password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	authHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "sm_session" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected sm_session cookie in response")
	}
}

func TestAuthMiddleware_allowsWhenDisabled(t *testing.T) {
	settingsMu.Lock()
	globalSettings = Settings{Auth: AuthSettings{Enabled: false}}
	settingsMu.Unlock()

	called := false
	handler := authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if !called {
		t.Error("expected handler to be called when auth disabled")
	}
}

func TestAuthMiddleware_blocks401WhenEnabled(t *testing.T) {
	settingsMu.Lock()
	globalSettings = Settings{Auth: AuthSettings{Enabled: true, Password: "x"}}
	settingsMu.Unlock()

	handler := authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil) // no cookie
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
