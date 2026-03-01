# SpaceMonger Linux — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a web-based disk space visualizer for Linux as a single Go binary on port 4322, displaying the filesystem as a squarified treemap with SpaceMonger-style green→yellow→red color coding. Installable via `curl | bash`.

**Architecture:** Single Go binary with `//go:embed static` for HTML/CSS/JS + D3.js v7. REST + SSE API for real-time filesystem scanning. Auth controlled via `settings.json`. Deployed as a systemd service.

**Tech Stack:** Go 1.21+, D3.js v7 (embedded offline), Vanilla JS, HTML5/CSS3, systemd, bash install scripts.

---

## Task 1: Project Scaffold

**Files:**
- Create: `go.mod`
- Create: `static/index.html` (placeholder — replaced in Task 8)
- Create: `.gitignore`
- Create: `settings.example.json`

**Step 1: Check Go is available**

```bash
go version
```
Expected: `go version go1.21` or higher. If missing: `apt-get install golang-go` or `snap install go --classic`

**Step 2: Initialize module**

```bash
cd /var/www/spacemonger
go mod init spacemonger
```
Expected: creates `go.mod` with `module spacemonger`

**Step 3: Create directory structure**

```bash
mkdir -p static .github/workflows
```

**Step 4: Create placeholder static/index.html** (needed for `//go:embed static` to compile)

Create `static/index.html`:
```html
<!DOCTYPE html><html><body>Loading...</body></html>
```

**Step 5: Create .gitignore**

Create `.gitignore`:
```
settings.json
spacemonger
spacemonger-linux-amd64
spacemonger-linux-arm64
spacemonger-linux-arm
*.exe
.DS_Store
```

**Step 6: Create settings.example.json**

Create `settings.example.json`:
```json
{
  "port": 4322,
  "bind": "0.0.0.0",
  "auth": {
    "enabled": false,
    "password": "changeme"
  }
}
```

**Step 7: Commit**

```bash
git init
git add go.mod .gitignore settings.example.json static/index.html
git commit -m "feat: initialize SpaceMonger Linux project"
```

---

## Task 2: Settings + GenPassword (TDD)

**Files:**
- Create: `main.go` (settings section only — we'll grow it task by task)
- Create: `main_test.go`

**Step 1: Write failing tests**

Create `main_test.go`:
```go
package main

import (
	"encoding/json"
	"os"
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
```

**Step 2: Run — expect compile failure**

```bash
go test ./... -run "TestLoad|TestGenPass" -v 2>&1 | head -20
```
Expected: `undefined: loadSettings` (or similar compile error)

**Step 3: Create main.go with settings**

Create `main.go`:
```go
package main

import (
	"bufio"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed static
var staticFiles embed.FS

// ─── Settings ─────────────────────────────────────────────────────────────────

type Settings struct {
	Port int          `json:"port"`
	Bind string       `json:"bind"`
	Auth AuthSettings `json:"auth"`
}

type AuthSettings struct {
	Enabled  bool   `json:"enabled"`
	Password string `json:"password"`
}

var (
	globalSettings Settings
	settingsMu     sync.RWMutex
)

func getSettings() Settings {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	return globalSettings
}

func loadSettings(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

func genPassword() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	rand.Read(b)
	for i, v := range b {
		b[i] = chars[int(v)%len(chars)]
	}
	return string(b)
}

// ─── Stubs for compile ────────────────────────────────────────────────────────
// (will be replaced in subsequent tasks)

type FileNode struct{}
type Scanner struct{ mu sync.RWMutex }
type Mount struct{}

var globalScanner = &Scanner{}

func getMounts() ([]Mount, error)          { return nil, nil }
func parseMountsFile(string) ([]Mount, error) { return nil, nil }

var (
	sessions  = make(map[string]struct{})
	sessionMu sync.RWMutex
)

func genToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func main() {
	fmt.Println("SpaceMonger (stub)")
	_ = bufio.NewScanner(nil)
	_ = filepath.Base("")
	_ = strings.Fields("")
	_ = time.Now()
	_ = syscall.Statfs_t{}
	_ = fs.FS(nil)
	_ = http.NewServeMux()
	_ = log.Writer()
}
```

**Step 4: Run — expect PASS**

```bash
go test ./... -run "TestLoad|TestGenPass" -v
```
Expected: 4 tests PASS

**Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: settings load/save and password generator with tests"
```

---

## Task 3: Filesystem Scanner (TDD)

**Files:**
- Modify: `main.go` (replace FileNode + Scanner stubs with real implementation)
- Modify: `main_test.go` (add scanner tests)

**Step 1: Write failing scanner tests**

Add to `main_test.go` (after existing tests):
```go
import (
	// add these to the existing import block:
	"path/filepath"
)

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
```

**Step 2: Run — expect compile failure**

```bash
go test ./... -run TestScanner -v 2>&1 | head -10
```
Expected: error because `Scanner` has no `Scan` method or `root` field

**Step 3: Replace FileNode + Scanner stubs in main.go**

Replace the stub section (the `type FileNode struct{}` through `var globalScanner = &Scanner{}` lines) with:
```go
// ─── File Tree ────────────────────────────────────────────────────────────────

type FileNode struct {
	Name     string      `json:"name"`
	Size     int64       `json:"size"`
	Mtime    time.Time   `json:"mtime"`
	IsDir    bool        `json:"isDir"`
	Children []*FileNode `json:"children,omitempty"`
}

// ─── Scanner ──────────────────────────────────────────────────────────────────

type Scanner struct {
	mu        sync.RWMutex
	root      *FileNode
	files     int64
	dirs      int64
	current   string
	done      bool
	scanErr   error
	totalDisk int64
	freeDisk  int64
}

var globalScanner = &Scanner{}

func (s *Scanner) Scan(path string) {
	s.mu.Lock()
	s.root = nil
	s.files = 0
	s.dirs = 0
	s.current = path
	s.done = false
	s.scanErr = nil
	s.totalDisk = 0
	s.freeDisk = 0
	s.mu.Unlock()

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err == nil {
		s.mu.Lock()
		s.totalDisk = int64(stat.Blocks) * int64(stat.Bsize)
		s.freeDisk = int64(stat.Bavail) * int64(stat.Bsize)
		s.mu.Unlock()
	}

	root, err := s.scanDir(path)

	s.mu.Lock()
	s.root = root
	s.done = true
	s.scanErr = err
	s.mu.Unlock()
}

func (s *Scanner) scanDir(path string) (*FileNode, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}

	node := &FileNode{
		Name:  filepath.Base(path),
		Mtime: info.ModTime(),
		IsDir: info.IsDir(),
	}

	if !info.IsDir() {
		node.Size = info.Size()
		s.mu.Lock()
		s.files++
		s.mu.Unlock()
		return node, nil
	}

	s.mu.Lock()
	s.dirs++
	s.current = path
	s.mu.Unlock()

	entries, err := os.ReadDir(path)
	if err != nil {
		return node, nil // return empty dir on permission error — don't abort scan
	}

	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue // skip symlinks to avoid loops and cross-filesystem traversal
		}
		child, err := s.scanDir(filepath.Join(path, entry.Name()))
		if err != nil {
			continue
		}
		node.Children = append(node.Children, child)
		node.Size += child.Size
	}

	return node, nil
}
```

**Step 4: Run — expect PASS**

```bash
go test ./... -run TestScanner -v
```
Expected: 3 tests PASS

**Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: recursive filesystem scanner with size aggregation"
```

---

## Task 4: Mountpoints Parser (TDD)

**Files:**
- Modify: `main.go` (replace Mount + getMounts stubs)
- Modify: `main_test.go` (add mount tests)

**Step 1: Write failing mount tests**

Add to `main_test.go`:
```go
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
```

**Step 2: Run — expect compile failure**

```bash
go test ./... -run TestParseMounts -v 2>&1 | head -10
```
Expected: error: `Mount` has no fields / `parseMountsFile` is stub

**Step 3: Replace Mount stubs in main.go**

Replace the `type Mount struct{}` stub and `getMounts`/`parseMountsFile` stubs with:
```go
// ─── Mounts ───────────────────────────────────────────────────────────────────

type Mount struct {
	Device string `json:"device"`
	Path   string `json:"path"`
	FSType string `json:"fstype"`
	Total  int64  `json:"total"`
	Free   int64  `json:"free"`
}

var realFSTypes = map[string]bool{
	"ext4":     true,
	"ext3":     true,
	"ext2":     true,
	"xfs":      true,
	"btrfs":    true,
	"zfs":      true,
	"ntfs":     true,
	"vfat":     true,
	"fat32":    true,
	"f2fs":     true,
	"reiserfs": true,
	"jfs":      true,
	"fuseblk":  true,
}

func parseMountsFile(path string) ([]Mount, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var mounts []Mount
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		device, mountPath, fstype := fields[0], fields[1], fields[2]
		if !realFSTypes[fstype] {
			continue
		}
		m := Mount{Device: device, Path: mountPath, FSType: fstype}
		var stat syscall.Statfs_t
		if err := syscall.Statfs(mountPath, &stat); err == nil {
			m.Total = int64(stat.Blocks) * int64(stat.Bsize)
			m.Free = int64(stat.Bavail) * int64(stat.Bsize)
		}
		mounts = append(mounts, m)
	}
	return mounts, sc.Err()
}

func getMounts() ([]Mount, error) {
	return parseMountsFile("/proc/mounts")
}
```

**Step 4: Run — expect PASS**

```bash
go test ./... -run TestParseMounts -v
```
Expected: 2 tests PASS

**Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: mountpoint parser reads /proc/mounts, filters real filesystems"
```

---

## Task 5: Auth System (TDD)

**Files:**
- Modify: `main.go` (add auth handlers + middleware)
- Modify: `main_test.go` (add auth tests)

**Step 1: Write failing auth tests**

Add to `main_test.go`:
```go
import (
	// add to existing imports:
	"bytes"
	"net/http"
	"net/http/httptest"
)

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
```

**Step 2: Run — expect compile failure**

```bash
go test ./... -run TestAuth -v 2>&1 | head -10
```
Expected: `authHandler undefined`, `authMiddleware undefined`

**Step 3: Add auth system to main.go**

Add after the `getMounts` function:
```go
// ─── Auth / Sessions ──────────────────────────────────────────────────────────

func isAuthenticated(r *http.Request) bool {
	cfg := getSettings()
	if !cfg.Auth.Enabled {
		return true
	}
	cookie, err := r.Cookie("sm_session")
	if err != nil {
		return false
	}
	sessionMu.RLock()
	defer sessionMu.RUnlock()
	_, ok := sessions[cookie.Value]
	return ok
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func authHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	cfg := getSettings()
	if req.Password != cfg.Auth.Password {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	token := genToken()
	sessionMu.Lock()
	sessions[token] = struct{}{}
	sessionMu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "sm_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
```

**Step 4: Run — expect PASS**

```bash
go test ./... -run TestAuth -v
```
Expected: 4 tests PASS

**Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: session auth with cookie, middleware, enable/disable toggle"
```

---

## Task 6: HTTP Handlers + main() (complete the backend)

**Files:**
- Modify: `main.go` (add all HTTP handlers + replace stub main())

**Step 1: Add remaining handlers + replace main()**

Replace the stub `main()` in `main.go` with the full version. Add the handlers and main function. The complete addition after `authHandler`:

```go
// ─── HTTP Handlers ────────────────────────────────────────────────────────────

func mountsHandler(w http.ResponseWriter, r *http.Request) {
	mounts, err := getMounts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mounts)
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	globalScanner.mu.RLock()
	root := globalScanner.root
	files := globalScanner.files
	dirs := globalScanner.dirs
	total := globalScanner.totalDisk
	free := globalScanner.freeDisk
	globalScanner.mu.RUnlock()

	if root == nil {
		http.Error(w, "no scan data available", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"root":      root,
		"files":     files,
		"dirs":      dirs,
		"totalDisk": total,
		"freeDisk":  free,
	})
}

func scanHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path parameter required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	done := make(chan struct{})
	go func() {
		globalScanner.Scan(path)
		close(done)
	}()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	sendProgress := func() bool {
		globalScanner.mu.RLock()
		isDone := globalScanner.done
		files := globalScanner.files
		dirs := globalScanner.dirs
		current := globalScanner.current
		root := globalScanner.root
		total := globalScanner.totalDisk
		free := globalScanner.freeDisk
		globalScanner.mu.RUnlock()

		var data []byte
		if isDone {
			data, _ = json.Marshal(map[string]interface{}{
				"status":    "done",
				"files":     files,
				"dirs":      dirs,
				"root":      root,
				"totalDisk": total,
				"freeDisk":  free,
			})
		} else {
			data, _ = json.Marshal(map[string]interface{}{
				"status":  "scanning",
				"files":   files,
				"dirs":    dirs,
				"current": current,
			})
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		return isDone
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-done:
			sendProgress()
			return
		case <-ticker.C:
			if sendProgress() {
				return
			}
		}
	}
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	settingsPath := "settings.json"
	cfg, err := loadSettings(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			cfg = Settings{
				Port: 4322,
				Bind: "0.0.0.0",
				Auth: AuthSettings{
					Enabled:  false,
					Password: genPassword(),
				},
			}
			data, _ := json.MarshalIndent(cfg, "", "  ")
			if writeErr := os.WriteFile(settingsPath, data, 0600); writeErr != nil {
				log.Printf("warning: could not write settings.json: %v", writeErr)
			}
			log.Printf("Created settings.json — generated password: %s", cfg.Auth.Password)
		} else {
			log.Fatalf("failed to load settings: %v", err)
		}
	}

	settingsMu.Lock()
	globalSettings = cfg
	settingsMu.Unlock()

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("failed to access embedded static files: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("/api/auth", authHandler)
	mux.HandleFunc("/api/mounts", authMiddleware(mountsHandler))
	mux.HandleFunc("/api/scan", authMiddleware(scanHandler))
	mux.HandleFunc("/api/data", authMiddleware(dataHandler))

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)
	log.Printf("SpaceMonger running at http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
```

Also remove the stub `main()` that has `fmt.Println("SpaceMonger (stub)")`.

**Step 2: Run all tests**

```bash
go test ./... -v
```
Expected: all tests PASS (7+ tests)

**Step 3: Test that it builds**

```bash
go build -o /tmp/sm-test . && echo "Build OK" && rm /tmp/sm-test
```
Expected: `Build OK`

**Step 4: Commit**

```bash
git add main.go
git commit -m "feat: complete HTTP server with scan/mounts/data/auth endpoints"
```

---

## Task 7: Download D3.js v7 (embed it offline)

**Files:**
- Create: `static/d3.v7.min.js`

**Step 1: Download D3.js v7 minified**

```bash
curl -fsSL https://cdn.jsdelivr.net/npm/d3@7/dist/d3.min.js -o static/d3.v7.min.js
```

**Step 2: Verify download**

```bash
wc -c static/d3.v7.min.js
head -c 100 static/d3.v7.min.js
```
Expected: size ~270KB, starts with `// https://d3js.org/d3/ v7...` or similar

**Step 3: Test build still works with new file**

```bash
go build -o /tmp/sm-test . && echo "Build OK" && rm /tmp/sm-test
```
Expected: `Build OK`

**Step 4: Commit**

```bash
git add static/d3.v7.min.js
git commit -m "feat: embed D3.js v7 for offline treemap rendering"
```

---

## Task 8: index.html (complete frontend skeleton)

**Files:**
- Modify: `static/index.html` (replace placeholder with full HTML)

**Step 1: Write static/index.html**

Overwrite `static/index.html` with:
```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>SpaceMonger</title>
    <link rel="stylesheet" href="style.css">
</head>
<body>

<!-- ─── Login overlay (shown when auth.enabled=true) ─────────────────────── -->
<div id="login-overlay">
    <div class="login-box">
        <h2>SpaceMonger</h2>
        <input type="password" id="login-password"
               placeholder="Enter password" autocomplete="current-password">
        <button id="login-btn">Login</button>
        <div id="login-error" class="login-error"></div>
    </div>
</div>

<!-- ─── Main application ──────────────────────────────────────────────────── -->
<div id="app">

    <!-- Title bar (Windows-style blue bar) -->
    <div id="titlebar">SpaceMonger — Disk Space Analyzer</div>

    <!-- Toolbar -->
    <div id="toolbar">
        <button id="btn-open"       title="Open a drive or partition">&#128194; Open</button>
        <button id="btn-reload"     title="Reload / rescan current path">&#8635; Reload</button>
        <div class="toolbar-sep"></div>
        <button id="btn-zoom-full"  title="Zoom to full view" disabled>&#8862; Full</button>
        <button id="btn-zoom-in"    title="Zoom into selected folder" disabled>&#43; In</button>
        <button id="btn-zoom-out"   title="Zoom out one level" disabled>&#8722; Out</button>
        <div class="toolbar-spacer"></div>
        <button id="btn-free-space" title="Show free space proportionally">Free Space</button>
    </div>

    <!-- Breadcrumb navigation -->
    <div id="breadcrumb"></div>

    <!-- Treemap drawing area -->
    <div id="treemap-container"></div>

    <!-- Status bar -->
    <div id="statusbar">
        <span>Files: <span id="status-files">—</span></span>
        <span>Dirs: <span id="status-dirs">—</span></span>
        <span id="status-space" style="display:none">
            Used: <span id="status-used">—</span>
            &nbsp;|&nbsp;
            Free: <span id="status-free">—</span>
        </span>
    </div>

</div>

<!-- ─── Scan progress overlay ─────────────────────────────────────────────── -->
<div id="progress-overlay">
    <div class="progress-box">
        <h3>Scanning filesystem...</h3>
        <p>
            Files: <span id="progress-files">0</span>
            &nbsp;&nbsp;
            Folders: <span id="progress-dirs">0</span>
        </p>
        <p class="progress-path" id="progress-current"></p>
    </div>
</div>

<!-- ─── Open drive modal ──────────────────────────────────────────────────── -->
<div id="open-modal" class="modal-overlay">
    <div class="modal-box">
        <div class="modal-header">
            <h3>Select Drive or Partition</h3>
            <button id="close-open-modal">&#10005;</button>
        </div>
        <div class="modal-body" id="mounts-list"></div>
    </div>
</div>

<script src="d3.v7.min.js"></script>
<script src="app.js"></script>
</body>
</html>
```

**Step 2: Verify build**

```bash
go build -o /tmp/sm-test . && echo "Build OK" && rm /tmp/sm-test
```

**Step 3: Commit**

```bash
git add static/index.html
git commit -m "feat: complete HTML structure with toolbar, treemap area, modals"
```

---

## Task 9: style.css (full Windows-classic styling)

**Files:**
- Create: `static/style.css`

**Step 1: Create static/style.css**

Create `static/style.css` with the complete stylesheet:
```css
/* ─── Reset & Base ─────────────────────────────────────────────────────────── */
*, *::before, *::after {
    box-sizing: border-box;
    margin: 0;
    padding: 0;
}

html, body {
    height: 100%;
    overflow: hidden;
    font-family: 'Courier New', Courier, monospace;
    font-size: 12px;
}

/* ─── Login Overlay ─────────────────────────────────────────────────────────── */
#login-overlay {
    position: fixed;
    inset: 0;
    background: #008080;
    display: none;           /* shown by JS when auth required */
    align-items: center;
    justify-content: center;
    z-index: 900;
}

.login-box {
    background: #d4d0c8;
    border: 2px solid;
    border-color: #ffffff #808080 #808080 #ffffff;
    padding: 28px 36px;
    min-width: 260px;
    box-shadow: 4px 4px 10px rgba(0, 0, 0, 0.5);
}

.login-box h2 {
    font-size: 14px;
    color: #000080;
    text-align: center;
    margin-bottom: 18px;
}

.login-box input {
    display: block;
    width: 100%;
    padding: 4px 6px;
    font-size: 12px;
    font-family: inherit;
    border: 1px solid;
    border-color: #808080 #ffffff #ffffff #808080;
    background: #ffffff;
    margin-bottom: 8px;
}

.login-box button {
    display: block;
    width: 100%;
    padding: 5px;
    font-size: 12px;
    font-family: inherit;
    background: #d4d0c8;
    border: 1px solid;
    border-color: #ffffff #808080 #808080 #ffffff;
    cursor: pointer;
    margin-bottom: 10px;
}

.login-box button:hover { background: #e0dcd8; }
.login-box button:active { border-color: #808080 #ffffff #ffffff #808080; }

.login-error {
    color: #cc0000;
    font-size: 11px;
    text-align: center;
    min-height: 14px;
}

/* ─── App Layout ─────────────────────────────────────────────────────────────── */
#app {
    display: flex;
    flex-direction: column;
    height: 100vh;
}

/* ─── Title Bar ──────────────────────────────────────────────────────────────── */
#titlebar {
    background: #000080;
    color: #ffffff;
    font-size: 11px;
    padding: 2px 8px;
    flex-shrink: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
}

/* ─── Toolbar ────────────────────────────────────────────────────────────────── */
#toolbar {
    background: #d4d0c8;
    border-bottom: 2px solid #808080;
    padding: 3px 6px;
    display: flex;
    align-items: center;
    gap: 3px;
    flex-shrink: 0;
    box-shadow: inset 0 1px 0 #ffffff;
}

#toolbar button {
    padding: 2px 8px;
    font-size: 11px;
    font-family: inherit;
    background: #d4d0c8;
    border: 1px solid;
    border-color: #ffffff #808080 #808080 #ffffff;
    cursor: pointer;
    height: 22px;
    white-space: nowrap;
}

#toolbar button:hover:not(:disabled) { background: #e8e4e0; }
#toolbar button:active:not(:disabled) { border-color: #808080 #ffffff #ffffff #808080; }
#toolbar button:disabled { color: #a0a0a0; cursor: default; }
#toolbar button.active {
    border-color: #808080 #ffffff #ffffff #808080;
    background: #b8b4b0;
}

.toolbar-sep {
    width: 1px;
    height: 18px;
    background: #808080;
    box-shadow: 1px 0 0 #ffffff;
    margin: 0 3px;
    flex-shrink: 0;
}

.toolbar-spacer { flex: 1; }

/* ─── Breadcrumb ─────────────────────────────────────────────────────────────── */
#breadcrumb {
    background: #f0f0f0;
    border-bottom: 1px solid #c0c0c0;
    padding: 2px 8px;
    font-size: 11px;
    flex-shrink: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-height: 18px;
    line-height: 14px;
}

.bc-link {
    color: #000080;
    cursor: pointer;
    text-decoration: underline;
}
.bc-link:hover { color: #0000ff; }
.bc-sep { color: #888; margin: 0 2px; }
.bc-current { color: #000; font-weight: bold; }

/* ─── Treemap Container ──────────────────────────────────────────────────────── */
#treemap-container {
    flex: 1;
    position: relative;
    overflow: hidden;
    background: #ffffff;
    cursor: default;
}

/* ─── Status Bar ─────────────────────────────────────────────────────────────── */
#statusbar {
    background: #d4d0c8;
    border-top: 1px solid #808080;
    padding: 2px 8px;
    font-size: 11px;
    flex-shrink: 0;
    display: flex;
    gap: 16px;
    height: 20px;
    align-items: center;
}

/* ─── Tooltip ────────────────────────────────────────────────────────────────── */
.tooltip {
    position: fixed;
    background: #ffffc8;
    border: 1px solid #808060;
    box-shadow: 2px 2px 5px rgba(0, 0, 0, 0.3);
    padding: 5px 9px;
    font-size: 11px;
    font-family: 'Courier New', monospace;
    pointer-events: none;
    z-index: 1000;
    max-width: 320px;
    line-height: 1.5;
}

.tooltip strong {
    display: block;
    font-size: 12px;
    margin-bottom: 2px;
}

/* ─── Progress Overlay ───────────────────────────────────────────────────────── */
#progress-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.45);
    display: none;             /* shown by JS during scan */
    align-items: center;
    justify-content: center;
    z-index: 500;
}

.progress-box {
    background: #d4d0c8;
    border: 2px solid;
    border-color: #ffffff #808080 #808080 #ffffff;
    padding: 22px 36px;
    min-width: 360px;
    box-shadow: 4px 4px 10px rgba(0, 0, 0, 0.4);
}

.progress-box h3 {
    font-size: 13px;
    color: #000080;
    margin-bottom: 12px;
}

.progress-box p {
    font-size: 11px;
    margin: 4px 0;
}

.progress-path {
    color: #555;
    max-width: 320px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 10px;
    margin-top: 8px;
}

/* ─── Modal Overlay ──────────────────────────────────────────────────────────── */
.modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.35);
    display: none;             /* shown by JS */
    align-items: center;
    justify-content: center;
    z-index: 400;
}

.modal-box {
    background: #d4d0c8;
    border: 2px solid;
    border-color: #ffffff #808080 #808080 #ffffff;
    min-width: 380px;
    max-width: 580px;
    width: 90%;
    box-shadow: 4px 4px 10px rgba(0, 0, 0, 0.4);
}

.modal-header {
    background: #000080;
    color: #ffffff;
    padding: 4px 8px;
    display: flex;
    justify-content: space-between;
    align-items: center;
}

.modal-header h3 {
    font-size: 12px;
    font-weight: normal;
}

.modal-header button {
    background: #d4d0c8;
    border: 1px solid;
    border-color: #ffffff #808080 #808080 #ffffff;
    color: #000;
    padding: 0 6px;
    cursor: pointer;
    font-size: 13px;
    line-height: 1.2;
}
.modal-header button:hover { background: #e0dcd8; }

.modal-body {
    padding: 10px;
    max-height: 320px;
    overflow-y: auto;
}

/* Scrollbar — Windows classic style */
.modal-body::-webkit-scrollbar { width: 16px; }
.modal-body::-webkit-scrollbar-track {
    background: #d4d0c8;
    border: 1px inset #808080;
}
.modal-body::-webkit-scrollbar-thumb {
    background: #d4d0c8;
    border: 1px solid;
    border-color: #ffffff #808080 #808080 #ffffff;
}

/* ─── Mount Items ────────────────────────────────────────────────────────────── */
.mount-item {
    padding: 7px 9px;
    cursor: pointer;
    border: 1px solid transparent;
    margin-bottom: 3px;
    font-size: 11px;
}

.mount-item:hover {
    background: #000080;
    color: #ffffff;
    border-color: #000060;
}

.mount-path {
    font-weight: bold;
    font-size: 12px;
}

.mount-info {
    color: #666;
    font-size: 10px;
    margin-top: 2px;
}

.mount-item:hover .mount-info { color: #ccc; }

/* ─── Free Space Panel ───────────────────────────────────────────────────────── */
.free-space-panel {
    position: absolute;
    left: 0;
    top: 0;
    background: #e8e8e8;
    border-right: 2px solid #aaaaaa;
    display: flex;
    align-items: center;
    justify-content: center;
}

.free-space-info {
    text-align: center;
    font-size: 12px;
    color: #555;
    line-height: 2;
    padding: 16px;
}

/* ─── SVG Treemap cells ──────────────────────────────────────────────────────── */
.cell rect { transition: opacity 0.1s; }
.cell rect:hover { opacity: 0.85; }
```

**Step 2: Verify build**

```bash
go build -o /tmp/sm-test . && echo "Build OK" && rm /tmp/sm-test
```

**Step 3: Commit**

```bash
git add static/style.css
git commit -m "feat: Windows-classic CSS styling for toolbar, treemap, modals"
```

---

## Task 10: app.js — Complete Frontend Logic

**Files:**
- Create: `static/app.js`

**Step 1: Create static/app.js** (full file, ~350 lines)

Create `static/app.js`:
```javascript
'use strict';

// ─── State ────────────────────────────────────────────────────────────────────
let scanMeta      = null;   // { root, files, dirs, totalDisk, freeDisk }
let currentNode   = null;   // currently displayed root node
let navStack      = [];     // navigation history: array of FileNode
let freeSpaceMode = false;  // free-space overlay toggle
let lastScanPath  = null;   // path of last scan (for Reload)
let evtSource     = null;   // active EventSource (or null)
let tooltip       = null;   // tooltip DOM element

// ─── Size Formatting ──────────────────────────────────────────────────────────
function formatSize(bytes) {
    if (!bytes || bytes === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(
        Math.floor(Math.log(Math.abs(bytes)) / Math.log(1024)),
        units.length - 1
    );
    const v = bytes / Math.pow(1024, i);
    return (i === 0 ? v.toFixed(0) : v.toFixed(1)) + '\u00a0' + units[i];
}

// ─── HTML Escaping ────────────────────────────────────────────────────────────
function esc(s) {
    return String(s)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

// ─── Color Scale (SpaceMonger: green=small → yellow → red=large) ──────────────
function getColor(value, maxValue) {
    if (!maxValue || maxValue === 0) return '#66bb6a';
    const ratio = Math.min(value / maxValue, 1);
    // d3.interpolateRdYlGn: 0=red, 1=green — we reverse: small=green, large=red
    return d3.interpolateRdYlGn(1 - ratio);
}

// ─── Tooltip ──────────────────────────────────────────────────────────────────
function initTooltip() {
    tooltip = document.createElement('div');
    tooltip.className = 'tooltip';
    tooltip.style.display = 'none';
    document.body.appendChild(tooltip);
}

function showTooltip(event, node, value) {
    const mtime = node.mtime
        ? new Date(node.mtime).toLocaleString()
        : '—';
    tooltip.innerHTML =
        `<strong>${esc(node.name)}</strong>` +
        `<div>Size: ${formatSize(value != null ? value : node.size)}</div>` +
        `<div>Modified: ${mtime}</div>` +
        `<div>${node.isDir ? 'Directory' : 'File'}</div>`;
    tooltip.style.display = 'block';
    positionTooltip(event);
}

function moveTooltip(event) { positionTooltip(event); }
function hideTooltip()       { tooltip.style.display = 'none'; }

function positionTooltip(event) {
    const x  = event.clientX + 14;
    const y  = event.clientY - 10;
    const tw = tooltip.offsetWidth  || 180;
    const th = tooltip.offsetHeight || 80;
    tooltip.style.left = Math.min(x, window.innerWidth  - tw - 6) + 'px';
    tooltip.style.top  = Math.min(y, window.innerHeight - th - 6) + 'px';
}

// ─── SVG Text Truncation ─────────────────────────────────────────────────────
function truncateSVGText(el, maxWidth) {
    if (maxWidth <= 0) { el.textContent = ''; return; }
    const full = el.textContent;
    if (el.getComputedTextLength() <= maxWidth) return;
    let s = full;
    while (s.length > 0 && el.getComputedTextLength() > maxWidth) {
        s = s.slice(0, -1);
        el.textContent = s + '\u2026'; // ellipsis
    }
    if (el.getComputedTextLength() > maxWidth) el.textContent = '';
}

// ─── Treemap Rendering ───────────────────────────────────────────────────────
function renderTreemap(node) {
    const container = document.getElementById('treemap-container');
    container.innerHTML = '';
    if (!node) return;

    const W = container.clientWidth;
    const H = container.clientHeight;
    if (W <= 0 || H <= 0) return;

    // ── Free Space panel (left side) ──────────────────────────────────────────
    let svgX = 0;
    let svgW = W;

    if (freeSpaceMode && scanMeta && scanMeta.totalDisk > 0) {
        const usedBytes  = scanMeta.root ? scanMeta.root.size : 0;
        const totalBytes = scanMeta.totalDisk;
        const usedRatio  = Math.max(0.05, Math.min(usedBytes / totalBytes, 0.99));
        svgW = Math.floor(W * usedRatio);
        svgX = W - svgW;

        const freeBytes = scanMeta.freeDisk || 0;
        const freePct   = (freeBytes / totalBytes * 100).toFixed(1);

        const panel = document.createElement('div');
        panel.className = 'free-space-panel';
        panel.style.cssText = `left:0;top:0;width:${svgX}px;height:${H}px;`;
        panel.innerHTML = `
            <div class="free-space-info">
                <div>&lt;Free Space: ${freePct}%&gt;</div>
                <div>${formatSize(freeBytes)} Free</div>
                <div>Files Total: ${(scanMeta.files || 0).toLocaleString()}</div>
                <div>Folders Total: ${(scanMeta.dirs || 0).toLocaleString()}</div>
            </div>`;
        container.appendChild(panel);
    }

    // ── SVG for treemap ───────────────────────────────────────────────────────
    const svgEl = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svgEl.setAttribute('width',  svgW);
    svgEl.setAttribute('height', H);
    svgEl.style.cssText = `position:absolute;left:${svgX}px;top:0;`;
    container.appendChild(svgEl);

    const root = d3.hierarchy(node)
        .sum(d => (!d.children || d.children.length === 0)
            ? Math.max(d.size, 1)
            : 0)
        .sort((a, b) => b.value - a.value);

    d3.treemap()
        .tile(d3.treemapSquarify)
        .size([svgW, H])
        .paddingOuter(2)
        .paddingTop(18)
        .paddingInner(1)
        .round(true)(root);

    const maxValue = root.value;
    const nodes    = root.descendants().filter(d => d.depth > 0);
    const svg      = d3.select(svgEl);

    const cell = svg.selectAll('g.cell')
        .data(nodes)
        .join('g')
        .attr('class', 'cell')
        .attr('transform', d => `translate(${d.x0},${d.y0})`);

    // Background rectangle
    cell.append('rect')
        .attr('width',  d => Math.max(0, d.x1 - d.x0 - 0.5))
        .attr('height', d => Math.max(0, d.y1 - d.y0 - 0.5))
        .attr('fill',   d => getColor(d.value, maxValue))
        .attr('stroke', 'rgba(0,0,0,0.25)')
        .attr('stroke-width', 0.5)
        .style('cursor', d => (d.data.isDir && d.data.children) ? 'pointer' : 'default')
        .on('click',     (ev, d) => { ev.stopPropagation(); handleClick(d); })
        .on('mouseover', (ev, d) => showTooltip(ev, d.data, d.value))
        .on('mousemove', moveTooltip)
        .on('mouseout',  hideTooltip);

    // Name label
    cell.filter(d => (d.x1 - d.x0) >= 18 && (d.y1 - d.y0) >= 13)
        .append('text')
        .attr('x', 3).attr('y', 12)
        .attr('font-size', '11px')
        .attr('font-family', "'Courier New',monospace")
        .attr('fill', '#000')
        .attr('pointer-events', 'none')
        .text(d => d.data.name)
        .each(function(d) { truncateSVGText(this, d.x1 - d.x0 - 6); });

    // Size label (only for larger cells)
    cell.filter(d => (d.x1 - d.x0) >= 45 && (d.y1 - d.y0) >= 27)
        .append('text')
        .attr('x', 3).attr('y', 24)
        .attr('font-size', '10px')
        .attr('font-family', "'Courier New',monospace")
        .attr('fill', '#333')
        .attr('pointer-events', 'none')
        .text(d => formatSize(d.value))
        .each(function(d) { truncateSVGText(this, d.x1 - d.x0 - 6); });
}

function handleClick(d) {
    if (d.data.isDir && d.data.children && d.data.children.length > 0) {
        zoomInto(d.data);
    }
}

// ─── Navigation ───────────────────────────────────────────────────────────────
function zoomInto(node) {
    navStack.push(node);
    currentNode = node;
    renderTreemap(currentNode);
    updateBreadcrumb();
    updateButtons();
}

function zoomOut() {
    if (navStack.length <= 1) return;
    navStack.pop();
    currentNode = navStack[navStack.length - 1];
    renderTreemap(currentNode);
    updateBreadcrumb();
    updateButtons();
}

function zoomFull() {
    if (!scanMeta) return;
    navStack    = [scanMeta.root];
    currentNode = scanMeta.root;
    renderTreemap(currentNode);
    updateBreadcrumb();
    updateButtons();
}

function updateBreadcrumb() {
    const bc = document.getElementById('breadcrumb');
    bc.innerHTML = navStack.map((node, i) => {
        if (i === navStack.length - 1) {
            return `<span class="bc-current">${esc(node.name)}</span>`;
        }
        return `<span class="bc-link" data-depth="${i}">${esc(node.name)}</span>` +
               `<span class="bc-sep"> › </span>`;
    }).join('');

    bc.querySelectorAll('.bc-link[data-depth]').forEach(el => {
        el.addEventListener('click', () => {
            const depth = parseInt(el.dataset.depth, 10);
            navStack    = navStack.slice(0, depth + 1);
            currentNode = navStack[navStack.length - 1];
            renderTreemap(currentNode);
            updateBreadcrumb();
            updateButtons();
        });
    });
}

function updateButtons() {
    const atRoot = navStack.length <= 1;
    document.getElementById('btn-zoom-out').disabled  = atRoot;
    document.getElementById('btn-zoom-full').disabled = atRoot;
}

// ─── Status Bar ───────────────────────────────────────────────────────────────
function updateStatusBar() {
    if (!scanMeta) return;
    document.getElementById('status-files').textContent = (scanMeta.files || 0).toLocaleString();
    document.getElementById('status-dirs').textContent  = (scanMeta.dirs  || 0).toLocaleString();

    if (scanMeta.totalDisk > 0) {
        const usedBytes = scanMeta.totalDisk - scanMeta.freeDisk;
        document.getElementById('status-used').textContent = formatSize(usedBytes);
        document.getElementById('status-free').textContent = formatSize(scanMeta.freeDisk);
        document.getElementById('status-space').style.display = '';
    }
    document.getElementById('titlebar').textContent =
        `${currentNode?.name || '/'} — ` +
        (scanMeta.totalDisk > 0
            ? `${formatSize(scanMeta.totalDisk)} Total — ${formatSize(scanMeta.freeDisk)} Free — `
            : '') +
        'SpaceMonger';
}

// ─── Scanning (SSE) ───────────────────────────────────────────────────────────
function startScan(path) {
    if (evtSource) { evtSource.close(); evtSource = null; }
    lastScanPath = path;
    showProgress();
    updateProgressUI({ files: 0, dirs: 0, current: path });

    const es = new EventSource(`/api/scan?path=${encodeURIComponent(path)}`);
    evtSource = es;

    es.onmessage = function(e) {
        const msg = JSON.parse(e.data);
        updateProgressUI(msg);

        if (msg.status === 'done') {
            es.close();
            evtSource = null;

            scanMeta = {
                root:      msg.root,
                files:     msg.files,
                dirs:      msg.dirs,
                totalDisk: msg.totalDisk || 0,
                freeDisk:  msg.freeDisk  || 0,
            };

            navStack    = [scanMeta.root];
            currentNode = scanMeta.root;

            hideProgress();
            renderTreemap(currentNode);
            updateBreadcrumb();
            updateButtons();
            updateStatusBar();
        }
    };

    es.onerror = function() {
        es.close();
        evtSource = null;
        hideProgress();
        showError('Scan failed — check server logs or permissions.');
    };
}

function showProgress() {
    document.getElementById('progress-overlay').style.display = 'flex';
}
function hideProgress() {
    document.getElementById('progress-overlay').style.display = 'none';
}
function updateProgressUI(msg) {
    document.getElementById('progress-files').textContent = (msg.files || 0).toLocaleString();
    document.getElementById('progress-dirs').textContent  = (msg.dirs  || 0).toLocaleString();
    if (msg.current) {
        const s   = msg.current;
        const max = 58;
        document.getElementById('progress-current').textContent =
            s.length > max ? '…' + s.slice(-(max - 1)) : s;
    }
}

// ─── Open Drive Dialog ────────────────────────────────────────────────────────
async function openDriveDialog() {
    let mounts;
    try {
        const resp = await fetch('/api/mounts');
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        mounts = await resp.json();
    } catch (e) {
        showError('Could not load drives: ' + e.message);
        return;
    }

    const list = document.getElementById('mounts-list');
    if (!mounts || mounts.length === 0) {
        list.innerHTML = '<p style="padding:12px;color:#666;font-size:11px">No mountpoints found.</p>';
    } else {
        list.innerHTML = mounts.map(m => `
            <div class="mount-item" data-path="${esc(m.path)}">
                <div class="mount-path">${esc(m.path)}</div>
                <div class="mount-info">
                    ${esc(m.device)} &nbsp;|&nbsp;
                    ${esc(m.fstype)} &nbsp;|&nbsp;
                    ${formatSize(m.total)} total &nbsp;|&nbsp;
                    ${formatSize(m.free)} free
                </div>
            </div>`).join('');

        list.querySelectorAll('.mount-item').forEach(el => {
            el.addEventListener('click', () => {
                closeModal('open-modal');
                startScan(el.dataset.path);
            });
        });
    }
    document.getElementById('open-modal').style.display = 'flex';
}

function closeModal(id) {
    document.getElementById(id).style.display = 'none';
}

// ─── Free Space Toggle ────────────────────────────────────────────────────────
function toggleFreeSpace() {
    freeSpaceMode = !freeSpaceMode;
    document.getElementById('btn-free-space').classList.toggle('active', freeSpaceMode);
    if (currentNode) renderTreemap(currentNode);
}

// ─── Auth ─────────────────────────────────────────────────────────────────────
function showLoginOverlay() {
    document.getElementById('login-overlay').style.display = 'flex';
    document.getElementById('app').style.display = 'none';
    setTimeout(() => document.getElementById('login-password').focus(), 50);
}

function hideLoginOverlay() {
    document.getElementById('login-overlay').style.display = 'none';
    document.getElementById('app').style.display = 'flex';
}

async function doLogin() {
    const password = document.getElementById('login-password').value;
    const errEl    = document.getElementById('login-error');
    errEl.textContent = '';
    try {
        const resp = await fetch('/api/auth', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ password }),
        });
        if (resp.ok) {
            hideLoginOverlay();
        } else {
            errEl.textContent = 'Incorrect password.';
            document.getElementById('login-password').select();
        }
    } catch (e) {
        errEl.textContent = 'Connection error.';
    }
}

// ─── Error Toast ──────────────────────────────────────────────────────────────
function showError(msg) {
    const div = document.createElement('div');
    div.style.cssText =
        'position:fixed;bottom:40px;left:50%;transform:translateX(-50%);' +
        'background:#cc0000;color:#fff;padding:8px 18px;font-size:12px;' +
        'z-index:9999;border:1px solid #900;box-shadow:2px 2px 6px rgba(0,0,0,.4);';
    div.textContent = msg;
    document.body.appendChild(div);
    setTimeout(() => div.remove(), 4000);
}

// ─── Resize Handling ──────────────────────────────────────────────────────────
let resizeTimer = null;
window.addEventListener('resize', () => {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(() => {
        if (currentNode) renderTreemap(currentNode);
    }, 150);
});

// ─── Keyboard Shortcuts ───────────────────────────────────────────────────────
document.addEventListener('keydown', e => {
    if (e.key === 'Backspace' || e.key === 'ArrowLeft') {
        if (navStack.length > 1 &&
            document.activeElement.tagName !== 'INPUT') {
            zoomOut();
        }
    }
    if (e.key === 'Escape') closeModal('open-modal');
});

// ─── Initialization ───────────────────────────────────────────────────────────
async function init() {
    initTooltip();

    // Check auth: if API returns 401, show login overlay
    const resp = await fetch('/api/mounts').catch(() => null);
    if (!resp || resp.status === 401) {
        showLoginOverlay();
        return;
    }
    document.getElementById('app').style.display = 'flex';
    document.getElementById('login-overlay').style.display = 'none';
    bindEvents();
}

function bindEvents() {
    document.getElementById('btn-open')
        .addEventListener('click', openDriveDialog);

    document.getElementById('btn-reload')
        .addEventListener('click', () => {
            if (lastScanPath) startScan(lastScanPath);
        });

    document.getElementById('btn-zoom-full')
        .addEventListener('click', zoomFull);

    document.getElementById('btn-zoom-in')
        .addEventListener('click', () => {
            // zoom into the clicked node — double-click also works
        });

    document.getElementById('btn-zoom-out')
        .addEventListener('click', zoomOut);

    document.getElementById('btn-free-space')
        .addEventListener('click', toggleFreeSpace);

    document.getElementById('close-open-modal')
        .addEventListener('click', () => closeModal('open-modal'));

    document.getElementById('open-modal')
        .addEventListener('click', e => {
            if (e.target === e.currentTarget) closeModal('open-modal');
        });

    document.getElementById('login-btn')
        .addEventListener('click', doLogin);

    document.getElementById('login-password')
        .addEventListener('keydown', e => { if (e.key === 'Enter') doLogin(); });
}

document.addEventListener('DOMContentLoaded', init);
```

**Step 2: Verify build compiles**

```bash
go build -o /tmp/sm-test . && echo "Build OK" && rm /tmp/sm-test
```
Expected: `Build OK`

**Step 3: Run all tests**

```bash
go test ./... -v
```
Expected: all tests PASS

**Step 4: Commit**

```bash
git add static/app.js
git commit -m "feat: complete frontend - D3 treemap, zoom, tooltips, scan SSE, auth"
```

---

## Task 11: Smoke Test — Manual Verification

**Step 1: Build final binary**

```bash
go build -ldflags="-s -w" -o spacemonger .
echo "Binary size: $(du -h spacemonger | cut -f1)"
```
Expected: `Build OK`, binary size typically 8–12 MB

**Step 2: Start the server**

```bash
./spacemonger &
sleep 1
```
Expected: log line: `SpaceMonger running at http://0.0.0.0:4322`

**Step 3: Test API endpoints**

```bash
# Check mounts
curl -s http://localhost:4322/api/mounts | head -c 200

# Start a scan of /tmp (fast, small)
curl -s "http://localhost:4322/api/scan?path=/tmp" &
sleep 2

# Get data
curl -s http://localhost:4322/api/data | head -c 300
```
Expected: JSON responses, scan produces tree data

**Step 4: Stop test server**

```bash
pkill spacemonger 2>/dev/null || true
```

**Step 5: Commit binary to .gitignore**

```bash
# binary should already be in .gitignore — verify:
git status
```
Expected: `spacemonger` is NOT listed as untracked (it's gitignored)

**Step 6: Commit**

```bash
git add -A
git commit -m "test: smoke test passed - binary builds and serves correctly"
```

---

## Task 12: install.sh

**Files:**
- Create: `install.sh`

**Step 1: Create install.sh**

Create `install.sh`:
```bash
#!/usr/bin/env bash
# SpaceMonger Linux — Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/OWNER/spacemonger/main/install.sh | sudo bash

set -euo pipefail

REPO="OWNER/spacemonger"          # ← change to your GitHub user/repo
INSTALL_DIR="/opt/spacemonger"
SERVICE_NAME="spacemonger"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

# ─── Colors ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
info()    { echo -e "${GREEN}[✓]${NC} $*"; }
warn()    { echo -e "${YELLOW}[!]${NC} $*"; }
error()   { echo -e "${RED}[✗]${NC} $*"; exit 1; }

echo "╔══════════════════════════════════════╗"
echo "║     SpaceMonger Linux Installer      ║"
echo "╚══════════════════════════════════════╝"
echo ""

# ─── Root check ───────────────────────────────────────────────────────────────
[ "$EUID" -eq 0 ] || error "Please run as root: sudo bash install.sh"

# ─── Detect architecture ──────────────────────────────────────────────────────
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)         ARCH_TAG="amd64" ;;
    aarch64|arm64)  ARCH_TAG="arm64" ;;
    armv7l)         ARCH_TAG="arm"   ;;
    *)              error "Unsupported architecture: $ARCH" ;;
esac

BINARY_NAME="spacemonger-linux-${ARCH_TAG}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${BINARY_NAME}"

info "Architecture: $ARCH_TAG"
info "Installing to: $INSTALL_DIR"

# ─── Create install directory ─────────────────────────────────────────────────
mkdir -p "$INSTALL_DIR"

# ─── Download binary ──────────────────────────────────────────────────────────
info "Downloading $BINARY_NAME..."
if command -v curl &>/dev/null; then
    curl -fsSL "$DOWNLOAD_URL" -o "$INSTALL_DIR/spacemonger" || \
        error "Download failed. Check: $DOWNLOAD_URL"
elif command -v wget &>/dev/null; then
    wget -q "$DOWNLOAD_URL" -O "$INSTALL_DIR/spacemonger" || \
        error "Download failed. Check: $DOWNLOAD_URL"
else
    error "curl or wget is required for installation"
fi
chmod +x "$INSTALL_DIR/spacemonger"
info "Downloaded spacemonger binary"

# ─── Generate settings.json if missing ───────────────────────────────────────
if [ ! -f "$INSTALL_DIR/settings.json" ]; then
    PASSWORD=$(< /dev/urandom tr -dc 'a-z0-9' | head -c 6 || true)
    [ -z "$PASSWORD" ] && PASSWORD=$(date +%s | sha256sum | head -c 6)
    cat > "$INSTALL_DIR/settings.json" <<SETTINGS
{
  "port": 4322,
  "bind": "0.0.0.0",
  "auth": {
    "enabled": false,
    "password": "${PASSWORD}"
  }
}
SETTINGS
    chmod 600 "$INSTALL_DIR/settings.json"
    info "Created settings.json (generated password: ${YELLOW}${PASSWORD}${NC})"
    warn "Auth is disabled by default. Edit settings.json to enable."
else
    info "Using existing settings.json"
fi

# ─── Install systemd service ──────────────────────────────────────────────────
cat > "$SERVICE_FILE" <<SERVICE
[Unit]
Description=SpaceMonger - Web-based Disk Space Analyzer
After=network.target
Documentation=https://github.com/${REPO}

[Service]
Type=simple
User=root
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/spacemonger
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
SERVICE

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"
info "Systemd service installed and started"

# ─── Get local IP ─────────────────────────────────────────────────────────────
LOCAL_IP=$(hostname -I 2>/dev/null | awk '{print $1}') || LOCAL_IP="<your-server-ip>"

# ─── Done ─────────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}╔══════════════════════════════════════╗${NC}"
echo -e "${GREEN}║   SpaceMonger installed successfully  ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════╝${NC}"
echo ""
echo -e "  Local:    ${GREEN}http://localhost:4322${NC}"
echo -e "  Network:  ${GREEN}http://${LOCAL_IP}:4322${NC}"
echo ""
echo "  Config:   $INSTALL_DIR/settings.json"
echo ""
echo "  Commands:"
echo "    Status:  systemctl status $SERVICE_NAME"
echo "    Logs:    journalctl -u $SERVICE_NAME -f"
echo "    Stop:    systemctl stop $SERVICE_NAME"
echo "    Remove:  bash uninstall.sh"
echo ""
```

**Step 2: Make executable**

```bash
chmod +x install.sh
```

**Step 3: Verify script syntax**

```bash
bash -n install.sh && echo "Syntax OK"
```
Expected: `Syntax OK`

**Step 4: Commit**

```bash
git add install.sh
git commit -m "feat: one-line curl install script with arch detection and systemd setup"
```

---

## Task 13: uninstall.sh

**Files:**
- Create: `uninstall.sh`

**Step 1: Create uninstall.sh**

Create `uninstall.sh`:
```bash
#!/usr/bin/env bash
# SpaceMonger Linux — Uninstaller
# Usage: sudo bash /opt/spacemonger/uninstall.sh

set -euo pipefail

INSTALL_DIR="/opt/spacemonger"
SERVICE_NAME="spacemonger"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

RED='\033[0;31m'; GREEN='\033[0;32m'; NC='\033[0m'
info()  { echo -e "${GREEN}[✓]${NC} $*"; }
error() { echo -e "${RED}[✗]${NC} $*"; exit 1; }

echo "SpaceMonger Uninstaller"
echo "========================"

[ "$EUID" -eq 0 ] || error "Please run as root: sudo bash uninstall.sh"

# ─── Stop and disable service ─────────────────────────────────────────────────
if systemctl is-active  --quiet "$SERVICE_NAME" 2>/dev/null; then
    systemctl stop "$SERVICE_NAME"
    info "Stopped $SERVICE_NAME service"
fi

if systemctl is-enabled --quiet "$SERVICE_NAME" 2>/dev/null; then
    systemctl disable "$SERVICE_NAME"
    info "Disabled $SERVICE_NAME service"
fi

# ─── Remove service file ──────────────────────────────────────────────────────
if [ -f "$SERVICE_FILE" ]; then
    rm -f "$SERVICE_FILE"
    systemctl daemon-reload
    info "Removed systemd service"
fi

# ─── Remove install directory ─────────────────────────────────────────────────
if [ -d "$INSTALL_DIR" ]; then
    rm -rf "$INSTALL_DIR"
    info "Removed $INSTALL_DIR"
fi

echo ""
echo -e "${GREEN}SpaceMonger has been completely removed.${NC}"
```

**Step 2: Make executable + verify**

```bash
chmod +x uninstall.sh
bash -n uninstall.sh && echo "Syntax OK"
```

**Step 3: Commit**

```bash
git add uninstall.sh
git commit -m "feat: uninstall script removes service, binary, and all files cleanly"
```

---

## Task 14: systemd Service File

**Files:**
- Create: `spacemonger.service`

**Step 1: Create spacemonger.service**

Create `spacemonger.service`:
```ini
[Unit]
Description=SpaceMonger - Web-based Disk Space Analyzer
After=network.target
Documentation=https://github.com/OWNER/spacemonger

[Service]
Type=simple
User=root
WorkingDirectory=/opt/spacemonger
ExecStart=/opt/spacemonger/spacemonger
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

**Step 2: Commit**

```bash
git add spacemonger.service
git commit -m "feat: systemd unit file for SpaceMonger service"
```

---

## Task 15: GitHub Actions — Build & Release

**Files:**
- Create: `.github/workflows/build.yml`

**Step 1: Create build.yml**

Create `.github/workflows/build.yml`:
```yaml
name: Build and Release

on:
  push:
    tags:
      - 'v*'
  pull_request:
    branches: [ main ]

permissions:
  contents: write

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Run tests
        run: go test ./... -v

  release:
    needs: test
    runs-on: ubuntu-latest
    if: startsWith(github.ref, 'refs/tags/v')

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Build linux/amd64
        run: GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o spacemonger-linux-amd64 .

      - name: Build linux/arm64
        run: GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o spacemonger-linux-arm64 .

      - name: Build linux/arm (armv7)
        run: GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="-s -w" -o spacemonger-linux-arm .

      - name: Create GitHub Release
        uses: softprops/action-gh-release@v2
        with:
          files: |
            spacemonger-linux-amd64
            spacemonger-linux-arm64
            spacemonger-linux-arm
            install.sh
            uninstall.sh
            settings.example.json
          generate_release_notes: true
```

**Step 2: Commit**

```bash
git add .github/workflows/build.yml
git commit -m "ci: GitHub Actions builds for linux amd64/arm64/arm on version tags"
```

---

## Task 16: README.md

**Files:**
- Create: `README.md`

**Step 1: Create README.md**

Create `README.md`:
```markdown
# SpaceMonger Linux

Web-based disk space analyzer for Linux. Displays your filesystem as an
interactive squarified treemap with color-coded sizes — inspired by SpaceMonger
for Windows.

![SpaceMonger Screenshot](docs/screenshot.png)

## Features

- 🗺️ Squarified treemap — green (small) → yellow → red (large)
- 🔍 Click any folder to zoom in, navigate back with Zoom Out / breadcrumb
- 📊 Free Space button shows free vs used proportionally
- ⚡ Real-time scan progress with file/folder counter
- 🔒 Optional password protection via `settings.json`
- 🌐 Accessible from local network (binds to 0.0.0.0)
- 📦 Single binary, no dependencies, ~10 MB

## Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/OWNER/spacemonger/main/install.sh | sudo bash
```

Opens at **http://localhost:4322** (also accessible from the local network).

## Uninstall

```bash
sudo bash /opt/spacemonger/uninstall.sh
```

## Configuration

Edit `/opt/spacemonger/settings.json`:

```json
{
  "port": 4322,
  "bind": "0.0.0.0",
  "auth": {
    "enabled": false,
    "password": "abc123"
  }
}
```

Restart after changes: `sudo systemctl restart spacemonger`

## Usage

| Button       | Action                                      |
|-------------|---------------------------------------------|
| **Open**    | Choose a drive/partition to analyze         |
| **Reload**  | Rescan the current path                     |
| **Full**    | Zoom back to the scan root                  |
| **In**      | Zoom into selected folder                   |
| **Out**     | Go up one directory level                   |
| **Free Space** | Show free disk space proportionally      |

Click any folder rectangle to zoom in. Use the breadcrumb path to navigate.
Press **Backspace** or **←** to go up.

## Build from Source

```bash
git clone https://github.com/OWNER/spacemonger.git
cd spacemonger
go build -o spacemonger .
./spacemonger          # runs on port 4322
```

Requires Go 1.21+.

## Releasing a New Version

```bash
git tag v1.0.0
git push origin v1.0.0
```

GitHub Actions will build binaries for amd64/arm64/arm and attach them to the release.
```

**Step 2: Commit**

```bash
git add README.md
git commit -m "docs: README with install instructions and feature overview"
```

---

## Task 17: Final Verification

**Step 1: Run all tests**

```bash
go test ./... -v -count=1
```
Expected: all tests PASS, 0 failures

**Step 2: Build production binary**

```bash
go build -ldflags="-s -w" -o spacemonger .
ls -lh spacemonger
```
Expected: binary exists, size 8–12 MB

**Step 3: Test the full flow manually**

```bash
# Start server
./spacemonger &
SM_PID=$!
sleep 1

# Verify it serves the UI
curl -s http://localhost:4322/ | grep -q "SpaceMonger" && echo "UI OK"

# Verify mounts API
curl -s http://localhost:4322/api/mounts | python3 -m json.tool | head -20

# Stop
kill $SM_PID 2>/dev/null || true
```
Expected: `UI OK`, JSON output from mounts

**Step 4: Check git status is clean**

```bash
git status
```
Expected: `nothing to commit, working tree clean`

**Step 5: Tag initial release**

Only do this after pushing to GitHub and replacing `OWNER` in `install.sh` and `README.md`.
```bash
# After updating OWNER references:
# git tag v1.0.0
# git push origin main --tags
```

**Step 6: Final commit if any changes**

```bash
git add -A
git diff --cached --quiet || git commit -m "chore: final cleanup and verification"
```

---

## Summary

| Task | Component | Tests |
|------|-----------|-------|
| 1  | Project scaffold | — |
| 2  | Settings + genPassword | 4 unit tests |
| 3  | Filesystem scanner | 3 unit tests |
| 4  | Mountpoints parser | 2 unit tests |
| 5  | Auth system | 4 unit tests |
| 6  | HTTP handlers + main() | — |
| 7  | D3.js v7 (download) | — |
| 8  | index.html | — |
| 9  | style.css | — |
| 10 | app.js (complete frontend) | — |
| 11 | Smoke test | manual |
| 12 | install.sh | syntax check |
| 13 | uninstall.sh | syntax check |
| 14 | spacemonger.service | — |
| 15 | GitHub Actions | — |
| 16 | README.md | — |
| 17 | Final verification | all tests + build |

**Total Go unit tests: 13**
**Deployment: `curl | sudo bash` one-liner**
