package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"
)

//go:embed static
var staticFiles embed.FS

var version = "dev" // overridden at build time via -ldflags "-X main.version=vX.Y.Z"

func main() {
	settingsPath := flag.String("settings", "settings.json", "path to settings file")
	flag.Parse()

	cfg, err := loadSettings(*settingsPath)
	switch {
	case err == nil:
		if cfg.Auth.Enabled && cfg.Auth.Password == "" {
			cfg.Auth.Password = genPassword()
			if err := saveSettings(*settingsPath, cfg); err != nil {
				log.Printf("warning: could not update %s: %v", *settingsPath, err)
			}
			log.Printf("Auth enabled but no password set — generated: %s", cfg.Auth.Password)
		}
	case errors.Is(err, fs.ErrNotExist):
		cfg = defaultSettings()
		if err := saveSettings(*settingsPath, cfg); err != nil {
			log.Printf("warning: could not write %s: %v", *settingsPath, err)
		}
		log.Printf("Created %s — login password: %s", *settingsPath, cfg.Auth.Password)
	default:
		log.Fatalf("failed to load settings: %v", err)
	}
	setSettings(cfg)
	store.SetMax(cfg.HistorySize)
	secureCookies = cfg.TLS.Enabled()
	touchActivity()
	go idleReleaser(cfg.releaseAfterDuration())

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("failed to access embedded static files: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("/api/version", versionHandler)
	mux.HandleFunc("/api/auth", authHandler)
	mux.HandleFunc("/api/logout", logoutHandler)
	mux.HandleFunc("/api/config", authMiddleware(configHandler))
	mux.HandleFunc("/api/mounts", authMiddleware(mountsHandler))
	mux.HandleFunc("/api/scan", authMiddleware(scanHandler))
	mux.HandleFunc("/api/scans", authMiddleware(scansHandler))
	mux.HandleFunc("/api/data", authMiddleware(gzipMiddleware(dataHandler)))
	mux.HandleFunc("/api/compare", authMiddleware(compareHandler))
	mux.HandleFunc("/api/export", authMiddleware(gzipMiddleware(exportHandler)))
	mux.HandleFunc("/api/delete", authMiddleware(deleteHandler))

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// No WriteTimeout: /api/scan is a long-lived SSE stream.
	}

	authState := "disabled"
	if cfg.Auth.Enabled {
		authState = "enabled"
	}
	scheme := "http"
	if cfg.TLS.Enabled() {
		scheme = "https"
	}
	log.Printf("SpaceMonger %s listening on %s://%s (auth %s, %d scan workers, delete %v)",
		version, scheme, addr, authState, cfg.effectiveWorkers(), cfg.AllowDelete)
	if cfg.TLS.Enabled() {
		err = srv.ListenAndServeTLS(cfg.TLS.Cert, cfg.TLS.Key)
	} else {
		err = srv.ListenAndServe()
	}
	log.Fatal(err)
}
