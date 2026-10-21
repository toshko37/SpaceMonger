package main

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExport_csvAndJSON(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "f.txt"), []byte("hello"), 0644)
	_, msg := runScan(t, dir)
	id := msg["id"].(string)

	w := httptest.NewRecorder()
	exportHandler(w, httptest.NewRequest(http.MethodGet, "/api/export?id="+id+"&format=csv", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Disposition"), ".csv") {
		t.Fatalf("csv export failed: %d %s", w.Code, w.Header().Get("Content-Disposition"))
	}
	rows, err := csv.NewReader(w.Body).ReadAll()
	if err != nil || len(rows) != 4 { // header + root + sub + f.txt
		t.Fatalf("unexpected csv rows: %v %v", err, rows)
	}
	if rows[3][0] != filepath.Join(dir, "sub", "f.txt") || rows[3][1] != "file" {
		t.Errorf("unexpected row %v", rows[3])
	}

	// Subtree export, relative path, JSON.
	w = httptest.NewRecorder()
	exportHandler(w, httptest.NewRequest(http.MethodGet, "/api/export?id="+id+"&format=json&path=sub", nil))
	var out struct {
		Path string    `json:"path"`
		Root *FileNode `json:"root"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Root.Name != "sub" || out.Path != filepath.Join(dir, "sub") {
		t.Errorf("unexpected json export: %v %+v", err, out)
	}

	w = httptest.NewRecorder()
	exportHandler(w, httptest.NewRequest(http.MethodGet, "/api/export?id="+id+"&path=../x", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for traversal, got %d", w.Code)
	}
}
