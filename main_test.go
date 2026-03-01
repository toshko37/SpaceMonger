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
