package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// FileNode is one entry of the scanned tree. JSON keys are deliberately
// short: a scan of a few million files produces a tree of tens of MB, and
// key names dominate the payload.
type FileNode struct {
	Name     string      `json:"n"`
	Size     int64       `json:"s"` // bytes actually allocated on disk
	Mtime    int64       `json:"m"` // unix seconds
	IsDir    bool        `json:"d,omitempty"`
	Children []*FileNode `json:"c,omitempty"`
}

// ScanSnapshot is a consistent copy of the scanner state for handlers.
type ScanSnapshot struct {
	Path       string
	Root       *FileNode
	Files      int64
	Dirs       int64
	Current    string
	Done       bool
	Err        error
	TotalDisk  int64
	FreeDisk   int64
	Apparent   int64 // sum of logical sizes; compare to Root.Size for sparse/compression
	StartedAt  time.Time
	FinishedAt time.Time
}

type Scanner struct {
	mu            sync.RWMutex
	path          string
	root          *FileNode
	files         int64
	dirs          int64
	current       string
	done          bool
	scanErr       error
	totalDisk     int64
	freeDisk      int64
	totalApparent int64
	startedAt     time.Time
	finishedAt    time.Time

	// Scan-goroutine-only state; never touched by readers, so no lock.
	ctx        context.Context
	rootDev    uint64             // device ID of scan root; stay on one filesystem
	seenInodes map[[2]uint64]bool // (dev, ino) of hard-linked files already counted
}

var errScanCancelled = errors.New("scan cancelled")

// Scan walks path and stores the result. It returns early when ctx is
// cancelled (client disconnected or a newer scan superseded this one).
func (s *Scanner) Scan(ctx context.Context, path string) {
	s.mu.Lock()
	s.path = path
	s.root = nil
	s.files, s.dirs = 0, 0
	s.current = path
	s.done = false
	s.scanErr = nil
	s.totalDisk, s.freeDisk, s.totalApparent = 0, 0, 0
	s.startedAt = time.Now()
	s.finishedAt = time.Time{}
	s.mu.Unlock()

	s.ctx = ctx
	s.rootDev = 0
	s.seenInodes = make(map[[2]uint64]bool)

	var rootStat syscall.Stat_t
	if err := syscall.Stat(path, &rootStat); err == nil {
		s.rootDev = rootStat.Dev
	}

	var fsStat syscall.Statfs_t
	if err := syscall.Statfs(path, &fsStat); err == nil {
		s.mu.Lock()
		s.totalDisk = int64(fsStat.Blocks) * int64(fsStat.Bsize)
		s.freeDisk = int64(fsStat.Bavail) * int64(fsStat.Bsize)
		s.mu.Unlock()
	}

	root, err := s.scanDir(path)
	if errors.Is(err, context.Canceled) {
		root, err = nil, errScanCancelled
	}

	s.mu.Lock()
	s.root = root
	s.done = true
	s.scanErr = err
	s.finishedAt = time.Now()
	s.seenInodes = nil // release memory
	s.mu.Unlock()
}

func (s *Scanner) Snapshot() ScanSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return ScanSnapshot{
		Path: s.path, Root: s.root, Files: s.files, Dirs: s.dirs,
		Current: s.current, Done: s.done, Err: s.scanErr,
		TotalDisk: s.totalDisk, FreeDisk: s.freeDisk, Apparent: s.totalApparent,
		StartedAt: s.startedAt, FinishedAt: s.finishedAt,
	}
}

// fileNode builds a leaf node. Size is the on-disk footprint (st.Blocks is
// always in 512-byte units on Linux), which accounts for sparse files and
// transparent compression. Hard links are counted once.
func (s *Scanner) fileNode(name string, info os.FileInfo, apparentSum *int64) *FileNode {
	apparent := info.Size()
	actual := apparent
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		actual = st.Blocks * 512
		if st.Nlink > 1 {
			key := [2]uint64{st.Dev, st.Ino}
			if s.seenInodes[key] {
				actual, apparent = 0, 0
			} else {
				s.seenInodes[key] = true
			}
		}
	}
	*apparentSum += apparent
	return &FileNode{Name: name, Size: actual, Mtime: info.ModTime().Unix()}
}

func (s *Scanner) scanDir(path string) (*FileNode, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		var apparent int64
		n := s.fileNode(filepath.Base(path), info, &apparent)
		s.mu.Lock()
		s.files++
		s.totalApparent += apparent
		s.mu.Unlock()
		return n, nil
	}

	node := &FileNode{Name: filepath.Base(path), Mtime: info.ModTime().Unix(), IsDir: true}

	s.mu.Lock()
	s.dirs++
	s.current = path
	s.mu.Unlock()

	entries, err := os.ReadDir(path)
	if err != nil {
		return node, nil // unreadable dir: keep it as empty, don't abort the scan
	}

	var nFiles, apparentSum int64
	for _, entry := range entries {
		if s.ctx != nil && s.ctx.Err() != nil {
			return node, s.ctx.Err()
		}
		name := entry.Name()
		if entry.Type()&os.ModeSymlink != 0 {
			continue // never follow symlinks: avoids loops and cross-fs traversal
		}
		childPath := filepath.Join(path, name)

		if entry.IsDir() {
			// .zfs exposes snapshot trees whose blocks overlap the live data.
			if name == ".zfs" {
				continue
			}
			// Skip mount points of other filesystems (/proc, /sys, /dev, ...).
			if s.rootDev != 0 {
				var st syscall.Stat_t
				if err := syscall.Stat(childPath, &st); err == nil && st.Dev != s.rootDev {
					continue
				}
			}
			child, err := s.scanDir(childPath)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return node, err
				}
				continue
			}
			node.Children = append(node.Children, child)
			node.Size += child.Size
			continue
		}

		fi, err := entry.Info()
		if err != nil {
			continue
		}
		child := s.fileNode(name, fi, &apparentSum)
		nFiles++
		node.Children = append(node.Children, child)
		node.Size += child.Size
	}

	// One lock per directory instead of one per file.
	s.mu.Lock()
	s.files += nFiles
	s.totalApparent += apparentSum
	s.mu.Unlock()
	return node, nil
}
