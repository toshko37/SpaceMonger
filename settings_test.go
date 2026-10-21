package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSettings_missingFile(t *testing.T) {
	if _, err := loadSettings("/nonexistent/path/settings.json"); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadSettings_valid(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	in := Settings{Port: 9999, Bind: "127.0.0.1", Auth: AuthSettings{Enabled: true, Password: "abc123"}}
	data, _ := json.Marshal(in)
	os.WriteFile(p, data, 0600)

	s, err := loadSettings(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Port != 9999 || s.Bind != "127.0.0.1" || s.Auth.Password != "abc123" || !s.Auth.Enabled {
		t.Errorf("unexpected settings: %+v", s)
	}
}

func TestLoadSettings_defaultsForMissingFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(p, []byte(`{"auth":{"enabled":true,"password":"x"}}`), 0600)
	s, err := loadSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Port != 4322 || s.Bind != "0.0.0.0" {
		t.Errorf("expected defaults, got port=%d bind=%q", s.Port, s.Bind)
	}
}

func TestSaveSettings_roundTripAndMode(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	in := defaultSettings()
	if err := saveSettings(p, in); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0600 {
		t.Errorf("expected mode 0600, got %o", fi.Mode().Perm())
	}
	out, err := loadSettings(p)
	if err != nil || out != in {
		t.Errorf("round trip mismatch: %+v vs %+v (%v)", out, in, err)
	}
}

func TestDefaultSettings_authEnabled(t *testing.T) {
	s := defaultSettings()
	if !s.Auth.Enabled || len(s.Auth.Password) != passwordLen {
		t.Errorf("expected auth enabled with %d-char password, got %+v", passwordLen, s.Auth)
	}
}

func TestGenPassword_alphanumeric(t *testing.T) {
	for i := 0; i < 100; i++ {
		p := genPassword()
		if len(p) != passwordLen {
			t.Fatalf("expected %d chars, got %d: %q", passwordLen, len(p), p)
		}
		for _, c := range p {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
				t.Errorf("non-alphanumeric char %c in password %s", c, p)
			}
		}
	}
}
