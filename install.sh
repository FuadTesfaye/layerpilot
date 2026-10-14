#!/usr/bin/env bash
set -euo pipefail

REPO="FuadTesfaye/layerpilot"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
    linux*)  OS="linux" ;;
    darwin*) OS="darwin" ;;
    msys*|mingw*|cygwin*) OS="windows" ;;
    *) echo "Unsupported operating system: $OS" >&2; exit 1 ;;
esac

# Detect Architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

echo "=> Installing Universal App Compatibility Tool (compat) for ${OS}/${ARCH}..."

# Fetch latest release tag
LATEST_TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || echo "v0.1.0")"
if [ -z "$LATEST_TAG" ]; then
    LATEST_TAG="v0.1.0"
fi

TARBALL="compat_${LATEST_TAG}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST_TAG}/${TARBALL}"

TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

echo "=> Downloading ${DOWNLOAD_URL}..."
if ! curl -fsSL "$DOWNLOAD_URL" -o "${TEMP_DIR}/${TARBALL}"; then
    echo "Direct asset download failed, attempting latest asset resolution..."
    DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${TARBALL}"
    curl -fsSL "$DOWNLOAD_URL" -o "${TEMP_DIR}/${TARBALL}"
fi

echo "=> Extracting archive..."
mkdir -p "$INSTALL_DIR"
tar -xzf "${TEMP_DIR}/${TARBALL}" -C "$TEMP_DIR"

if [ -f "${TEMP_DIR}/compat" ]; then
    mv "${TEMP_DIR}/compat" "${INSTALL_DIR}/compat"
    chmod +x "${INSTALL_DIR}/compat"
elif [ -f "${TEMP_DIR}/compat.exe" ]; then
    mv "${TEMP_DIR}/compat.exe" "${INSTALL_DIR}/compat.exe"
fi

echo "=> Successfully installed compat to ${INSTALL_DIR}/compat"

# Check PATH
case ":$PATH:" in
    *:"$INSTALL_DIR":*) ;;
    *)
        echo ""
        echo "Note: ${INSTALL_DIR} is not in your PATH."
        echo "Add it by running:"
        echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
        ;;
esac

echo ""
"${INSTALL_DIR}/compat" version
echo ""
echo "Quick start: run 'compat doctor' to verify host readiness."
