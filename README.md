# SpaceMonger Linux

Web-based disk space analyzer for Linux. Displays your filesystem as an
interactive squarified treemap with color-coded sizes — inspired by SpaceMonger
for Windows.

![SpaceMonger Screenshot](docs/screenshot.png)

## Features

- 🗺️ Squarified treemap — green (small) → yellow → red (large)
- 🔍 Click any folder to zoom in, navigate back with Zoom Out / breadcrumb
- 📊 Free Space button shows free vs used proportionally
- ⚡ Real-time scan progress with file/folder counter
- 🔒 Optional password protection via `settings.json`
- 🌐 Accessible from local network (binds to 0.0.0.0)
- 📦 Single binary, no dependencies, ~10 MB

## Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/OWNER/spacemonger/main/install.sh | sudo bash
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

| Button       | Action                                      |
|-------------|---------------------------------------------|
| **Open**    | Choose a drive/partition to analyze         |
| **Reload**  | Rescan the current path                     |
| **Full**    | Zoom back to the scan root                  |
| **In**      | Zoom into selected folder                   |
| **Out**     | Go up one directory level                   |
| **Free Space** | Show free disk space proportionally      |

Click any folder rectangle to zoom in. Use the breadcrumb path to navigate.
Press **Backspace** or **←** to go up.

## Build from Source

```bash
git clone https://github.com/OWNER/spacemonger.git
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
