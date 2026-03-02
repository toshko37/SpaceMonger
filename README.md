# SpaceMonger Linux — Retro

Web-based disk space analyzer for Linux with a retro Windows 98 aesthetic.
Displays your filesystem as an interactive squarified treemap with depth-based
color coding — inspired by SpaceMonger for Windows.

## Features

- 🗺️ Squarified treemap — depth-based colors matching original SpaceMonger palette
- 🖱️ Single-click to select a folder, double-click to zoom in
- 🔍 Navigate back with **Zoom Out** button or breadcrumb path
- 📊 **Free Space** button shows free vs. used space proportionally
- ⚡ Real-time scan progress with file/folder counter
- 🔒 Optional password protection via `settings.json`
- 🌐 Accessible from local network (binds to 0.0.0.0)
- 📦 Single binary, no dependencies, ~10 MB

## Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/toshko37/SpaceMonger/main/install.sh | sudo bash
```

Opens at **http://localhost:4322** (also accessible from the local network).

## Uninstall

```bash
sudo bash /opt/spacemonger/uninstall.sh
```

## Configuration

Edit `/opt/spacemonger/settings.json`:

```json
{
  "port": 4322,
  "bind": "0.0.0.0",
  "auth": {
    "enabled": false,
    "password": "abc123"
  }
}
```

Restart after changes: `sudo systemctl restart spacemonger`

## Usage

| Button          | Action                                      |
|----------------|---------------------------------------------|
| **Open**       | Choose a drive/partition to analyze         |
| **Reload**     | Rescan the current path                     |
| **Full**       | Zoom back to the scan root                  |
| **In**         | Zoom into selected folder                   |
| **Out**        | Go up one directory level                   |
| **Free Space** | Show free disk space proportionally         |
| **About**      | Version and author information              |

Click any folder rectangle to zoom in. Single-click selects (highlights border),
double-click zooms in. Use the breadcrumb path to navigate.
Press **Backspace** or **←** to go up.

## Build from Source

```bash
git clone https://github.com/toshko37/SpaceMonger.git
cd spacemonger
go build -o spacemonger .
./spacemonger          # runs on port 4322
```

Requires Go 1.21+.

## Releasing a New Version

```bash
git tag v1.0.0
git push origin v1.0.0
```

GitHub Actions will build binaries for amd64/arm64/arm and attach them to the release.

## Author

Created by Todor Karachorbadzhiev — [app@techbg.net](mailto:app@techbg.net)

## License

MIT — see [LICENSE](LICENSE)
