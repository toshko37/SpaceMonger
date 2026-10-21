package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Paths that are never deletable from the UI, whatever settings say.
var protectedExact = []string{
	"/", "/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/lib32", "/lib64",
	"/opt", "/proc", "/root", "/run", "/sbin", "/srv", "/sys", "/tmp", "/usr", "/var",
	"/usr/bin", "/usr/lib", "/usr/sbin", "/usr/share", "/var/lib", "/var/log",
}
var protectedPrefixes = []string{"/proc/", "/sys/", "/dev/", "/boot/", "/etc/"}

var (
	errDeleteDisabled = errors.New("deleting is disabled; set \"allowDelete\": true in settings.json")
	errProtected      = errors.New("path is protected")
	errOutsideScan    = errors.New("path is outside the scanned tree")
)

func isProtected(p string) bool {
	for _, e := range protectedExact {
		if p == e {
			return true
		}
	}
	for _, pre := range protectedPrefixes {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
}

// relativeTo returns p relative to root when p is root or inside it.
func relativeTo(root, p string) (string, bool) {
	if p == root {
		return "", true
	}
	prefix := strings.TrimSuffix(root, "/") + "/"
	if !strings.HasPrefix(p, prefix) {
		return "", false
	}
	return strings.TrimPrefix(p, prefix), true
}

type deleteResponse struct {
	Freed int64 `json:"freed"`
	Files int64 `json:"files"`
	Dirs  int64 `json:"dirs"`
}

// deleteFromScan removes the path from disk and from the stored tree. The
// path must exist in the scan (so the user actually saw it) and must not
// be the scan root or a protected system path.
func deleteFromScan(id, path string) (deleteResponse, error) {
	var resp deleteResponse
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) || isProtected(path) {
		return resp, errProtected
	}
	err := store.Write(id, func(r *ScanResult) error {
		rel, ok := relativeTo(r.Snap.Path, path)
		if !ok || rel == "" {
			return errOutsideScan
		}
		node, ancestors, found := findNode(r.Snap.Root, rel)
		if !found {
			return errOutsideScan
		}
		// Refuse to follow a symlink that appeared after the scan.
		if fi, err := os.Lstat(path); err != nil {
			return err
		} else if fi.Mode()&os.ModeSymlink != 0 {
			return errProtected
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		resp.Files, resp.Dirs = countNodes(node)
		resp.Freed = node.Size
		removeNode(&r.Snap, node, ancestors)
		return nil
	})
	return resp, err
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !getSettings().AllowDelete {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": errDeleteDisabled.Error()})
		return
	}
	var req struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil || req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	resp, err := deleteFromScan(req.ID, req.Path)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, errNotFound):
			status = http.StatusNotFound
		case errors.Is(err, errProtected), errors.Is(err, errOutsideScan):
			status = http.StatusForbidden
		case os.IsPermission(err):
			status = http.StatusForbidden
		case os.IsNotExist(err):
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
