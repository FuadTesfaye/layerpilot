#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

echo "=> Uninstalling Universal App Compatibility Tool (compat)..."
if [ -f "${INSTALL_DIR}/compat" ]; then
    rm -f "${INSTALL_DIR}/compat"
    echo "   Removed ${INSTALL_DIR}/compat"
fi
if [ -f "${INSTALL_DIR}/compat.exe" ]; then
    rm -f "${INSTALL_DIR}/compat.exe"
    echo "   Removed ${INSTALL_DIR}/compat.exe"
fi

if [ "${1:-}" = "--purge" ]; then
    echo "=> Purging managed application data in ~/.local/share/compat..."
    rm -rf "$HOME/.local/share/compat"
    echo "   Removed ~/.local/share/compat"
fi

echo "=> Successfully uninstalled compat."
