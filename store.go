package main

import (
	"errors"
	"strings"
	"sync"
	"syscall"
)

// ScanResult is a completed scan kept in memory.
type ScanResult struct {
	ID   string
	Snap ScanSnapshot
}

// ScanMeta is the lightweight listing entry for /api/scans.
type ScanMeta struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	Files      int64  `json:"files"`
	Dirs       int64  `json:"dirs"`
	ScannedAt  int64  `json:"scannedAt"`
	DurationMs int64  `json:"durationMs"`
}

// ScanStore keeps the last N completed scans, newest first. All access to
// a stored tree goes through the store lock because deletions mutate it.
type ScanStore struct {
	mu    sync.RWMutex
	items []*ScanResult
	max   int
}

var store = &ScanStore{max: defaultHistorySize}

var errNotFound = errors.New("scan not found")

func (st *ScanStore) SetMax(n int) {
	st.mu.Lock()
	st.max = n
	st.trim()
	st.mu.Unlock()
}

func (st *ScanStore) trim() {
	if st.max > 0 && len(st.items) > st.max {
		st.items = st.items[:st.max]
	}
}

func (st *ScanStore) Add(snap ScanSnapshot) *ScanResult {
	r := &ScanResult{ID: genToken()[:12], Snap: snap}
	st.mu.Lock()
	st.items = append([]*ScanResult{r}, st.items...)
	st.trim()
	st.mu.Unlock()
	return r
}

func (st *ScanStore) find(id string) *ScanResult {
	if id == "" {
		if len(st.items) == 0 {
			return nil
		}
		return st.items[0]
	}
	for _, r := range st.items {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// Read runs fn with the result under the read lock; id "" means latest.
func (st *ScanStore) Read(id string, fn func(*ScanResult)) error {
	st.mu.RLock()
	defer st.mu.RUnlock()
	r := st.find(id)
	if r == nil {
		return errNotFound
	}
	fn(r)
	return nil
}

// Write runs fn with the result under the write lock.
func (st *ScanStore) Write(id string, fn func(*ScanResult) error) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	r := st.find(id)
	if r == nil {
		return errNotFound
	}
	return fn(r)
}

func (st *ScanStore) Remove(id string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	for i, r := range st.items {
		if r.ID == id {
			st.items = append(st.items[:i], st.items[i+1:]...)
			return true
		}
	}
	return false
}

func (st *ScanStore) Len() int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return len(st.items)
}

// Clear forgets every stored scan.
func (st *ScanStore) Clear() {
	st.mu.Lock()
	st.items = nil
	st.mu.Unlock()
}

func (st *ScanStore) List() []ScanMeta {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]ScanMeta, 0, len(st.items))
	for _, r := range st.items {
		out = append(out, metaOf(r))
	}
	return out
}

func metaOf(r *ScanResult) ScanMeta {
	s := r.Snap
	return ScanMeta{
		ID: r.ID, Path: s.Path, Size: s.Root.Size, Files: s.Files, Dirs: s.Dirs,
		ScannedAt: s.FinishedAt.Unix(), DurationMs: s.FinishedAt.Sub(s.StartedAt).Milliseconds(),
	}
}

// ─── Tree helpers ─────────────────────────────────────────────────────────────

// findNode walks the tree by the path relative to the scan root and returns
// the node and its ancestors (root first). rel "" returns the root.
func findNode(root *FileNode, rel string) (node *FileNode, ancestors []*FileNode, ok bool) {
	node = root
	if rel == "" || rel == "." {
		return node, nil, true
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" {
			continue
		}
		var next *FileNode
		for _, c := range node.Children {
			if c.Name == seg {
				next = c
				break
			}
		}
		if next == nil {
			return nil, nil, false
		}
		ancestors = append(ancestors, node)
		node = next
	}
	return node, ancestors, true
}

// countNodes returns the number of files and directories in a subtree,
// including the node itself.
func countNodes(n *FileNode) (files, dirs int64) {
	if !n.IsDir {
		return 1, 0
	}
	dirs = 1
	for _, c := range n.Children {
		f, d := countNodes(c)
		files += f
		dirs += d
	}
	return
}

// removeNode detaches node from the tree, subtracting its size from every
// ancestor and adjusting the snapshot counters and free space.
func removeNode(snap *ScanSnapshot, node *FileNode, ancestors []*FileNode) {
	if len(ancestors) == 0 {
		return
	}
	parent := ancestors[len(ancestors)-1]
	for i, c := range parent.Children {
		if c == node {
			parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
			break
		}
	}
	for _, a := range ancestors {
		a.Size -= node.Size
	}
	f, d := countNodes(node)
	snap.Files -= f
	snap.Dirs -= d
	var fsStat syscall.Statfs_t
	if err := syscall.Statfs(snap.Path, &fsStat); err == nil {
		snap.FreeDisk = int64(fsStat.Bavail) * int64(fsStat.Bsize)
	}
}
