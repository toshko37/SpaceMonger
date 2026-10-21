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

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("failed to access embedded static files: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("/api/version", versionHandler)
	mux.HandleFunc("/api/auth", authHandler)
	mux.HandleFunc("/api/logout", logoutHandler)
	mux.HandleFunc("/api/mounts", authMiddleware(mountsHandler))
	mux.HandleFunc("/api/scan", authMiddleware(scanHandler))
	mux.HandleFunc("/api/data", authMiddleware(dataHandler))

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
	log.Printf("SpaceMonger %s listening on http://%s (auth %s)", version, addr, authState)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
