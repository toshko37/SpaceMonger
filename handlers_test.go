package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runScan(t *testing.T, path string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/scan?path="+url.QueryEscape(path), nil)
	w := httptest.NewRecorder()
	scanHandler(w, req)

	var last string
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if strings.HasPrefix(line, "data: ") {
			last = strings.TrimPrefix(line, "data: ")
		}
	}
	var msg map[string]interface{}
	if err := json.Unmarshal([]byte(last), &msg); err != nil {
		t.Fatalf("bad last SSE event %q: %v", last, err)
	}
	return w, msg
}

func TestScanHandler_completesWithDone(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("data"), 0644)

	w, msg := runScan(t, dir)
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected SSE content type, got %q", ct)
	}
	if msg["status"] != "done" || msg["error"] != "" || msg["root"] == nil {
		t.Errorf("unexpected done event: %v", msg)
	}
	if msg["path"] != dir {
		t.Errorf("expected path %q, got %v", dir, msg["path"])
	}
}

func TestScanHandler_missingPathParam(t *testing.T) {
	w := httptest.NewRecorder()
	scanHandler(w, httptest.NewRequest(http.MethodGet, "/api/scan", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestScanHandler_pathNormalization(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)
	messy := dir + "//subdir/../subdir"

	_, msg := runScan(t, messy)
	if msg["path"] != filepath.Clean(messy) {
		t.Errorf("expected cleaned path %q, got %v", filepath.Clean(messy), msg["path"])
	}
}

func TestScanHandler_errorFieldInDoneEvent(t *testing.T) {
	_, msg := runScan(t, "/nonexistent/path/does/not/exist/xyz")
	if msg["status"] != "done" {
		t.Errorf("expected status=done, got %v", msg["status"])
	}
	if s, _ := msg["error"].(string); s == "" {
		t.Error("expected non-empty error for non-existent path")
	}
}

func TestScanHandler_newScanSupersedesRunning(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0644)

	// Simulate a running scan holding the lock with a registered cancel.
	ctx, cancel := acquireScan(httptest.NewRequest(http.MethodGet, "/", nil).Context())
	go func() {
		<-ctx.Done() // the next acquireScan must cancel us
		cancel()
		scanMu.Unlock()
	}()

	_, msg := runScan(t, dir) // must not block or 409
	if msg["status"] != "done" || msg["error"] != "" {
		t.Errorf("expected successful superseding scan, got %v", msg)
	}
}

func TestDataHandler_returnsLastScan(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a"), []byte("abc"), 0644)
	runScan(t, dir)

	w := httptest.NewRecorder()
	dataHandler(w, httptest.NewRequest(http.MethodGet, "/api/data", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var msg map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &msg)
	if msg["path"] != dir || msg["root"] == nil {
		t.Errorf("unexpected data payload: %v", msg)
	}
}

func TestDataHandler_noneWhenNoScan(t *testing.T) {
	runScan(t, "/nonexistent/path/xyz") // leaves root == nil
	w := httptest.NewRecorder()
	dataHandler(w, httptest.NewRequest(http.MethodGet, "/api/data", nil))
	var msg map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &msg)
	if w.Code != http.StatusOK || msg["status"] != "none" || msg["root"] != nil {
		t.Errorf("expected status none with nil root, got %d %v", w.Code, msg)
	}
}

func TestFileNode_compactJSON(t *testing.T) {
	n := &FileNode{Name: "x", Size: 1, Mtime: 2}
	b, _ := json.Marshal(n)
	if string(b) != `{"n":"x","s":1,"m":2}` {
		t.Errorf("unexpected JSON %s", b)
	}
}
