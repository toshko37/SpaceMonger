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

func postDelete(id, path string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"id": id, "path": path})
	w := httptest.NewRecorder()
	deleteHandler(w, httptest.NewRequest(http.MethodPost, "/api/delete", bytes.NewReader(body)))
	return w
}

func TestDelete_disabledByDefault(t *testing.T) {
	setSettings(Settings{AllowDelete: false})
	if w := postDelete("", "/tmp/whatever"); w.Code != http.StatusForbidden {
		t.Errorf("expected 403 when disabled, got %d", w.Code)
	}
}

func TestDelete_removesAndUpdatesTree(t *testing.T) {
	setSettings(Settings{AllowDelete: true})
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0755)
	os.WriteFile(filepath.Join(sub, "big"), make([]byte, 8192), 0644)
	os.WriteFile(filepath.Join(dir, "keep"), make([]byte, 512), 0644)
	_, msg := runScan(t, dir)
	id := msg["id"].(string)
	before := int64(msg["size"].(float64))

	w := postDelete(id, sub)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp deleteResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Freed < 8192 || resp.Files != 1 || resp.Dirs != 1 {
		t.Errorf("unexpected response %+v", resp)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Error("directory should be gone from disk")
	}
	store.Read(id, func(r *ScanResult) {
		if r.Snap.Root.Size != before-resp.Freed || len(r.Snap.Root.Children) != 1 {
			t.Errorf("tree not updated: size=%d children=%d", r.Snap.Root.Size, len(r.Snap.Root.Children))
		}
		if r.Snap.Files != 1 || r.Snap.Dirs != 1 {
			t.Errorf("counters not updated: files=%d dirs=%d", r.Snap.Files, r.Snap.Dirs)
		}
	})
}

func TestDelete_refusesRootProtectedAndOutside(t *testing.T) {
	setSettings(Settings{AllowDelete: true})
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0644)
	_, msg := runScan(t, dir)
	id := msg["id"].(string)

	cases := map[string]string{
		"scan root":   dir,
		"outside":     filepath.Join(t.TempDir(), "other"),
		"protected":   "/etc/passwd",
		"not in tree": filepath.Join(dir, "missing"),
		"traversal":   filepath.Join(dir, "..", "x"),
	}
	for name, p := range cases {
		if w := postDelete(id, p); w.Code != http.StatusForbidden {
			t.Errorf("%s: expected 403, got %d (%s)", name, w.Code, w.Body.String())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "f")); err != nil {
		t.Error("file should still exist")
	}
}

func TestIsProtected(t *testing.T) {
	for _, p := range []string{"/", "/usr", "/etc/ssh", "/proc/1", "/var/lib"} {
		if !isProtected(p) {
			t.Errorf("%s should be protected", p)
		}
	}
	for _, p := range []string{"/var/lib/docker/overlay2", "/home/user/junk", "/srv/data/old"} {
		if isProtected(p) {
			t.Errorf("%s should not be protected", p)
		}
	}
}
