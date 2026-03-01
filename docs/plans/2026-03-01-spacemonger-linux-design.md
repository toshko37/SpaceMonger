# SpaceMonger Linux — Design Document

**Date:** 2026-03-01
**Status:** Approved

## Overview

Web-based disk space visualizer for Linux, inspired by SpaceMonger for Windows. Displays filesystem structure as a squarified treemap with color-coded rectangles proportional to file/folder sizes. Self-hosted on port 4322, accessible from the local network.

---

## Technology Stack

- **Backend:** Go 1.21+, compiled to a single binary
- **Frontend:** HTML5 + CSS3 + Vanilla JS + D3.js v7 (embedded in binary)
- **Communication:** REST API + SSE (Server-Sent Events) for scan progress
- **Distribution:** Single binary with `//go:embed`, curl install script

---

## Project Structure

```
spacemonger/
├── main.go                        # Go server — all backend logic
├── static/
│   ├── index.html                 # Main page
│   ├── app.js                     # Treemap + UI logic
│   ├── style.css                  # Styles
│   └── d3.v7.min.js               # D3.js v7 (~250KB, bundled offline)
├── settings.example.json          # Example config (committed to git)
├── settings.json                  # Real config (gitignored)
├── install.sh                     # Installation script
├── uninstall.sh                   # Uninstallation script
├── spacemonger.service            # systemd unit file
├── .github/
│   └── workflows/
│       └── build.yml              # GitHub Actions: build + release
├── go.mod
├── go.sum
├── .gitignore
└── README.md
```

---

## Backend (Go)

### HTTP Server
- Listens on configurable port (default: 4322), bind address (default: 0.0.0.0)
- All static files embedded via `//go:embed static/*`
- Auth middleware: if `auth.enabled = true`, redirects unauthenticated requests to login page

### REST API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/auth` | Login with password → sets session cookie |
| `GET`  | `/api/mounts` | List real mountpoints from `/proc/mounts` |
| `GET`  | `/api/scan?path=/` | Start scan → SSE stream with progress |
| `GET`  | `/api/data` | Get last scan result as JSON tree |

### SSE Scan Stream Format
```
data: {"status":"scanning","files":12453,"dirs":1847,"current":"/home/user/..."}
...
data: {"status":"done","files":58093,"dirs":20475,"root":{...full tree...}}
```

### JSON Tree Structure
```json
{
  "name": "/",
  "size": 0,
  "children": [
    {
      "name": "home",
      "size": 45678901234,
      "mtime": "2026-01-15T10:30:00Z",
      "children": [...]
    },
    {
      "name": "bigfile.iso",
      "size": 4294967296,
      "mtime": "2026-02-20T14:22:00Z"
    }
  ]
}
```

### Mountpoints Detection
Reads `/proc/mounts`, filters to real filesystems: `ext4`, `ext3`, `xfs`, `btrfs`, `ntfs`, `vfat`, `f2fs`, `zfs`. Excludes: `tmpfs`, `proc`, `sysfs`, `devpts`, `cgroup`, etc.

### Authentication
- Session token: 32-byte random hex, stored in memory only
- Cookie name: `sm_session`
- If `auth.enabled = true`: all endpoints require valid session cookie
- Login endpoint: `POST /api/auth` with `{"password": "..."}` → returns session cookie
- No session expiry (clears when server restarts or browser closes)

---

## Frontend

### Toolbar
```
[Open] [Reload] [Zoom Full] [Zoom In*] [Zoom Out*]     [Free Space]
```
- `*` — grayed out / disabled when at root level (Zoom Full, Zoom Out)
- **Open** → modal dialog listing real mountpoints
- **Reload** → rescans current path
- **Zoom Full** → jumps back to scan root
- **Zoom In** → zooms into selected folder (or double-click)
- **Zoom Out** → goes up one level
- **Free Space** → toggle: adds proportional gray rectangle for free disk space

### Treemap
- D3.js squarified treemap: `d3.treemap().tile(d3.treemapSquarify)`
- Color scale: `d3.scaleSequential(d3.interpolateRdYlGn)` reversed: **green = small, yellow = medium, red = large**
- Each rectangle shows: name + size text (if space allows)
- Hover tooltip: full name, exact size (auto-formatted), last modified date
- Click folder → zoom in (it becomes treemap root)
- Responsive: redraws on window resize

### Breadcrumb Navigation (below toolbar)
```
/ > home > user > Downloads > [current]
```
Click any segment to zoom to that level.

### Scan Progress Overlay
Shown during scanning, uses SSE stream:
```
╔══════════════════════════════════╗
║  Сканиране...                    ║
║  Файлове: 12,453  Папки: 1,847   ║
║  /home/user/Downloads/...        ║
╚══════════════════════════════════╝
```

### Status Bar (bottom)
```
Total: 58,093 files | 20,475 dirs | Used: 345.2 GB | Free: 124.8 GB
```

---

## settings.json

```json
{
  "port": 4322,
  "bind": "0.0.0.0",
  "auth": {
    "enabled": false,
    "password": "k3x9mz"
  }
}
```

- `auth.enabled`: false by default
- `auth.password`: auto-generated 6-char alphanumeric string at install time
- File lives at `/opt/spacemonger/settings.json`
- `settings.example.json` (no real password) is committed to git

---

## install.sh

1. Detect CPU architecture (`uname -m` → amd64 / arm64)
2. Download pre-built binary from GitHub Releases for correct arch
3. Create `/opt/spacemonger/` directory
4. Place binary there, make executable
5. Generate `settings.json` with random 6-char password
6. Copy `spacemonger.service` to `/etc/systemd/system/`
7. `systemctl daemon-reload && systemctl enable --now spacemonger`
8. Print: `SpaceMonger installed! Open: http://<IP>:4322`

## uninstall.sh

1. `systemctl stop spacemonger && systemctl disable spacemonger`
2. `rm /etc/systemd/system/spacemonger.service`
3. `systemctl daemon-reload`
4. `rm -rf /opt/spacemonger/`
5. Confirm: "SpaceMonger removed."

---

## GitHub Actions (build.yml)

On push to `main` with a version tag (`v*`):
1. Build for `linux/amd64` and `linux/arm64`
2. Attach both binaries to GitHub Release
3. Binary names: `spacemonger-linux-amd64`, `spacemonger-linux-arm64`

---

## Security Considerations

- Filesystem access is read-only (only `os.ReadDir`, `os.Stat`)
- Server runs as the user that starts it (root for system service — gives full FS access)
- Auth password in `settings.json` is plain text (acceptable for self-hosted, local network tool)
- No user input is executed as a shell command (no injection risk)
- CORS not enabled (same-origin only)
