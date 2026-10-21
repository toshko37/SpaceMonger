package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMaybeRelease(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0644)
	runScan(t, dir)
	if store.Len() == 0 {
		t.Fatal("expected a stored scan")
	}

	// Recent activity: nothing is released.
	touchActivity()
	if maybeRelease(time.Hour) {
		t.Error("should not release while active")
	}

	// Idle long enough: scans are dropped and the scanner result cleared.
	lastActivity.Store(time.Now().Add(-2 * time.Hour).Unix())
	if !maybeRelease(time.Hour) {
		t.Fatal("expected release after idle period")
	}
	if store.Len() != 0 || globalScanner.Snapshot().Root != nil {
		t.Error("stored scans should be gone")
	}
	if maybeRelease(time.Hour) {
		t.Error("nothing left to release")
	}

	// A running scan (lock held) blocks the release.
	runScan(t, dir)
	lastActivity.Store(time.Now().Add(-2 * time.Hour).Unix())
	scanMu.Lock()
	released := maybeRelease(time.Hour)
	scanMu.Unlock()
	if released || store.Len() == 0 {
		t.Error("must not release while a scan holds the lock")
	}
}

func TestSettings_releaseAfter(t *testing.T) {
	if d := normalize(Settings{}).releaseAfterDuration(); d != 30*time.Minute {
		t.Errorf("default should be 30m, got %s", d)
	}
	if d := (Settings{ReleaseAfter: "0"}).releaseAfterDuration(); d != 0 {
		t.Errorf("\"0\" should disable, got %s", d)
	}
	if d := (Settings{ReleaseAfter: "bogus"}).releaseAfterDuration(); d != 0 {
		t.Errorf("invalid should disable, got %s", d)
	}
	if d := (Settings{ReleaseAfter: "2h"}).releaseAfterDuration(); d != 2*time.Hour {
		t.Errorf("expected 2h, got %s", d)
	}
}
