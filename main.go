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
