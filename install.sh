#!/bin/sh
# ABOUTME: Shell installer for keytun. Downloads the latest release from GitHub.
# ABOUTME: Usage: curl -fsSL https://keytun.com/install.sh | sh
set -e

REPO="gboston/keytun"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
    darwin) ;;
    linux) ;;
    *)
        echo "Error: unsupported OS: $OS" >&2
        exit 1
        ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)
        echo "Error: unsupported architecture: $ARCH" >&2
        exit 1
        ;;
esac

# Get latest version
echo "Fetching latest release..."
VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')"
if [ -z "$VERSION" ]; then
    echo "Error: could not determine latest version" >&2
    exit 1
fi
echo "Latest version: $VERSION"

# Strip leading v for asset name
VERSION_NUM="${VERSION#v}"
ASSET="keytun_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"

# Download and extract
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading ${ASSET}..."
curl -fsSL "$URL" -o "${TMPDIR}/${ASSET}"

echo "Extracting..."
tar xzf "${TMPDIR}/${ASSET}" -C "$TMPDIR"

# Install
if [ -w "$INSTALL_DIR" ]; then
    mv "${TMPDIR}/keytun" "${INSTALL_DIR}/keytun"
else
    echo "Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo mv "${TMPDIR}/keytun" "${INSTALL_DIR}/keytun"
fi
chmod +x "${INSTALL_DIR}/keytun"

echo "keytun ${VERSION} installed to ${INSTALL_DIR}/keytun"
