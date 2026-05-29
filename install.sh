#!/bin/sh
# Install mantisbt-cli from GitHub Releases.
#
#   curl -fsSL https://raw.githubusercontent.com/overworks/mantisbt-cli/0.x/install.sh | sh
#
# Environment overrides:
#   VERSION       release tag to install (default: latest), e.g. VERSION=v0.1.0
#   INSTALL_DIR   directory to install into (default: $HOME/.local/bin)
set -eu

REPO="overworks/mantisbt-cli"
BIN="mantisbt-cli"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

err() {
	echo "install: $*" >&2
	exit 1
}

command -v curl >/dev/null 2>&1 || err "curl is required"
command -v tar >/dev/null 2>&1 || err "tar is required"

# Detect OS.
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
linux | darwin) ;;
*) err "unsupported OS '$os' — on Windows, download a .zip from https://github.com/$REPO/releases" ;;
esac

# Detect architecture.
arch=$(uname -m)
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) err "unsupported architecture '$arch'" ;;
esac

# Resolve the release tag.
tag="${VERSION:-latest}"
if [ "$tag" = "latest" ]; then
	tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
		grep '"tag_name"' | head -1 | cut -d '"' -f4)
fi
[ -n "$tag" ] || err "could not determine the latest version"
ver="${tag#v}"

asset="${BIN}_${ver}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $asset ($tag)…"
curl -fsSL -o "$tmp/$asset" "$base/$asset" || err "download failed: $base/$asset"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || err "could not fetch checksums.txt"

# Verify the SHA-256 checksum.
echo "Verifying checksum…"
expected=$(awk -v f="$asset" '$2 == f {print $1}' "$tmp/checksums.txt")
[ -n "$expected" ] || err "no checksum listed for $asset"
if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$tmp/$asset" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
	actual=$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')
else
	err "need sha256sum or shasum to verify the download"
fi
[ "$expected" = "$actual" ] || err "checksum mismatch for $asset"

# Extract and install.
tar -xzf "$tmp/$asset" -C "$tmp"
mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/$BIN" "$INSTALL_DIR/$BIN" 2>/dev/null ||
	{ cp "$tmp/$BIN" "$INSTALL_DIR/$BIN" && chmod 0755 "$INSTALL_DIR/$BIN"; } ||
	err "could not write to $INSTALL_DIR (set INSTALL_DIR or re-run with sudo)"

echo "Installed $BIN $ver to $INSTALL_DIR/$BIN"
case ":$PATH:" in
*":$INSTALL_DIR:"*) ;;
*) echo "Note: $INSTALL_DIR is not on your PATH. Add it, e.g.:" && echo "  export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac
