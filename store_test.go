package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func snapOf(root *FileNode, path string) ScanSnapshot {
	f, d := countNodes(root)
	return ScanSnapshot{Path: path, Root: root, Files: f, Dirs: d, Done: true,
		StartedAt: time.Now(), FinishedAt: time.Now()}
}

func TestStore_capAndOrder(t *testing.T) {
	st := &ScanStore{max: 2}
	a := st.Add(snapOf(&FileNode{Name: "a", IsDir: true}, "/a"))
	b := st.Add(snapOf(&FileNode{Name: "b", IsDir: true}, "/b"))
	c := st.Add(snapOf(&FileNode{Name: "c", IsDir: true}, "/c"))
	list := st.List()
	if len(list) != 2 || list[0].ID != c.ID || list[1].ID != b.ID {
		t.Errorf("expected [c b], got %+v", list)
	}
	if err := st.Read(a.ID, func(*ScanResult) {}); err == nil {
		t.Error("oldest scan should have been evicted")
	}
	if err := st.Read("", func(r *ScanResult) {
		if r.ID != c.ID {
			t.Errorf("latest should be c, got %s", r.ID)
		}
	}); err != nil {
		t.Error(err)
	}
	if !st.Remove(b.ID) || len(st.List()) != 1 {
		t.Error("remove failed")
	}
}

func TestFindNode(t *testing.T) {
	root := &FileNode{Name: "r", IsDir: true, Children: []*FileNode{
		{Name: "x", IsDir: true, Children: []*FileNode{{Name: "f", Size: 5}}},
	}}
	n, anc, ok := findNode(root, "x/f")
	if !ok || n.Name != "f" || len(anc) != 2 {
		t.Errorf("findNode failed: %v %v %v", ok, n, anc)
	}
	if _, _, ok := findNode(root, "x/nope"); ok {
		t.Error("expected not found")
	}
}

func TestScansHandler_listAndDelete(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a"), []byte("abc"), 0644)
	_, msg := runScan(t, dir)
	id := msg["id"].(string)

	w := httptest.NewRecorder()
	scansHandler(w, httptest.NewRequest(http.MethodGet, "/api/scans", nil))
	var list []ScanMeta
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) == 0 || list[0].ID != id || list[0].Path != dir {
		t.Fatalf("unexpected list: %+v", list)
	}

	w = httptest.NewRecorder()
	scansHandler(w, httptest.NewRequest(http.MethodDelete, "/api/scans?id="+id, nil))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	dataHandler(w, httptest.NewRequest(http.MethodGet, "/api/data?id="+id, nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 after removal, got %d", w.Code)
	}
}
