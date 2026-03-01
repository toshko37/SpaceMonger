#!/usr/bin/env bash
# SpaceMonger Linux — Uninstaller
# Usage: sudo bash /opt/spacemonger/uninstall.sh

set -euo pipefail

INSTALL_DIR="/opt/spacemonger"
SERVICE_NAME="spacemonger"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

RED='\033[0;31m'; GREEN='\033[0;32m'; NC='\033[0m'
info()  { echo -e "${GREEN}[✓]${NC} $*"; }
error() { echo -e "${RED}[✗]${NC} $*"; exit 1; }

echo "SpaceMonger Uninstaller"
echo "========================"

[ "$EUID" -eq 0 ] || error "Please run as root: sudo bash uninstall.sh"

# ─── Stop and disable service ─────────────────────────────────────────────────
if systemctl is-active  --quiet "$SERVICE_NAME" 2>/dev/null; then
    systemctl stop "$SERVICE_NAME"
    info "Stopped $SERVICE_NAME service"
fi

if systemctl is-enabled --quiet "$SERVICE_NAME" 2>/dev/null; then
    systemctl disable "$SERVICE_NAME"
    info "Disabled $SERVICE_NAME service"
fi

# ─── Remove service file ──────────────────────────────────────────────────────
if [ -f "$SERVICE_FILE" ]; then
    rm -f "$SERVICE_FILE"
    systemctl daemon-reload
    info "Removed systemd service"
fi

# ─── Remove install directory ─────────────────────────────────────────────────
if [ -d "$INSTALL_DIR" ]; then
    rm -rf "$INSTALL_DIR"
    info "Removed $INSTALL_DIR"
fi

echo ""
echo -e "${GREEN}SpaceMonger has been completely removed.${NC}"
