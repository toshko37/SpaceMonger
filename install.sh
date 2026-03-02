#!/usr/bin/env bash
# SpaceMonger Linux — Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/toshko37/SpaceMonger/main/install.sh | sudo bash

set -euo pipefail

REPO="toshko37/SpaceMonger"
INSTALL_DIR="/opt/spacemonger"
SERVICE_NAME="spacemonger"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

# ─── Colors ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
info()    { echo -e "${GREEN}[✓]${NC} $*"; }
warn()    { echo -e "${YELLOW}[!]${NC} $*"; }
error()   { echo -e "${RED}[✗]${NC} $*"; exit 1; }

echo "╔══════════════════════════════════════╗"
echo "║     SpaceMonger Linux Installer      ║"
echo "╚══════════════════════════════════════╝"
echo ""

# ─── Root check ───────────────────────────────────────────────────────────────
[ "$EUID" -eq 0 ] || error "Please run as root: sudo bash install.sh"

# ─── Detect architecture ──────────────────────────────────────────────────────
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)         ARCH_TAG="amd64" ;;
    aarch64|arm64)  ARCH_TAG="arm64" ;;
    armv7l)         ARCH_TAG="arm"   ;;
    *)              error "Unsupported architecture: $ARCH" ;;
esac

BINARY_NAME="spacemonger-linux-${ARCH_TAG}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${BINARY_NAME}"

info "Architecture: $ARCH_TAG"
info "Installing to: $INSTALL_DIR"

# ─── Create install directory ─────────────────────────────────────────────────
mkdir -p "$INSTALL_DIR"

# ─── Download binary ──────────────────────────────────────────────────────────
info "Downloading $BINARY_NAME..."
if command -v curl &>/dev/null; then
    curl -fsSL "$DOWNLOAD_URL" -o "$INSTALL_DIR/spacemonger" || \
        error "Download failed. Check: $DOWNLOAD_URL"
elif command -v wget &>/dev/null; then
    wget -q "$DOWNLOAD_URL" -O "$INSTALL_DIR/spacemonger" || \
        error "Download failed. Check: $DOWNLOAD_URL"
else
    error "curl or wget is required for installation"
fi
chmod +x "$INSTALL_DIR/spacemonger"
info "Downloaded spacemonger binary"

# ─── Write uninstall script ───────────────────────────────────────────────────
cat > "$INSTALL_DIR/uninstall.sh" <<'UNINSTALL'
#!/usr/bin/env bash
# SpaceMonger Linux — Uninstaller
set -euo pipefail

INSTALL_DIR="/opt/spacemonger"
SERVICE_NAME="spacemonger"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

RED='\033[0;31m'; GREEN='\033[0;32m'; NC='\033[0m'
info()  { echo -e "${GREEN}[✓]${NC} $*"; }
error() { echo -e "${RED}[✗]${NC} $*"; exit 1; }

echo "SpaceMonger Uninstaller"
echo "========================"

[ "$EUID" -eq 0 ] || error "Please run as root: sudo bash /opt/spacemonger/uninstall.sh"

if systemctl is-active  --quiet "$SERVICE_NAME" 2>/dev/null; then
    systemctl stop "$SERVICE_NAME"
    info "Stopped $SERVICE_NAME service"
fi

if systemctl is-enabled --quiet "$SERVICE_NAME" 2>/dev/null; then
    systemctl disable "$SERVICE_NAME"
    info "Disabled $SERVICE_NAME service"
fi

if [ -f "$SERVICE_FILE" ]; then
    rm -f "$SERVICE_FILE"
    systemctl daemon-reload
    info "Removed systemd service"
fi

if [ -d "$INSTALL_DIR" ]; then
    rm -rf "$INSTALL_DIR"
    info "Removed $INSTALL_DIR"
fi

echo ""
echo -e "${GREEN}SpaceMonger has been completely removed.${NC}"
UNINSTALL
chmod +x "$INSTALL_DIR/uninstall.sh"
info "Wrote uninstall script"

# ─── Generate settings.json if missing ───────────────────────────────────────
if [ ! -f "$INSTALL_DIR/settings.json" ]; then
    PASSWORD=$(< /dev/urandom tr -dc 'a-z0-9' | head -c 6 || true)
    [ -z "$PASSWORD" ] && PASSWORD=$(date +%s | sha256sum | head -c 6)
    cat > "$INSTALL_DIR/settings.json" <<SETTINGS
{
  "port": 4322,
  "bind": "0.0.0.0",
  "auth": {
    "enabled": false,
    "password": "${PASSWORD}"
  }
}
SETTINGS
    chmod 600 "$INSTALL_DIR/settings.json"
    info "Created settings.json (generated password: ${YELLOW}${PASSWORD}${NC})"
    warn "Auth is disabled by default. Edit settings.json to enable."
else
    info "Using existing settings.json"
fi

# ─── Install systemd service ──────────────────────────────────────────────────
cat > "$SERVICE_FILE" <<SERVICE
[Unit]
Description=SpaceMonger - Web-based Disk Space Analyzer
After=network.target
Documentation=https://github.com/${REPO}

[Service]
Type=simple
User=root
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/spacemonger
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
SERVICE

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"
info "Systemd service installed and started"

# ─── Get local IP ─────────────────────────────────────────────────────────────
LOCAL_IP=$(hostname -I 2>/dev/null | awk '{print $1}') || LOCAL_IP="<your-server-ip>"

# ─── Done ─────────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}╔══════════════════════════════════════╗${NC}"
echo -e "${GREEN}║  SpaceMonger installed successfully  ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════╝${NC}"
echo ""
echo -e "  Local:    ${GREEN}http://localhost:4322${NC}"
echo -e "  Network:  ${GREEN}http://${LOCAL_IP}:4322${NC}"
echo ""
echo "  Config:   $INSTALL_DIR/settings.json"
echo "  Auth:     edit settings.json, then: systemctl restart $SERVICE_NAME"
echo ""
echo "  Commands:"
echo "    Status:  systemctl status $SERVICE_NAME"
echo "    Logs:    journalctl -u $SERVICE_NAME -f"
echo "    Stop:    systemctl stop $SERVICE_NAME"
echo "    Remove:  bash ${INSTALL_DIR}/uninstall.sh"
echo ""
