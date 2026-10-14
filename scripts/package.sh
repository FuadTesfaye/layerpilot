#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-v0.1.0}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_DATE="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
DIST_DIR="dist"

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

LDFLAGS="-s -w -X 'github.com/FuadTesfaye/layerpilot/internal/version.Version=${VERSION}' -X 'github.com/FuadTesfaye/layerpilot/internal/version.GitCommit=${COMMIT}' -X 'github.com/FuadTesfaye/layerpilot/internal/version.BuildDate=${BUILD_DATE}'"

PLATFORMS=(
    "linux/amd64/compat"
    "linux/arm64/compat"
    "darwin/amd64/compat"
    "darwin/arm64/compat"
    "windows/amd64/compat.exe"
)

echo "Building release binaries for version ${VERSION}..."

for p in "${PLATFORMS[@]}"; do
    OS="$(echo "$p" | cut -d'/' -f1)"
    ARCH="$(echo "$p" | cut -d'/' -f2)"
    BIN_NAME="$(echo "$p" | cut -d'/' -f3)"
    
    STAGE_DIR="${DIST_DIR}/stage_${OS}_${ARCH}"
    mkdir -p "$STAGE_DIR"
    
    echo "  -> Compiling ${OS}/${ARCH}..."
    CGO_ENABLED=0 GOOS="$OS" GOARCH="$ARCH" go build -trimpath -ldflags="${LDFLAGS}" -o "${STAGE_DIR}/${BIN_NAME}" ./cmd/compat
    
    # Copy metadata
    cp LICENSE README.md "$STAGE_DIR/"
    
    ARCHIVE_BASE="compat_${VERSION}_${OS}_${ARCH}"
    if [ "$OS" = "windows" ]; then
        (cd "$STAGE_DIR" && zip -q -r "../../${DIST_DIR}/${ARCHIVE_BASE}.zip" .)
    else
        tar -czf "${DIST_DIR}/${ARCHIVE_BASE}.tar.gz" -C "$STAGE_DIR" .
    fi
    
    rm -rf "$STAGE_DIR"
done

echo "Generating SHA-256 checksums..."
(
    cd "$DIST_DIR"
    sha256sum compat_* > checksums.txt
)

echo "Release artifacts ready in ${DIST_DIR}/:"
ls -lh "$DIST_DIR"
