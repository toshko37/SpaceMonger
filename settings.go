package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Settings is the on-disk configuration (settings.json).
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

func setSettings(s Settings) {
	settingsMu.Lock()
	globalSettings = s
	settingsMu.Unlock()
}

// defaultSettings returns a secure-by-default configuration: auth is
// enabled with a freshly generated password.
func defaultSettings() Settings {
	return Settings{
		Port: 4322,
		Bind: "0.0.0.0",
		Auth: AuthSettings{Enabled: true, Password: genPassword()},
	}
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
	if s.Port <= 0 || s.Port > 65535 {
		s.Port = 4322
	}
	if s.Bind == "" {
		s.Bind = "0.0.0.0"
	}
	return s, nil
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
