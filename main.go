package main

import (
	"bufio"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	for i, v := range b {
		b[i] = chars[int(v)%len(chars)]
	}
	return string(b)
}

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
	rootDev   uint64 // device ID of scan root — skip directories on other devices
}

var globalScanner = &Scanner{}
var scanMu sync.Mutex

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
	s.rootDev = 0
	s.mu.Unlock()

	// Record the device ID of the scan root so we stay on one filesystem.
	var rootStat syscall.Stat_t
	if err := syscall.Stat(path, &rootStat); err == nil {
		s.rootDev = rootStat.Dev
	}

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
		childPath := filepath.Join(path, entry.Name())
		// Skip directories that are mount points for a different filesystem
		// (e.g. /proc, /sys, /dev) to avoid virtual/inflated sizes.
		if entry.IsDir() && s.rootDev != 0 {
			var st syscall.Stat_t
			if err := syscall.Stat(childPath, &st); err == nil && st.Dev != s.rootDev {
				continue
			}
		}
		child, err := s.scanDir(childPath)
		if err != nil {
			continue
		}
		node.Children = append(node.Children, child)
		node.Size += child.Size
	}

	return node, nil
}

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

	// seen tracks the shortest-path mount per device to deduplicate bind mounts
	// (e.g. container overlay storage re-mounting the same physical device).
	seen := make(map[string]int) // device → index in mounts slice
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
		if idx, exists := seen[device]; exists {
			// Keep the entry with the shorter (more canonical) path
			if len(mountPath) < len(mounts[idx].Path) {
				mounts[idx] = m
			}
			continue
		}
		seen[device] = len(mounts)
		mounts = append(mounts, m)
	}
	return mounts, sc.Err()
}

func getMounts() ([]Mount, error) {
	return parseMountsFile("/proc/mounts")
}

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

var (
	sessions  = make(map[string]struct{})
	sessionMu sync.RWMutex
)

func genToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	return hex.EncodeToString(b)
}

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
	path = filepath.Clean(path)

	if !scanMu.TryLock() {
		http.Error(w, "scan already in progress", http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		scanMu.Unlock()
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	done := make(chan struct{})
	go func() {
		defer scanMu.Unlock()
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
		scanErr := globalScanner.scanErr
		globalScanner.mu.RUnlock()

		var data []byte
		if isDone {
			var errMsg string
			if scanErr != nil {
				errMsg = scanErr.Error()
			}
			data, _ = json.Marshal(map[string]interface{}{
				"status":    "done",
				"files":     files,
				"dirs":      dirs,
				"root":      root,
				"totalDisk": total,
				"freeDisk":  free,
				"error":     errMsg,
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
			// Send a lightweight "building" signal immediately so the browser
			// can switch its UI to "Building treemap…" before we spend time
			// JSON-serialising the entire file tree (which can take seconds).
			fmt.Fprintf(w, "data: {\"status\":\"building\"}\n\n")
			flusher.Flush()
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
		if errors.Is(err, fs.ErrNotExist) {
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
