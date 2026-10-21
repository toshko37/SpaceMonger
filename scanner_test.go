package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func scanTemp(t *testing.T, dir string) ScanSnapshot {
	t.Helper()
	s := &Scanner{}
	s.Scan(context.Background(), dir)
	return s.Snapshot()
}

func TestScanner_singleFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello world"), 0644)

	snap := scanTemp(t, dir)
	if snap.Root == nil || !snap.Done || snap.Err != nil {
		t.Fatalf("bad snapshot: %+v", snap)
	}
	if len(snap.Root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(snap.Root.Children))
	}
	c := snap.Root.Children[0]
	// Size is on-disk allocation (st.Blocks*512): >= content and 512-aligned.
	if c.Size < 11 || c.Size%512 != 0 {
		t.Errorf("unexpected size %d", c.Size)
	}
	if c.IsDir || c.Name != "test.txt" || c.Mtime == 0 {
		t.Errorf("unexpected node %+v", c)
	}
	if snap.Files != 1 || snap.Dirs != 1 {
		t.Errorf("expected files=1 dirs=1, got %d/%d", snap.Files, snap.Dirs)
	}
	if snap.Apparent != 11 {
		t.Errorf("expected apparent=11, got %d", snap.Apparent)
	}
}

func TestScanner_dirSizeIsSum(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), make([]byte, 100), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), make([]byte, 200), 0644)

	snap := scanTemp(t, dir)
	var sum int64
	for _, c := range snap.Root.Children {
		sum += c.Size
	}
	if snap.Root.Size != sum || snap.Root.Size < 300 {
		t.Errorf("root size %d != children sum %d", snap.Root.Size, sum)
	}
	if snap.Files != 2 {
		t.Errorf("expected files=2, got %d", snap.Files)
	}
}

func TestScanner_nested(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	os.Mkdir(sub, 0755)
	os.WriteFile(filepath.Join(sub, "file.txt"), make([]byte, 500), 0644)

	snap := scanTemp(t, dir)
	if snap.Root.Size < 500 || snap.Dirs != 2 || snap.Files != 1 {
		t.Errorf("unexpected: size=%d dirs=%d files=%d", snap.Root.Size, snap.Dirs, snap.Files)
	}
	if len(snap.Root.Children) != 1 || !snap.Root.Children[0].IsDir {
		t.Fatalf("expected one dir child, got %+v", snap.Root.Children)
	}
}

func TestScanner_skipsSymlinks(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "real.txt"), make([]byte, 100), 0644)
	os.Symlink(filepath.Join(dir, "real.txt"), filepath.Join(dir, "link.txt"))
	os.Symlink(dir, filepath.Join(dir, "loop"))

	snap := scanTemp(t, dir)
	if snap.Files != 1 || len(snap.Root.Children) != 1 {
		t.Errorf("symlinks should be skipped: files=%d children=%d", snap.Files, len(snap.Root.Children))
	}
}

func TestScanner_hardLinksCountedOnce(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.bin")
	os.WriteFile(a, make([]byte, 4096), 0644)
	if err := os.Link(a, filepath.Join(dir, "b.bin")); err != nil {
		t.Skip("hard links not supported here")
	}
	snap := scanTemp(t, dir)
	if snap.Root.Size != 4096 || snap.Apparent != 4096 {
		t.Errorf("expected 4096 counted once, got size=%d apparent=%d", snap.Root.Size, snap.Apparent)
	}
}

func TestScanner_missingPathSetsError(t *testing.T) {
	snap := scanTemp(t, "/nonexistent/path/xyz")
	if snap.Root != nil || snap.Err == nil || !snap.Done {
		t.Errorf("expected nil root with error, got %+v", snap)
	}
}

func TestScanner_cancelled(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		os.WriteFile(filepath.Join(dir, "f"+string(rune('a'+i))), []byte("x"), 0644)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before the scan starts

	s := &Scanner{}
	s.Scan(ctx, dir)
	snap := s.Snapshot()
	if snap.Root != nil || snap.Err != errScanCancelled {
		t.Errorf("expected cancelled scan with nil root, got root=%v err=%v", snap.Root, snap.Err)
	}
}
