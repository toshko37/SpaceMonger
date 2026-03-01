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

func getMounts() ([]Mount, error)             { return nil, nil }
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
