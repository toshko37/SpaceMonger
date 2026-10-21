package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// exportHandler streams a subtree of a stored scan as CSV or JSON.
// GET /api/export?id=&path=<absolute or relative>&format=csv|json
func exportHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	format := q.Get("format")
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "json" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "format must be csv or json"})
		return
	}

	err := store.Read(q.Get("id"), func(res *ScanResult) {
		rel, abs, ok := resolveExportPath(res.Snap.Path, q.Get("path"))
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path outside scan"})
			return
		}
		node, _, found := findNode(res.Snap.Root, rel)
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "path not found in scan"})
			return
		}
		base := strings.Trim(strings.ReplaceAll(strings.TrimPrefix(abs, "/"), "/", "_"), "_")
		if base == "" {
			base = "root"
		}
		name := fmt.Sprintf("spacemonger_%s_%s.%s", base, res.Snap.FinishedAt.Format("20060102-1504"), format)
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)

		if format == "json" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"path": abs, "scannedAt": res.Snap.FinishedAt.Unix(), "root": node,
			})
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		cw := csv.NewWriter(w)
		cw.Write([]string{"path", "type", "size_bytes", "modified"})
		var walk func(n *FileNode, p string)
		walk = func(n *FileNode, p string) {
			kind := "file"
			if n.IsDir {
				kind = "dir"
			}
			cw.Write([]string{p, kind, strconv.FormatInt(n.Size, 10), time.Unix(n.Mtime, 0).UTC().Format(time.RFC3339)})
			for _, c := range n.Children {
				walk(c, filepath.Join(p, c.Name))
			}
		}
		walk(node, abs)
		cw.Flush()
	})
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	}
}

// resolveExportPath accepts "" (root), an absolute path inside the scan, or
// a path relative to the scan root; returns (relative, absolute, ok).
func resolveExportPath(root, p string) (string, string, bool) {
	if p == "" {
		return "", root, true
	}
	if filepath.IsAbs(p) {
		p = filepath.Clean(p)
		rel, ok := relativeTo(root, p)
		return rel, p, ok
	}
	p = filepath.Clean(p)
	if p == "." {
		return "", root, true
	}
	if strings.HasPrefix(p, "../") || p == ".." {
		return "", "", false
	}
	return p, filepath.Join(root, p), true
}
