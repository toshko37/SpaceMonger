package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

// Names that exercise every escaping branch, including invalid UTF-8.
var trickyNames = []string{
	"plain.txt", `quo"te`, `back\slash`, "new\nline", "tab\there", "cr\rx",
	"bell\x07", "nul\x00byte", "<script>&amp;", "кирилица.txt", "emoji 🚀",
	"line sep ", "bad\xffutf8", "trunc\xe2\x82", "",
}

func trickyTree() *FileNode {
	root := &FileNode{Name: "root", IsDir: true, Mtime: 1700000000, Size: 0}
	sub := &FileNode{Name: "sub dir", IsDir: true, Mtime: -5}
	for i, n := range trickyNames {
		f := &FileNode{Name: n, Size: int64(i * 512), Mtime: int64(1600000000 + i)}
		if i%2 == 0 {
			root.Children = append(root.Children, f)
		} else {
			sub.Children = append(sub.Children, f)
		}
	}
	root.Children = append(root.Children, sub, &FileNode{Name: "empty", IsDir: true})
	return root
}

// decode normalises JSON so the comparison ignores key order and escapes.
func decode(t *testing.T, b []byte) interface{} {
	t.Helper()
	if !json.Valid(b) {
		t.Fatalf("invalid JSON: %q", b)
	}
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestWriteTree_matchesEncodingJSON(t *testing.T) {
	tree := trickyTree()
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := writeTree(w, tree); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	want, _ := json.Marshal(tree)
	if !reflect.DeepEqual(decode(t, buf.Bytes()), decode(t, want)) {
		t.Errorf("streamed tree differs from encoding/json\n got: %s\nwant: %s", buf.Bytes(), want)
	}
}

func TestWriteJSONString_escapesLikeEncodingJSON(t *testing.T) {
	for _, s := range trickyNames {
		var buf bytes.Buffer
		w := bufio.NewWriter(&buf)
		writeJSONString(w, s)
		w.Flush()
		want, _ := json.Marshal(s)
		if !reflect.DeepEqual(decode(t, buf.Bytes()), decode(t, want)) {
			t.Errorf("%q: got %s want %s", s, buf.Bytes(), want)
		}
		if bytes.ContainsAny(buf.Bytes(), "<>&") {
			t.Errorf("%q: HTML characters must be escaped: %s", s, buf.Bytes())
		}
	}
}

func TestWriteObjectWithTree(t *testing.T) {
	tree := trickyTree()
	meta := map[string]interface{}{"id": "abc", "files": 3, "error": ""}
	var buf bytes.Buffer
	if err := writeObjectWithTree(&buf, meta, "root", tree); err != nil {
		t.Fatal(err)
	}
	full := map[string]interface{}{"id": "abc", "files": 3, "error": "", "root": tree}
	want, _ := json.Marshal(full)
	if !reflect.DeepEqual(decode(t, buf.Bytes()), decode(t, want)) {
		t.Errorf("object differs\n got: %s\nwant: %s", buf.Bytes(), want)
	}

	// Empty metadata and nil tree are still valid JSON.
	buf.Reset()
	writeObjectWithTree(&buf, map[string]interface{}{}, "root", nil)
	if got := decode(t, buf.Bytes()); !reflect.DeepEqual(got, map[string]interface{}{"root": nil}) {
		t.Errorf("unexpected %v", got)
	}
}

// The streaming path must not allocate in proportion to the tree size.
func TestWriteTree_allocationsIndependentOfSize(t *testing.T) {
	big := &FileNode{Name: "big", IsDir: true}
	for i := 0; i < 20000; i++ {
		big.Children = append(big.Children, &FileNode{Name: "file-name-" + string(rune('a'+i%26)), Size: int64(i)})
	}
	w := bufio.NewWriterSize(discard{}, streamBufSize)
	allocs := testing.AllocsPerRun(5, func() { writeTree(w, big); w.Flush() })
	if allocs > 10 {
		t.Errorf("expected near-zero allocations, got %.0f for 20k nodes", allocs)
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
