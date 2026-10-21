package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCompareTrees(t *testing.T) {
	a := &FileNode{Name: "r", IsDir: true, Size: 30, Children: []*FileNode{
		{Name: "same", Size: 10},
		{Name: "grew", Size: 10},
		{Name: "gone", IsDir: true, Size: 10, Children: []*FileNode{{Name: "inner", Size: 10}}},
	}}
	b := &FileNode{Name: "r", IsDir: true, Size: 70, Children: []*FileNode{
		{Name: "same", Size: 10},
		{Name: "grew", Size: 20},
		{Name: "new", IsDir: true, Size: 40, Children: []*FileNode{{Name: "big", Size: 40}}},
	}}
	changes, sum := compareTrees(a, b, 0)
	if sum.Delta != 40 || sum.Added != 1 || sum.Removed != 1 || sum.Changed != 2 { // root + grew
		t.Errorf("unexpected summary %+v", sum)
	}
	byPath := map[string]Change{}
	for _, c := range changes {
		byPath[c.Path] = c
	}
	if byPath["new"].Delta != 40 || !byPath["new"].Added || byPath["grew"].Delta != 10 || byPath["gone"].Delta != -10 || !byPath["gone"].Gone {
		t.Errorf("unexpected changes %+v", changes)
	}
	if _, ok := byPath["gone/inner"]; ok {
		t.Error("descendants of a removed node should not be listed")
	}
	if _, ok := byPath["same"]; ok {
		t.Error("unchanged node should not be listed")
	}
	if changes[0].Path != "" && changes[0].Path != "new" { // root (+40) and new (+40) tie
		t.Errorf("expected largest delta first, got %+v", changes[0])
	}
}

func TestCompareHandler(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a"), make([]byte, 1000), 0644)
	_, first := runScan(t, dir)
	os.WriteFile(filepath.Join(dir, "b"), make([]byte, 100000), 0644)
	_, second := runScan(t, dir)

	req := httptest.NewRequest(http.MethodGet,
		"/api/compare?a="+first["id"].(string)+"&b="+second["id"].(string), nil)
	w := httptest.NewRecorder()
	compareHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Summary CompareSummary `json:"summary"`
		Changes []Change       `json:"changes"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Summary.Added != 1 || resp.Summary.Delta <= 0 {
		t.Errorf("unexpected summary %+v", resp.Summary)
	}
	found := false
	for _, c := range resp.Changes {
		if c.Path == "b" && c.Before == 0 && c.After > 0 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected added file b in changes: %+v", resp.Changes)
	}
}
