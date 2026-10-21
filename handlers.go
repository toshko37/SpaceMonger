package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ─── gzip ─────────────────────────────────────────────────────────────────────

type gzipWriter struct {
	http.ResponseWriter
	gz io.Writer
}

func (g gzipWriter) Write(b []byte) (int, error) { return g.gz.Write(b) }

// gzipMiddleware compresses responses for clients that accept it. Used for
// the large tree payloads (/api/data, /api/export): names compress 5-10x.
func gzipMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		gz, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
		defer gz.Close()
		next(gzipWriter{ResponseWriter: w, gz: gz}, r)
	}
}

// ─── Simple endpoints ─────────────────────────────────────────────────────────

func versionHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": version})
}

// configHandler exposes the settings the UI needs to adapt to.
func configHandler(w http.ResponseWriter, r *http.Request) {
	cfg := getSettings()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":     version,
		"allowDelete": cfg.AllowDelete,
		"historySize": cfg.HistorySize,
		"exclude":     cfg.Exclude,
		"tls":         cfg.TLS.Enabled(),
	})
}

func mountsHandler(w http.ResponseWriter, r *http.Request) {
	mounts, err := getMounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if mounts == nil {
		mounts = []Mount{}
	}
	writeJSON(w, http.StatusOK, mounts)
}

// ─── Scan results ─────────────────────────────────────────────────────────────

// resultPayload is the JSON shape of a finished scan. withRoot=false yields
// the metadata only (used by the SSE "done" event; the tree itself is then
// fetched gzip-compressed from /api/data).
func resultPayload(id string, snap ScanSnapshot, withRoot bool) map[string]interface{} {
	errMsg := ""
	if snap.Err != nil {
		errMsg = snap.Err.Error()
	}
	m := map[string]interface{}{
		"status":       "done",
		"id":           id,
		"path":         snap.Path,
		"files":        snap.Files,
		"dirs":         snap.Dirs,
		"totalDisk":    snap.TotalDisk,
		"freeDisk":     snap.FreeDisk,
		"apparentSize": snap.Apparent,
		"scannedAt":    snap.FinishedAt.Unix(),
		"durationMs":   snap.FinishedAt.Sub(snap.StartedAt).Milliseconds(),
		"error":        errMsg,
	}
	if snap.Root != nil {
		m["size"] = snap.Root.Size
	}
	if withRoot {
		m["root"] = snap.Root
	}
	return m
}

// dataHandler returns a stored scan (latest by default, or ?id=).
func dataHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	err := store.Read(id, func(res *ScanResult) {
		writeJSON(w, http.StatusOK, resultPayload(res.ID, res.Snap, true))
	})
	if err != nil {
		if id == "" {
			writeJSON(w, http.StatusOK, map[string]interface{}{"status": "none", "root": nil})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	}
}

// scansHandler: GET lists stored scans; DELETE ?id= removes one.
func scansHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, store.List())
	case http.MethodDelete:
		if !store.Remove(r.URL.Query().Get("id")) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "scan not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// compareHandler: GET ?a=<older id>&b=<newer id>&limit=300
func compareHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 5000 {
		limit = 300
	}
	var a, b *ScanResult
	err := store.Read(q.Get("a"), func(res *ScanResult) {
		a = res
		if e := store.find(q.Get("b")); e != nil { // same lock already held
			b = e
		}
	})
	if err != nil || b == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "scan not found"})
		return
	}
	if a.Snap.Path != b.Snap.Path {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scans are of different paths"})
		return
	}
	store.mu.RLock()
	changes, sum := compareTrees(a.Snap.Root, b.Snap.Root, limit)
	store.mu.RUnlock()
	if changes == nil {
		changes = []Change{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"a": metaOf(a), "b": metaOf(b), "summary": sum, "changes": changes,
	})
}

// ─── Scan (SSE) ───────────────────────────────────────────────────────────────

var (
	globalScanner = &Scanner{}
	scanMu        sync.Mutex         // held while a scan goroutine runs
	activeCancel  context.CancelFunc // cancels the running scan, if any
	activeMu      sync.Mutex
)

// acquireScan cancels any running scan and waits for it to release scanMu.
// A new scan request always wins: a reloaded page must not be stuck behind
// a scan whose client has gone away.
func acquireScan(parent context.Context) (context.Context, context.CancelFunc) {
	activeMu.Lock()
	if activeCancel != nil {
		activeCancel()
	}
	activeMu.Unlock()

	scanMu.Lock()
	ctx, cancel := context.WithCancel(parent)
	activeMu.Lock()
	activeCancel = cancel
	activeMu.Unlock()
	return ctx, cancel
}

func scanHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path parameter required"})
		return
	}
	path = filepath.Clean(path)

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	ctx, cancel := acquireScan(r.Context())
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")

	cfg := getSettings()
	globalScanner.Exclude = cfg.Exclude
	globalScanner.Workers = cfg.effectiveWorkers()

	done := make(chan *ScanResult, 1)
	go func() {
		defer scanMu.Unlock()
		defer close(done)
		globalScanner.Scan(ctx, path)
		snap := globalScanner.Snapshot()
		if snap.Root != nil && snap.Err == nil {
			done <- store.Add(snap)
		}
	}()

	send := func(v interface{}) {
		data, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Client disconnected or superseded by a newer scan; the scan
			// goroutine notices the cancelled ctx and exits on its own.
			return
		case res, ok := <-done:
			if !ok || res == nil { // scan failed or was cancelled
				send(resultPayload("", globalScanner.Snapshot(), false))
				return
			}
			send(resultPayload(res.ID, res.Snap, false))
			return
		case <-ticker.C:
			snap := globalScanner.Snapshot()
			send(map[string]interface{}{
				"status": "scanning", "files": snap.Files, "dirs": snap.Dirs, "current": snap.Current,
			})
		}
	}
}
