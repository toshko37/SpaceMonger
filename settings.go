package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
)

// Settings is the on-disk configuration (settings.json).
type Settings struct {
	Port        int          `json:"port"`
	Bind        string       `json:"bind"`
	Auth        AuthSettings `json:"auth"`
	TLS         TLSSettings  `json:"tls"`
	Exclude     []string     `json:"exclude"`     // absolute paths or name patterns (e.g. "node_modules", "*.tmp")
	ScanWorkers int          `json:"scanWorkers"` // 0 = number of CPUs
	HistorySize int          `json:"historySize"` // completed scans kept in memory
	AllowDelete bool         `json:"allowDelete"` // enable deleting files from the UI
	// ReleaseAfter drops stored scans and returns memory to the OS after this
	// much time without API activity, e.g. "30m". "0" keeps them forever.
	ReleaseAfter string `json:"releaseAfter"`
}

type AuthSettings struct {
	Enabled  bool   `json:"enabled"`
	Password string `json:"password"`
}

// TLSSettings enables HTTPS when both files are set.
type TLSSettings struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

func (t TLSSettings) Enabled() bool { return t.Cert != "" && t.Key != "" }

const (
	defaultReleaseAfter = "30m"
	defaultPort         = 4322
	defaultHistorySize  = 5
	maxHistorySize      = 20
	maxScanWorkers      = 32
)

var (
	globalSettings Settings
	settingsMu     sync.RWMutex
)

func getSettings() Settings {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	return globalSettings
}

func setSettings(s Settings) {
	settingsMu.Lock()
	globalSettings = s
	settingsMu.Unlock()
}

// defaultSettings returns a secure-by-default configuration: auth is
// enabled with a freshly generated password.
func defaultSettings() Settings {
	return normalize(Settings{
		Port:    defaultPort,
		Bind:    "0.0.0.0",
		Auth:    AuthSettings{Enabled: true, Password: genPassword()},
		Exclude: []string{},
	})
}

// normalize fills in defaults for missing or invalid fields.
func normalize(s Settings) Settings {
	if s.Port <= 0 || s.Port > 65535 {
		s.Port = defaultPort
	}
	if s.Bind == "" {
		s.Bind = "0.0.0.0"
	}
	if s.Exclude == nil {
		s.Exclude = []string{}
	}
	if s.HistorySize <= 0 {
		s.HistorySize = defaultHistorySize
	}
	if s.HistorySize > maxHistorySize {
		s.HistorySize = maxHistorySize
	}
	if s.ScanWorkers < 0 {
		s.ScanWorkers = 0
	}
	if s.ScanWorkers > maxScanWorkers {
		s.ScanWorkers = maxScanWorkers
	}
	if s.ReleaseAfter == "" {
		s.ReleaseAfter = defaultReleaseAfter
	}
	return s
}

// releaseAfterDuration parses ReleaseAfter; 0 means never release.
func (s Settings) releaseAfterDuration() time.Duration {
	d, err := time.ParseDuration(s.ReleaseAfter)
	if err != nil || d < 0 {
		return 0
	}
	return d
}

// effectiveWorkers resolves ScanWorkers == 0 to a CPU-based default.
func (s Settings) effectiveWorkers() int {
	if s.ScanWorkers > 0 {
		return s.ScanWorkers
	}
	n := runtime.NumCPU()
	if n < 2 {
		n = 2
	}
	if n > 16 {
		n = 16
	}
	return n
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
	return normalize(s), nil
}

func saveSettings(path string, s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

const passwordLen = 10

// genPassword returns a random lowercase alphanumeric password.
func genPassword() string {
	const chars = "abcdefghijkmnpqrstuvwxyz23456789" // no ambiguous l/o/0/1
	b := make([]byte, passwordLen)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	for i, v := range b {
		b[i] = chars[int(v)%len(chars)]
	}
	return string(b)
}
