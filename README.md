# SpaceMonger

Web-based disk space analyzer for Linux. Scans a filesystem and shows it as an
interactive squarified treemap, so you can see at a glance where the space went.
Single static binary, no dependencies, dark and light themes.

Inspired by the classic SpaceMonger for Windows.

## Features

- Squarified treemap with depth-based colours; single-click selects, double-click zooms in
- Breadcrumb navigation, **Full / In / Up** buttons and keyboard shortcuts
- **Free space** mode shows used vs. free space proportionally
- Real on-disk usage (`st_blocks`): sparse files, transparent compression (btrfs, ZFS)
  and hard links are accounted for, with an apparent-vs-actual summary bar
- Stays on one filesystem: skips other mount points, `/proc`, `/sys`, symlinks, `.zfs`
- Live scan progress over Server-Sent Events, cancellable at any time
- Last scan result is kept in memory, so reloading the page is instant
- Password protection **on by default**, rate-limited login, 24h sessions, logout
- Dark / light theme (follows the system, toggle in the toolbar)
- Single ~10 MB binary with the UI embedded; runs as a systemd service

## Quick install

```bash
curl -fsSL https://raw.githubusercontent.com/toshko37/SpaceMonger/main/install.sh | sudo bash
```

The installer prints the generated login password. The UI is at
**http://localhost:4322** and reachable from the local network.

Uninstall:

```bash
sudo bash /opt/spacemonger/uninstall.sh
```

## Configuration

`/opt/spacemonger/settings.json`:

```json
{
  "port": 4322,
  "bind": "0.0.0.0",
  "auth": {
    "enabled": true,
    "password": "changeme"
  }
}
```

| Key             | Meaning                                                             |
|-----------------|---------------------------------------------------------------------|
| `port`          | TCP port to listen on                                               |
| `bind`          | `0.0.0.0` for the whole LAN, `127.0.0.1` for this machine only      |
| `auth.enabled`  | Require the password. Leave on unless the host is otherwise isolated |
| `auth.password` | Login password. If empty while auth is enabled, one is generated and logged |

Restart after changes: `sudo systemctl restart spacemonger`.

The binary also accepts `-settings /path/to/settings.json`.

## Security notes

SpaceMonger runs as root (it has to, to read every directory) and exposes the
names and sizes of all files on the scanned filesystem. Keep auth enabled when
`bind` is `0.0.0.0`. Login attempts are limited to 5 per minute per IP. Traffic
is plain HTTP: put a reverse proxy with TLS in front if you expose it beyond a
trusted LAN.

## Usage

| Control         | Action                                            |
|-----------------|---------------------------------------------------|
| **Open**        | Choose a mounted filesystem to scan (`O`)         |
| **Reload**      | Rescan the current path (`R`)                     |
| **Full**        | Back to the scan root (`Home`)                    |
| **In**          | Zoom into the selected folder (`Enter`)           |
| **Up**          | One level up (`Backspace` / `←`)                  |
| **Free space**  | Show free space next to the treemap               |
| Single click    | Select a cell (details in the status bar)         |
| Double click    | Zoom into a folder                                |
| `Esc`           | Deselect / close dialog                           |

## API

All endpoints except `/api/version` and `/api/auth` require a session cookie
when auth is enabled.

| Method | Path                 | Description                                          |
|--------|----------------------|------------------------------------------------------|
| GET    | `/api/auth`          | `{authRequired, authenticated}`                      |
| POST   | `/api/auth`          | `{"password": "..."}` → sets session cookie          |
| POST   | `/api/logout`        | Invalidate the session                               |
| GET    | `/api/mounts`        | Real filesystems from `/proc/mounts` with usage      |
| GET    | `/api/scan?path=/`   | SSE stream: progress events, then the full tree      |
| GET    | `/api/data`          | Last completed scan                                  |
| GET    | `/api/version`       | Build version                                        |

A new scan request cancels any scan still running.

## Build from source

```bash
git clone https://github.com/toshko37/SpaceMonger.git
cd SpaceMonger
go build -o spacemonger .
./spacemonger            # creates settings.json and prints the password
```

Requires Go 1.22+. Run the tests with `go test -race ./...`.

Project layout:

```
main.go       server setup and routing
settings.go   settings.json loading / defaults
scanner.go    filesystem walker (cancellable, hard-link and mount aware)
mounts.go     /proc/mounts parsing
auth.go       sessions, login rate limiting
handlers.go   HTTP / SSE handlers
static/       embedded UI (vanilla JS + D3 v7)
```

## Releasing

```bash
git tag v1.1.0
git push origin v1.1.0
```

GitHub Actions runs the tests, builds amd64 / arm64 / armv7 binaries and
attaches them to the release.

## Author

Todor Karachorbadzhiev — [app@techbg.net](mailto:app@techbg.net)

## License

MIT — see [LICENSE](LICENSE)
