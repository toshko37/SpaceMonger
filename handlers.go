package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": version})
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

// resultPayload is the JSON shape of a finished scan, shared by the SSE
// "done" event and GET /api/data.
func resultPayload(snap ScanSnapshot) map[string]interface{} {
	errMsg := ""
	if snap.Err != nil {
		errMsg = snap.Err.Error()
	}
	return map[string]interface{}{
		"status":       "done",
		"path":         snap.Path,
		"root":         snap.Root,
		"files":        snap.Files,
		"dirs":         snap.Dirs,
		"totalDisk":    snap.TotalDisk,
		"freeDisk":     snap.FreeDisk,
		"apparentSize": snap.Apparent,
		"scannedAt":    snap.FinishedAt.Unix(),
		"durationMs":   snap.FinishedAt.Sub(snap.StartedAt).Milliseconds(),
		"error":        errMsg,
	}
}

// dataHandler returns the last completed scan so a reloaded page can
// restore its view without rescanning.
func dataHandler(w http.ResponseWriter, r *http.Request) {
	snap := globalScanner.Snapshot()
	if !snap.Done || snap.Root == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no scan data available"})
		return
	}
	writeJSON(w, http.StatusOK, resultPayload(snap))
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

	done := make(chan struct{})
	go func() {
		defer scanMu.Unlock()
		defer close(done)
		globalScanner.Scan(ctx, path)
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
		case <-done:
			// "building" lets the browser switch its UI before we spend
			// time serialising a possibly huge tree.
			send(map[string]string{"status": "building"})
			send(resultPayload(globalScanner.Snapshot()))
			return
		case <-ticker.C:
			snap := globalScanner.Snapshot()
			send(map[string]interface{}{
				"status": "scanning", "files": snap.Files, "dirs": snap.Dirs, "current": snap.Current,
			})
		}
	}
}
