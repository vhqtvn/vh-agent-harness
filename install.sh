#!/bin/bash
# install.sh — fetch the latest vh-agent-harness release and install the binary.
#
# vh-agent-harness publishes goreleaser tar.gz archives
# (vh-agent-harness_<version>_<os>_<arch>.tar.gz) plus a checksums.txt, so this
# script downloads the archive, verifies its SHA256, and unpacks the
# vh-agent-harness binary.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/vhqtvn/vh-agent-harness/main/install.sh | bash
set -euo pipefail

# REPO_URL / API_URL are overridable (e.g. for testing against a local mirror).
REPO_URL="${REPO_URL:-https://github.com/vhqtvn/vh-agent-harness}"
API_URL="${API_URL:-https://api.github.com/repos/vhqtvn/vh-agent-harness/releases/latest}"
# Preferred (system-wide) target; harness does NOT need to be system-wide — if
# this isn't writable we fall back to the user's bin path after confirmation.
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
USER_BIN_DIR="${XDG_BIN_HOME:-$HOME/.local/bin}"
BIN_NAME="vh-agent-harness"

need() { command -v "$1" >/dev/null 2>&1 || { echo "Error: '$1' is required but not installed." >&2; exit 1; }; }
need curl
need tar

# ask <prompt> <default> — read a line from the terminal even when the script
# itself arrives on stdin (curl | bash). Echoes the answer (or the default when
# input is empty or no tty is available).
ask() {
    local ans=""
    if [ -r /dev/tty ] && [ -w /dev/tty ]; then
        printf '%s ' "$1" > /dev/tty
        read -r ans < /dev/tty || ans=""
    fi
    echo "${ans:-$2}"
}

echo "Fetching latest release information..."
LATEST_RELEASE=$(curl -fsSL "$API_URL")

if [ -z "$LATEST_RELEASE" ] || echo "$LATEST_RELEASE" | grep -q '"message": *"Not Found"'; then
    echo "Error: Could not fetch latest release. Make sure the repository is public or you have access." >&2
    exit 1
fi

# Parse tag_name with jq when available, else a portable grep/sed fallback.
if command -v jq >/dev/null 2>&1; then
    TAG=$(echo "$LATEST_RELEASE" | jq -r '.tag_name')
else
    TAG=$(echo "$LATEST_RELEASE" | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
fi
if [ -z "$TAG" ] || [ "$TAG" = "null" ]; then
    echo "Error: Could not parse release version." >&2
    exit 1
fi

# goreleaser strips the leading "v" from the version embedded in the filename.
VERSION="${TAG#v}"
echo "Latest version is $TAG"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
    x86_64 | amd64) ARCH="amd64" ;;
    aarch64 | arm64) ARCH="arm64" ;;
    *) echo "Error: unsupported architecture '$ARCH' (released targets: amd64, arm64)." >&2; exit 1 ;;
esac
case $OS in
    linux | darwin) ;;
    *) echo "Error: unsupported OS '$OS' (released targets: linux, darwin)." >&2; exit 1 ;;
esac

ARCHIVE="${BIN_NAME}_${VERSION}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="${REPO_URL}/releases/download/${TAG}/${ARCHIVE}"
CHECKSUMS_URL="${REPO_URL}/releases/download/${TAG}/checksums.txt"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Downloading $ARCHIVE..."
if ! curl -fsSL -o "$TMP_DIR/$ARCHIVE" "$DOWNLOAD_URL"; then
    echo "Error: download failed ($DOWNLOAD_URL)." >&2
    exit 1
fi

# Verify the archive against checksums.txt (best-effort: only if a checksum
# tool and the checksums asset are both available).
if curl -fsSL -o "$TMP_DIR/checksums.txt" "$CHECKSUMS_URL" 2>/dev/null; then
    WANT=$(grep -E "(^|[* ])${ARCHIVE}\$" "$TMP_DIR/checksums.txt" | awk '{print $1}' | head -n1)
    if [ -n "$WANT" ]; then
        if command -v sha256sum >/dev/null 2>&1; then
            GOT=$(sha256sum "$TMP_DIR/$ARCHIVE" | awk '{print $1}')
        elif command -v shasum >/dev/null 2>&1; then
            GOT=$(shasum -a 256 "$TMP_DIR/$ARCHIVE" | awk '{print $1}')
        fi
        if [ -n "${GOT:-}" ] && [ "$GOT" != "$WANT" ]; then
            echo "Error: checksum mismatch (got $GOT, expected $WANT)." >&2
            exit 1
        fi
        [ -n "${GOT:-}" ] && echo "Checksum verified."
    fi
fi

echo "Unpacking..."
tar -xzf "$TMP_DIR/$ARCHIVE" -C "$TMP_DIR" "$BIN_NAME"
chmod +x "$TMP_DIR/$BIN_NAME"

# Install target. If the system dir is writable, use it directly. Otherwise
# harness doesn't need to be system-wide, so ask where to install: the system
# path (which we then write via sudo) or the user's bin path (no sudo).
TARGET_DIR="$INSTALL_DIR"
USE_SUDO=0
chmod 0755 "$TMP_DIR/$BIN_NAME"

if [ ! -w "$INSTALL_DIR" ] && [ ! -w "$(dirname "$INSTALL_DIR")" ]; then
    echo "Cannot write to $INSTALL_DIR (no permission)."
    echo "Choose install location:"
    echo "  [1] System path  $INSTALL_DIR  (requires sudo)"
    echo "  [2] User path    $USER_BIN_DIR"
    CHOICE=$(ask "Choice [2]:" 2)
    case "$CHOICE" in
        1) TARGET_DIR="$INSTALL_DIR"; USE_SUDO=1 ;;
        *) TARGET_DIR="$USER_BIN_DIR" ;;
    esac
fi

if [ "$USE_SUDO" = "1" ]; then
    need sudo
    echo "Installing to $TARGET_DIR (sudo)..."
    sudo install -m 0755 "$TMP_DIR/$BIN_NAME" "$TARGET_DIR/$BIN_NAME"
else
    mkdir -p "$TARGET_DIR"
    install -m 0755 "$TMP_DIR/$BIN_NAME" "$TARGET_DIR/$BIN_NAME" 2>/dev/null \
        || mv "$TMP_DIR/$BIN_NAME" "$TARGET_DIR/$BIN_NAME"
fi

echo "Installation complete: $TARGET_DIR/$BIN_NAME"
case ":$PATH:" in
    *":$TARGET_DIR:"*) ;;
    *) echo "Note: $TARGET_DIR is not on your PATH. Add it, e.g.:"
       echo "  export PATH=\"$TARGET_DIR:\$PATH\"" ;;
esac
echo "Run '$BIN_NAME --help' to get started (or '$BIN_NAME version')."
