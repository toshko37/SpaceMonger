package main

import (
	"log"
	"runtime/debug"
	"sync/atomic"
	"time"
)

// lastActivity is the unix time of the last authenticated API request or
// scan completion. The idle releaser uses it to decide when stored scans
// can be dropped.
var lastActivity atomic.Int64

func touchActivity() { lastActivity.Store(time.Now().Unix()) }

func idleFor() time.Duration {
	return time.Since(time.Unix(lastActivity.Load(), 0))
}

// idleReleaser periodically frees stored scans once the server has been
// idle for longer than `after`. The scanned trees are the only thing that
// makes the process large; without them it sits at a few MB.
func idleReleaser(after time.Duration) {
	if after <= 0 {
		return
	}
	interval := after / 2
	if interval > time.Minute {
		interval = time.Minute
	}
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		maybeRelease(after)
	}
}

// maybeRelease drops the stored scans if idle long enough and no scan is
// running. It returns true when something was released.
func maybeRelease(after time.Duration) bool {
	if idleFor() < after {
		return false
	}
	if store.Len() == 0 && globalScanner.Snapshot().Root == nil {
		return false
	}
	if !scanMu.TryLock() { // a scan is in progress
		return false
	}
	defer scanMu.Unlock()

	n := store.Len()
	store.Clear()
	globalScanner.Release()
	debug.FreeOSMemory() // run GC and hand freed pages back to the kernel now
	log.Printf("idle for %s: released %d stored scan(s)", after, n)
	return true
}

// Scanning and JSON encoding allocate far more temporary memory than the
// resulting tree occupies (directory entries, stat buffers, the encoded
// response). Go keeps freed heap mapped and hands it back to the kernel
// only slowly, so RSS stays at the peak. trimMemory runs in the background
// after those operations so idle RSS reflects the live data only.
var trimPending atomic.Bool

func trimMemory() {
	if !trimPending.CompareAndSwap(false, true) {
		return // one is already queued
	}
	go func() {
		defer trimPending.Store(false)
		time.Sleep(2 * time.Second) // let the response finish and coalesce bursts
		debug.FreeOSMemory()
	}()
}
